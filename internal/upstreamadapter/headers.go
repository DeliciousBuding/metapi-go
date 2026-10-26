package upstreamadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

func buildDirectCodexHeaders(headers, downstream http.Header, accountID string, endpoint string, sessionNamespace string) {
	for _, name := range []string{"User-Agent", "Originator", "X-Codex-Turn-Metadata", "X-Codex-Turn-State", "X-Codex-Window-Id", "X-Client-Request-Id", "X-Codex-Beta-Features", "Thread-Id"} {
		if value := downstream.Get(name); value != "" {
			headers.Set(name, value)
		}
	}
	if headers.Get("Originator") == "" {
		headers.Set("Originator", "codex_cli_rs")
	}
	if accountID != "" {
		headers.Set("Chatgpt-Account-Id", accountID)
	}
	sessionID := strings.TrimSpace(downstream.Get("Session-Id"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(downstream.Get("Session_id"))
	}
	if sessionID == "" {
		var metadata struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal([]byte(headers.Get("X-Codex-Turn-Metadata")), &metadata)
		sessionID = strings.TrimSpace(metadata.SessionID)
	}
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	// Namespace the session identity per downstream key: shared OAuth upstream
	// accounts must not let one key resume another key's session (audit L28).
	rawSessionID := sessionID
	sessionID = NamespaceSessionID(sessionNamespace, sessionID)
	headers.Set("Session-Id", sessionID)
	headers.Set("Conversation_id", sessionID)
	namespaceCodexSessionHeaders(headers, rawSessionID, sessionID)
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

func buildDirectClaudeHeaders(headers, downstream http.Header, cli bool, sessionNamespace string) {
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
	if session := strings.TrimSpace(downstream.Get("X-Claude-Code-Session-Id")); session != "" {
		headers.Set("X-Claude-Code-Session-Id", NamespaceSessionID(sessionNamespace, session))
	}
	ua := "claude-cli/2.1.170 (external, cli)"
	if cli {
		ua = downstream.Get("User-Agent")
	}
	headers.Set("User-Agent", ua)
}

func buildDirectCopilotHeaders(headers, downstream http.Header) {
	defaults := map[string]string{
		"Editor-Version":                      "vscode/1.95.0",
		"Editor-Plugin-Version":               "copilot-chat/0.26.7",
		"User-Agent":                          "GitHubCopilotChat/0.26.7",
		"Copilot-Integration-Id":              "vscode-chat",
		"Openai-Intent":                       "conversation-edits",
		"X-GitHub-Api-Version":                "2025-04-01",
		"X-Vscode-User-Agent-Library-Version": "electron-fetch",
		"X-Initiator":                         "user",
	}
	for name, value := range defaults {
		if incoming := downstream.Get(name); incoming != "" {
			value = incoming
		}
		headers.Set(name, value)
	}
	if initiator := strings.ToLower(strings.TrimSpace(headers.Get("X-Initiator"))); initiator != "user" && initiator != "agent" {
		headers.Set("X-Initiator", "user")
	}
}

func buildDirectXAISubscriptionHeaders(headers http.Header) {
	headers.Set("X-Xai-Token-Auth", "xai-grok-cli")
	headers.Set("X-Grok-Client-Version", "0.2.114")
	headers.Set("X-Grok-Client-Identifier", "grok-shell")
	headers.Set("User-Agent", "xai-grok-workspace/0.2.114")
}

var directClaudeLegacyUserID = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func directClaudeUserID(body []byte, channelID int64, headers http.Header, sessionNamespace string) string {
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
	session := strings.TrimSpace(headers.Get("X-Claude-Code-Session-Id"))
	if session == "" {
		session = strings.TrimSpace(headers.Get("Session-Id"))
	}
	if session == "" {
		session = uuid.NewString()
	}
	session = NamespaceSessionID(sessionNamespace, session)
	data, _ := json.Marshal(map[string]string{"device_id": hex.EncodeToString(digest[:]), "account_uuid": uuid.NewSHA1(uuid.NameSpaceURL, []byte(identity)).String(), "session_id": session})
	return string(data)
}

// namespaceCodexSessionHeaders rewrites session-derived headers that were
// copied from the client before the namespaced Session-Id replaced them. It
// only touches values that still carry the raw client session id, so
// independent thread/window values pass through unchanged.
func namespaceCodexSessionHeaders(headers http.Header, rawSession, namespacedSession string) {
	if rawSession == "" || rawSession == namespacedSession {
		return
	}
	if headers.Get("Thread-Id") == rawSession {
		headers.Set("Thread-Id", namespacedSession)
	}
	if window := headers.Get("X-Codex-Window-Id"); window == rawSession || strings.HasPrefix(window, rawSession+":") {
		headers.Set("X-Codex-Window-Id", namespacedSession+strings.TrimPrefix(window, rawSession))
	}
	rawMetadata := headers.Get("X-Codex-Turn-Metadata")
	if rawMetadata == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil {
		return
	}
	changed := false
	for _, key := range []string{"session_id", "thread_id"} {
		if value, ok := metadata[key].(string); ok && value == rawSession {
			metadata[key] = namespacedSession
			changed = true
		}
	}
	if value, ok := metadata["window_id"].(string); ok && (value == rawSession || strings.HasPrefix(value, rawSession+":")) {
		metadata["window_id"] = namespacedSession + strings.TrimPrefix(value, rawSession)
		changed = true
	}
	if !changed {
		return
	}
	if encoded, err := json.Marshal(metadata); err == nil {
		headers.Set("X-Codex-Turn-Metadata", string(encoded))
	}
}

// NamespaceSessionID maps a client-supplied session id into the downstream
// key's namespace. The result is UUID-shaped so upstream headers that expect a
// UUID keep their format and length. Empty namespace/session values pass
// through unchanged for callers without authentication context.
func NamespaceSessionID(sessionNamespace, session string) string {
	session = strings.TrimSpace(session)
	if sessionNamespace == "" || session == "" {
		return session
	}
	digest := sha256.Sum256([]byte("metapi-downstream-session\x00" + sessionNamespace + "\x00" + session))
	raw := digest[:16]
	raw[6] = (raw[6] & 0x0f) | 0x50
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}
