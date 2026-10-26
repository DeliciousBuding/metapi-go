package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/trustedproxy"
	"github.com/deliciousbuding/metapi-go/routing"
)

const credentialExportFormatVersion = "1.1.0"

func (h *downstreamKeysHandler) exportKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	model, ok := credentialExportModel(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "model must be one non-empty public model name of at most 200 characters")
		return
	}
	row, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load API key")
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}
	key, _ := row["key"].(string)
	name, _ := row["name"].(string)
	if key == "" {
		writeError(w, http.StatusInternalServerError, "key is missing")
		return
	}
	baseURL := resolveExportBaseURL(r)
	profile := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("profile")))
	if profile == "" {
		profile = "all"
	}
	profiles := buildCredentialExportProfiles(name, key, baseURL, model)
	if profile != "all" {
		filtered := make([]map[string]any, 0, 1)
		for _, p := range profiles {
			if p["id"] == profile {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) == 0 {
			writeError(w, http.StatusBadRequest, "unknown profile: "+profile)
			return
		}
		profiles = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"formatVersion": credentialExportFormatVersion,
		"keyId":         id,
		"keyName":       name,
		"baseUrl":       baseURL,
		"model":         model,
		"profiles":      profiles,
		"notes": []string{
			"Export only includes the key already visible to this admin session",
			"formatVersion is semver for adapter schema evolution",
		},
	})
}

// Exporting a configuration does not probe or authorize the selected model.
// The explicit connection check uses the key's actual data-plane admission.
func credentialExportModel(r *http.Request) (string, bool) {
	values, present := r.URL.Query()["model"]
	if !present {
		return "", true
	}
	if len(values) != 1 {
		return "", false
	}
	model := strings.TrimSpace(values[0])
	if !utf8.ValidString(model) || utf8.RuneCountInString(model) > 200 || !routing.IsExactRouteModelPattern(model) {
		return "", false
	}
	for _, char := range model {
		if unicode.IsControl(char) {
			return "", false
		}
	}
	return model, true
}

// resolveExportBaseURL builds the base URL written into an exported client
// profile. Precedence:
//
//  1. SERVER_ADDRESS — the operator's explicit public base URL.
//  2. The request Host (the URL the client actually called).
//  3. X-Forwarded-Host, only when router.TrustedRealIP marked the peer as a
//     configured trusted proxy and the value is a bare host[:port].
//
// X-Forwarded-Host is never taken from a direct client: it is
// attacker-controlled and would let a spoofed export point the operator's CLI
// at an arbitrary host (audit L27). Scheme follows trustedproxy.IsHTTPS, so a
// forged X-Forwarded-Proto cannot downgrade or upgrade it either.
func resolveExportBaseURL(r *http.Request) string {
	if rt := config.RuntimeSafe(); rt != nil {
		if configured := strings.TrimRight(strings.TrimSpace(rt.ServerAddress), "/"); configured != "" {
			return configured
		}
	}
	proto := "http"
	if trustedproxy.IsHTTPS(r) {
		proto = "https"
	}
	host := trustedproxy.ValidateHost(r.Host)
	if host == "" {
		host = trustedproxy.ForwardedHost(r)
	}
	if host == "" {
		return "http://localhost:4000"
	}
	return proto + "://" + host
}

func buildCredentialExportProfiles(name, key, baseURL, model string) []map[string]any {
	v1 := strings.TrimRight(baseURL, "/") + "/v1"
	env := "export OPENAI_BASE_URL=" + credentialExportShellQuote(v1) + "\nexport OPENAI_API_KEY=" + credentialExportShellQuote(key) + "\n"
	claude := map[string]any{"env": map[string]string{
		"ANTHROPIC_BASE_URL":   strings.TrimRight(baseURL, "/"),
		"ANTHROPIC_AUTH_TOKEN": key,
	}}
	if model != "" {
		claude["model"] = model
	}
	// Strings and string maps always encode; MarshalIndent produces a real
	// settings.json document rather than an instruction embedded as a comment.
	claudeSettings, _ := json.MarshalIndent(claude, "", "  ")
	codexConfig := "model_provider = \"metapi\"\n"
	if model != "" {
		codexConfig += "model = " + credentialExportTOMLQuote(model) + "\n"
	}
	codexConfig +=
		"\n" +
			"[model_providers.metapi]\n" +
			"name = \"MetAPI\"\n" +
			"base_url = " + credentialExportTOMLQuote(v1) + "\n" +
			"env_key = \"METAPI_API_KEY\"\n" +
			"wire_api = \"responses\"\n"
	return []map[string]any{
		{
			"id":          "openai",
			"label":       "OpenAI-compatible env",
			"description": "OPENAI_BASE_URL + OPENAI_API_KEY for CLI/SDKs",
			"contentType": "text/plain",
			"content":     env,
		},
		{
			"id":          "cherry",
			"label":       "Cherry Studio / generic OpenAI provider JSON",
			"description": "Provider block for tools that accept OpenAI-compatible base URL + key",
			"contentType": "application/json",
			"content": map[string]any{
				"name":    name,
				"type":    "openai",
				"apiHost": v1,
				"apiKey":  key,
			},
		},
		{
			"id":          "generic",
			"label":       "Generic JSON",
			"description": "Minimal portable credential object",
			"contentType": "application/json",
			"content": map[string]any{
				"baseUrl": baseURL,
				"apiBase": v1,
				"apiKey":  key,
				"headers": map[string]string{"Authorization": "Bearer " + key},
			},
		},
		{
			"id":          "claude-code",
			"label":       "Claude Code",
			"description": "env block for ~/.claude/settings.json (ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN)",
			"contentType": "application/json",
			"content":     string(claudeSettings) + "\n",
			"protocol":    "messages",
		},
		{
			"id":          "codex",
			"label":       "OpenAI Codex CLI",
			"description": "model_providers.metapi block for ~/.codex/config.toml",
			"contentType": "text/plain",
			"content":     codexConfig,
			"protocol":    "responses",
			"setup": map[string]string{
				"contentType": "text/plain",
				"content":     "export METAPI_API_KEY=" + credentialExportShellQuote(key) + "\n",
			},
		},
		{
			"id":          "openwebui",
			"label":       "Open WebUI",
			"description": "Add as an \"OpenAI API\" connection in Open WebUI Admin → Settings → Connections",
			"contentType": "application/json",
			"content": map[string]any{
				"baseUrl": v1,
				"apiKey":  key,
			},
		},
	}
}

// POSIX shell snippets must keep dollar signs, backticks and quotes literal.
// strconv.Quote is a Go/JSON encoding, not shell escaping.
func credentialExportShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// JSON strings use only escapes accepted by TOML basic strings. In contrast,
// Go string quoting may emit \x or \a escapes that TOML cannot parse.
func credentialExportTOMLQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
