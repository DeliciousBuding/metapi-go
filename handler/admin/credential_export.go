package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/trustedproxy"
)

const credentialExportFormatVersion = "1.0.0"

func (h *downstreamKeysHandler) exportKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
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
	profiles := buildCredentialExportProfiles(name, key, baseURL)
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
		"profiles":      profiles,
		"notes": []string{
			"Export only includes the key already visible to this admin session",
			"formatVersion is semver for adapter schema evolution",
		},
	})
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

func buildCredentialExportProfiles(name, key, baseURL string) []map[string]any {
	v1 := strings.TrimRight(baseURL, "/") + "/v1"
	env := "export OPENAI_BASE_URL=" + strconv.Quote(v1) + "\nexport OPENAI_API_KEY=" + strconv.Quote(key) + "\n"
	claudeSettings := "// Save to ~/.claude/settings.json\n" +
		"{\n" +
		"  \"env\": {\n" +
		"    \"ANTHROPIC_BASE_URL\": " + strconv.Quote(v1) + ",\n" +
		"    \"ANTHROPIC_AUTH_TOKEN\": " + strconv.Quote(key) + "\n" +
		"  }\n" +
		"}\n"
	codexConfig := "# Save to ~/.codex/config.toml\n" +
		"model_provider = \"metapi\"\n" +
		"\n" +
		"[model_providers.metapi]\n" +
		"base_url = " + strconv.Quote(v1) + "\n" +
		"api_key = " + strconv.Quote(key) + "\n"
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
			"contentType": "text/plain",
			"content":     claudeSettings,
		},
		{
			"id":          "codex",
			"label":       "OpenAI Codex CLI",
			"description": "model_providers.metapi block for ~/.codex/config.toml",
			"contentType": "text/plain",
			"content":     codexConfig,
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
