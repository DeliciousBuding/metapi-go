package proxyhandler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
	"github.com/google/uuid"
)

type directProviderWire struct {
	Body            []byte
	Headers         http.Header
	Query           url.Values
	ForceStream     bool
	StripToolPrefix bool
	Profile         string
}
type directProviderWireKey struct{}

func withDirectProviderWire(ctx context.Context, wire *directProviderWire) context.Context {
	return context.WithValue(ctx, directProviderWireKey{}, wire)
}
func directProviderWireFromContext(ctx context.Context) *directProviderWire {
	wire, _ := ctx.Value(directProviderWireKey{}).(*directProviderWire)
	return wire
}

// prepareDirectProviderWire runs after the selected protocol conversion and
// parameter overrides. The caller must attach the returned context to the
// attempt and honor ForceStream independently of the downstream stream mode.
func prepareDirectProviderWire(endpoint *store.DirectEndpoint, channelID int64, credential *oauth.DirectCredentialResult, body []byte, downstream http.Header) (*directProviderWire, error) {
	wire := &directProviderWire{Body: body, Headers: make(http.Header), Query: make(url.Values)}
	if endpoint == nil || endpoint.Profile == "" {
		return wire, nil
	}
	wire.Profile = endpoint.Profile
	switch endpoint.Profile {
	case "deepseek", "zai":
		var err error
		wire.Body, err = prepareDomesticChatRequest(body, endpoint.Profile)
		if err != nil {
			return nil, err
		}
	case "codex":
		var err error
		wire.Body, err = responses.PrepareCodexRequest(body)
		if err != nil {
			return nil, err
		}
		wire.ForceStream = true
		buildDirectCodexHeaders(wire.Headers, downstream, credential, endpoint.URL)
	case "claudecode":
		cli := strings.HasPrefix(downstream.Get("User-Agent"), "claude-cli/")
		wire.StripToolPrefix = credential != nil && credential.Kind == store.DirectCredentialOAuth && !cli
		userID := directClaudeUserID(body, channelID, downstream)
		var err error
		wire.Body, err = messages.PrepareClaudeCodeRequest(body, userID, wire.StripToolPrefix)
		if err != nil {
			return nil, err
		}
		buildDirectClaudeHeaders(wire.Headers, downstream, cli)
		wire.Query.Set("beta", "true")
	default:
		return nil, fmt.Errorf("unsupported direct provider profile")
	}
	return wire, nil
}

// applyDirectProviderHeaders runs after generic request header construction.
// No authorization values are sourced from downstream/provider metadata.
func applyDirectProviderHeaders(req *http.Request, wire *directProviderWire) {
	if wire == nil {
		return
	}
	if wire.Profile == "codex" {
		req.Header.Del("X-Openai-Internal-Codex-Responses-Lite")
		req.Header.Del("Session_id")
	}
	for name, values := range wire.Headers {
		req.Header[name] = append([]string(nil), values...)
	}
	query := req.URL.Query()
	for name, values := range wire.Query {
		query[name] = append([]string(nil), values...)
	}
	req.URL.RawQuery = query.Encode()
	if wire.Profile == "codex" && wire.Headers.Get("Chatgpt-Account-Id") == "" {
		req.Header.Del("Chatgpt-Account-Id")
	}
}

func restoreDirectProviderResponse(ctx context.Context, body []byte) []byte {
	if wire := directProviderWireFromContext(ctx); wire != nil && wire.Profile == "claudecode" {
		return messages.RestoreClaudeCodeTools(body, wire.StripToolPrefix)
	}
	return body
}

func buildDirectCodexHeaders(headers, downstream http.Header, credential *oauth.DirectCredentialResult, endpoint string) {
	for _, name := range []string{"User-Agent", "Originator", "X-Codex-Turn-Metadata", "X-Codex-Turn-State", "X-Codex-Window-Id", "X-Client-Request-Id", "X-Codex-Beta-Features", "Thread-Id"} {
		if value := downstream.Get(name); value != "" {
			headers.Set(name, value)
		}
	}
	if headers.Get("Originator") == "" {
		headers.Set("Originator", "codex_cli_rs")
	}
	accountID := ""
	if credential != nil {
		accountID = credential.AccountID
	}
	if accountID != "" {
		headers.Set("Chatgpt-Account-Id", accountID)
	}
	sessionID := downstream.Get("Session-Id")
	if sessionID == "" {
		sessionID = downstream.Get("Session_id")
	}
	if sessionID == "" {
		var metadata struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal([]byte(headers.Get("X-Codex-Turn-Metadata")), &metadata)
		sessionID = metadata.SessionID
	}
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	headers.Set("Session-Id", sessionID)
	headers.Set("Conversation_id", sessionID)
	defaults := map[string]string{"Thread-Id": sessionID, "X-Codex-Window-Id": sessionID + ":0", "X-Client-Request-Id": uuid.NewString(), "X-Codex-Beta-Features": "remote_compaction_v2", "Version": "0.159.0"}
	for name, value := range defaults {
		if headers.Get(name) == "" {
			headers.Set(name, value)
		}
	}
	if headers.Get("X-Codex-Turn-Metadata") == "" {
		installation := ""
		if accountID != "" {
			installation = uuid.NewSHA1(uuid.NameSpaceOID, []byte(accountID)).String()
		}
		metadata, _ := json.Marshal(map[string]any{"installation_id": installation, "session_id": sessionID, "thread_id": sessionID, "turn_id": uuid.NewString(), "window_id": sessionID + ":0", "request_kind": "turn", "thread_source": "user", "sandbox": "none", "turn_started_at_unix_ms": time.Now().UnixMilli()})
		headers.Set("X-Codex-Turn-Metadata", string(metadata))
	}
	parsed, _ := url.Parse(endpoint)
	if parsed != nil && (parsed.Hostname() == "chatgpt.com" || strings.HasSuffix(parsed.Hostname(), ".chatgpt.com")) && strings.EqualFold(downstream.Get("X-Openai-Internal-Codex-Responses-Lite"), "true") {
		headers.Set("X-Openai-Internal-Codex-Responses-Lite", "true")
	}
	headers.Set("Accept", "text/event-stream")
}

const directClaudeBetas = "claude-code-20250219,context-1m-2025-08-07,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,context-management-2025-06-27,prompt-caching-scope-2026-01-05,mid-conversation-system-2026-04-07,effort-2025-11-24"

func buildDirectClaudeHeaders(headers, downstream http.Header, cli bool) {
	defaults := map[string]string{"Anthropic-Beta": directClaudeBetas, "Anthropic-Version": "2023-06-01", "Anthropic-Dangerous-Direct-Browser-Access": "true", "X-App": "cli", "X-Stainless-Helper-Method": "stream", "X-Stainless-Retry-Count": "0", "X-Stainless-Runtime-Version": "v24.3.0", "X-Stainless-Package-Version": "0.94.0", "X-Stainless-Runtime": "node", "X-Stainless-Lang": "js", "X-Stainless-Arch": "arm64", "X-Stainless-Os": "MacOS", "X-Stainless-Timeout": "600"}
	for name, value := range defaults {
		if incoming := downstream.Get(name); incoming != "" && name != "Anthropic-Beta" {
			value = incoming
		}
		headers.Set(name, value)
	}
	seen := map[string]bool{}
	betas := []string{}
	for _, raw := range []string{directClaudeBetas, downstream.Get("Anthropic-Beta")} {
		for _, beta := range strings.Split(raw, ",") {
			beta = strings.TrimSpace(beta)
			if beta != "" && !seen[beta] {
				betas = append(betas, beta)
				seen[beta] = true
			}
		}
	}
	headers.Set("Anthropic-Beta", strings.Join(betas, ","))
	if session := downstream.Get("X-Claude-Code-Session-Id"); session != "" {
		headers.Set("X-Claude-Code-Session-Id", session)
	}
	ua := "claude-cli/2.1.170 (external, cli)"
	if cli {
		ua = downstream.Get("User-Agent")
	}
	headers.Set("User-Agent", ua)
}

var directClaudeLegacyUserID = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func directClaudeUserID(body []byte, channelID int64, headers http.Header) string {
	var payload struct {
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(body, &payload)
	existing := payload.Metadata.UserID
	var parsed struct {
		SessionID string `json:"session_id"`
	}
	if (json.Unmarshal([]byte(existing), &parsed) == nil && parsed.SessionID != "") || directClaudeLegacyUserID.MatchString(existing) {
		return existing
	}
	identity := fmt.Sprint(channelID)
	digest := sha256.Sum256([]byte(identity))
	session := headers.Get("X-Claude-Code-Session-Id")
	if session == "" {
		session = headers.Get("Session-Id")
	}
	if session == "" {
		session = uuid.NewString()
	}
	data, _ := json.Marshal(map[string]string{"device_id": hex.EncodeToString(digest[:]), "account_uuid": uuid.NewSHA1(uuid.NameSpaceURL, []byte(identity)).String(), "session_id": session})
	return string(data)
}
