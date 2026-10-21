package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
)

// Budget persistence is synchronous and request-scoped, independent of the
// best-effort attempt log queue and its retention policy.
type userBudgetLedger interface {
	ReserveUserBudget(context.Context, string, int64, int64, int64) (store.UserBudgetReservation, error)
	SettleUserBudget(context.Context, string, *int64, bool) error
}
type requestUserBudget struct {
	ledger        userBudgetLedger
	tokens        int64
	usageFound    bool
	usageUnknown  bool
	reserved      bool
	seenAttempts  map[int]bool
	releaseActive func()
}

func (w *requestResponseWriter) AdmitProxyUserBudget(r *http.Request, pac *auth.ProxyAuthContext) (*http.Request, int, string) {
	w.state.bind(pac, r.URL.Path, "")
	if pac == nil || pac.OwnerUserID == nil {
		return r, 0, ""
	}
	if r.Method == http.MethodGet {
		if isUserBudgetModelList(r.URL.Path) || IsWebsocketUpgradeRequest(r) && strings.HasSuffix(r.URL.Path, "responses") {
			return r, 0, ""
		}
		return r, 403, "This resource protocol is not available with user token budgets"
	}
	if r.Method != http.MethodPost {
		return r, 403, "This protocol is not available with user token budgets"
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") && r.Header.Get("Content-Type") != "" {
		return r, 403, "User budgets require supported text JSON requests"
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return r, 400, "Could not read user request"
	}
	r.Body.Close()
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil || body == nil {
		return r, 400, "Invalid user request JSON"
	}
	status, message := w.state.admitUserBudget(r.Context(), pac, r.URL.Path, body)
	if status != 0 {
		return r, status, message
	}
	raw, err = json.Marshal(body)
	if err != nil {
		return r, 400, "Invalid user request JSON"
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return r, 0, ""
}
func isUserBudgetModelList(path string) bool {
	return path == "/v1/models" || path == "/v1beta/models" || strings.HasPrefix(path, "/gemini/") && strings.HasSuffix(path, "/models")
}

// Input estimate uses serialized UTF-8 bytes plus an explicit output ceiling,
// not bytes/4. It is conservative for ordinary tokenizers, not a guarantee
// about third-party usage. Opaque continuations and media fail closed.
func userBudgetTokenEstimate(path string, body map[string]any) (int64, error) {
	if !userBudgetTextValue(body) {
		return 0, store.ErrGatewayUserInvalid
	}
	output := int64(0)
	field := ""
	switch path {
	case "/v1/chat/completions", "/chat/completions":
		field = "max_completion_tokens"
		if _, ok := body["max_tokens"]; ok {
			field = "max_tokens"
		}
	case "/v1/completions", "/v1/messages":
		field = "max_tokens"
	case "/v1/responses", "/responses":
		field = "max_output_tokens"
	case "/v1/embeddings":
	default:
		if strings.Contains(path, ":generateContent") || strings.Contains(path, ":streamGenerateContent") {
			config, ok := body["generationConfig"].(map[string]any)
			if !ok {
				config = map[string]any{}
				body["generationConfig"] = config
			}
			var err error
			output, err = userBudgetOutputLimit(config, "maxOutputTokens")
			if err != nil {
				return 0, err
			}
		} else {
			return 0, store.ErrGatewayUserInvalid
		}
	}
	if field != "" {
		var err error
		output, err = userBudgetOutputLimit(body, field)
		if err != nil {
			return 0, err
		}
		if path == "/v1/chat/completions" || path == "/chat/completions" {
			for _, key := range []string{"max_tokens", "max_completion_tokens"} {
				if value, ok := body[key]; ok {
					n, valid := userBudgetPositiveInteger(value)
					if !valid {
						return 0, store.ErrGatewayUserInvalid
					}
					if n > output {
						output = n
					}
				}
			}
			if stream, _ := body["stream"].(bool); stream {
				options, ok := body["stream_options"].(map[string]any)
				if !ok {
					options = map[string]any{}
				}
				options["include_usage"] = true
				body["stream_options"] = options
			}
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	return int64(len(raw))*2 + output + 1024, nil
}
func userBudgetOutputLimit(body map[string]any, field string) (int64, error) {
	if value, ok := body[field]; ok {
		n, valid := userBudgetPositiveInteger(value)
		if !valid {
			return 0, store.ErrGatewayUserInvalid
		}
		return n, nil
	}
	body[field] = float64(4096)
	return 4096, nil
}
func userBudgetPositiveInteger(value any) (int64, bool) {
	n, ok := value.(float64)
	if !ok || n <= 0 || n > 16_777_216 || math.IsNaN(n) || math.Trunc(n) != n {
		return 0, false
	}
	return int64(n), true
}
func userBudgetTextValue(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, value := range v {
			switch key {
			case "image_url", "input_image", "input_audio", "audio", "video", "file_id", "file_data", "attachments", "inlineData", "inline_data", "fileData", "file_data_uri", "previous_response_id", "conversation", "cachedContent", "cached_content":
				if value != nil && value != "" {
					return false
				}
			case "background":
				if enabled, _ := value.(bool); enabled {
					return false
				}
			case "n", "best_of":
				if n, ok := value.(float64); !ok || n != 1 {
					return false
				}
			case "type":
				if kind, ok := value.(string); ok {
					switch kind {
					case "message", "text", "input_text", "output_text", "function", "function_call", "function_call_output", "tool_use", "tool_result", "thinking", "redacted_thinking", "json_schema", "json_object", "string", "number", "integer", "boolean", "object", "array", "null":
					default:
						return false
					}
				}
			}
			if !userBudgetTextValue(value) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !userBudgetTextValue(item) {
				return false
			}
		}
	}
	return true
}
func (s *requestLifecycle) admitUserBudget(ctx context.Context, pac *auth.ProxyAuthContext, path string, body map[string]any) (int, string) {
	if pac == nil || pac.OwnerUserID == nil {
		return 0, ""
	}
	if s == nil || pac.UserGroupID == nil {
		return 503, "User budget admission unavailable"
	}
	s.mu.Lock()
	already := s.userBudget != nil && s.userBudget.reserved
	s.mu.Unlock()
	if already {
		return 0, ""
	}
	estimate, err := userBudgetTokenEstimate(path, body)
	if err != nil {
		return 403, "User token budgets support bounded text requests only; media, asynchronous or opaque continuation requests are unavailable"
	}
	ledger := store.NewGatewayUserStore(store.GetDB())
	releaseActive := store.TrackUserBudgetRequest(s.entry.ID)
	if _, err = ledger.ReserveUserBudget(ctx, s.entry.ID, *pac.OwnerUserID, *pac.UserGroupID, estimate); err != nil {
		releaseActive()
		switch {
		case errors.Is(err, store.ErrUserBudgetExceeded):
			return 429, "User token budget exhausted or reserved by in-flight requests"
		case errors.Is(err, store.ErrUserBudgetUnknown):
			return 403, "User budget is frozen because previous usage is unresolved"
		default:
			return 503, "User budget admission unavailable"
		}
	}
	s.mu.Lock()
	s.userBudget = &requestUserBudget{ledger: ledger, reserved: true, seenAttempts: map[int]bool{}, releaseActive: releaseActive}
	s.mu.Unlock()
	return 0, ""
}

func (s *requestLifecycle) observeUserBudgetUsageValidity(usage ParsedUsage) {
	if s == nil || !usage.invalidForUserBudget {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userBudget != nil {
		s.userBudget.usageUnknown = true
	}
}
func (s *requestLifecycle) settleUserBudget(ctx context.Context) {
	s.mu.Lock()
	budget := s.userBudget
	if budget == nil || !budget.reserved {
		s.mu.Unlock()
		return
	}
	var tokens *int64
	if budget.usageFound && !budget.usageUnknown && len(budget.seenAttempts) >= s.attempts {
		value := budget.tokens
		tokens = &value
	}
	dispatched := s.attempted
	id := s.entry.ID
	s.mu.Unlock()
	if budget.releaseActive != nil {
		defer budget.releaseActive()
	}
	if err := budget.ledger.SettleUserBudget(ctx, id, tokens, dispatched); err != nil {
		slog.Warn("user budget settlement failed", "gateway_request_id", id, "err", err)
	}
}
