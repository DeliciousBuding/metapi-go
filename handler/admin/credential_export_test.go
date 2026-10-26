package admin

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/trustedproxy"
	"github.com/go-chi/chi/v5"
)

func TestBuildCredentialExportProfiles_Content(t *testing.T) {
	ps := buildCredentialExportProfiles("n", "sk-abc", "https://gw.test", "")
	if len(ps) != 6 {
		t.Fatalf("len=%d", len(ps))
	}
	if ps[0]["id"] != "openai" {
		t.Fatalf("%v", ps[0])
	}
	content, _ := ps[0]["content"].(string)
	if !strings.Contains(content, "OPENAI_BASE_URL") || !strings.Contains(content, "sk-abc") {
		t.Fatalf("content=%q", content)
	}
	cherry, _ := ps[1]["content"].(map[string]any)
	if cherry["apiKey"] != "sk-abc" {
		t.Fatalf("cherry=%v", cherry)
	}
	claude, _ := ps[3]["content"].(string)
	if ps[3]["id"] != "claude-code" ||
		!strings.Contains(claude, "ANTHROPIC_BASE_URL") ||
		!strings.Contains(claude, "ANTHROPIC_AUTH_TOKEN") ||
		!strings.Contains(claude, "sk-abc") ||
		!strings.Contains(claude, `"ANTHROPIC_BASE_URL": "https://gw.test"`) {
		t.Fatalf("claude=%q", claude)
	}
	codex, _ := ps[4]["content"].(string)
	if ps[4]["id"] != "codex" ||
		!strings.Contains(codex, `model_provider = "metapi"`) ||
		!strings.Contains(codex, "[model_providers.metapi]") ||
		!strings.Contains(codex, "base_url") ||
		strings.Contains(codex, "sk-abc") ||
		!strings.Contains(codex, `env_key = "METAPI_API_KEY"`) {
		t.Fatalf("codex=%q", codex)
	}
	webui, _ := ps[5]["content"].(map[string]any)
	if ps[5]["id"] != "openwebui" ||
		webui["baseUrl"] != "https://gw.test/v1" ||
		webui["apiKey"] != "sk-abc" {
		t.Fatalf("openwebui=%v", webui)
	}
}

func TestCredentialExportClientContracts(t *testing.T) {
	profiles := buildCredentialExportProfiles("Fixture", "synthetic-export-key", "https://gateway.example/prefix", "")
	claude := profiles[3]["content"].(string)
	var settings struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal([]byte(claude), &settings); err != nil {
		t.Errorf("Claude settings must be valid JSON: %v", err)
	} else if settings.Env["ANTHROPIC_BASE_URL"] != "https://gateway.example/prefix" {
		t.Errorf("Claude appends /v1/messages itself; got base %q", settings.Env["ANTHROPIC_BASE_URL"])
	}
	codex := profiles[4]["content"].(string)
	for _, required := range []string{`name = "MetAPI"`, `env_key = "METAPI_API_KEY"`, `wire_api = "responses"`} {
		if !strings.Contains(codex, required) {
			t.Errorf("Codex profile is missing %s", required)
		}
	}
	if strings.Contains(codex, "synthetic-export-key") || strings.Contains(codex, "\napi_key =") {
		t.Error("Codex provider config must use its documented environment-key field")
	}
}

func TestCredentialExportShellPreservesLiteralValues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell profile")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell unavailable")
	}
	// Harmless substitutions must remain literal in a pasted export block.
	key := "fixture'$(printf substituted)`printf also-substituted`\\literal"
	profiles := buildCredentialExportProfiles("Fixture", key, "https://gateway.example/path", "")
	content := profiles[0]["content"].(string)
	cmd := exec.Command("sh", "-c", content+`printf '%s' "$OPENAI_API_KEY"`)
	got, err := cmd.Output()
	if err != nil || string(got) != key {
		t.Fatalf("export must preserve the literal key; got %q, err=%v", got, err)
	}
	setup := profiles[4]["setup"].(map[string]string)["content"]
	cmd = exec.Command("sh", "-c", setup+`printf '%s' "$METAPI_API_KEY"`)
	got, err = cmd.Output()
	if err != nil || string(got) != key {
		t.Fatalf("Codex setup must preserve the literal key; got %q, err=%v", got, err)
	}
}

func TestCredentialExportSelectedModel(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "sqlite"
		if postgres {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			var r chi.Router
			if postgres {
				_, r = setupDownstreamKeysPostgresTest(t)
			} else {
				_, r = setupDownstreamKeysTest(t)
			}
			created := doPostJSON(t, r, "/api/downstream-keys", map[string]any{"name": "Fixture export", "key": "sk-fixture"})
			if created.Code != http.StatusOK && created.Code != http.StatusCreated {
				t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
			}
			var entity struct {
				Item struct {
					ID int `json:"id"`
				} `json:"item"`
			}
			if err := json.Unmarshal(created.Body.Bytes(), &entity); err != nil || entity.Item.ID == 0 {
				t.Fatalf("create response=%s err=%v", created.Body.String(), err)
			}
			model := "alias/模型\"\\selected"
			path := "/api/downstream-keys/" + strconv.Itoa(entity.Item.ID) + "/export?model=" + url.QueryEscape(model)
			response := doGet(t, r, path)
			if response.Code != http.StatusOK {
				t.Fatalf("export status=%d body=%s", response.Code, response.Body.String())
			}
			var data struct {
				FormatVersion string `json:"formatVersion"`
				Model         string `json:"model"`
				Profiles      []struct {
					ID      string          `json:"id"`
					Content json.RawMessage `json:"content"`
				} `json:"profiles"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil || data.Model != model || data.FormatVersion != "1.1.0" || len(data.Profiles) != 6 {
				t.Fatalf("export contract=%+v err=%v", data, err)
			}
			for _, profile := range data.Profiles {
				if profile.ID != "claude-code" && profile.ID != "codex" {
					continue
				}
				var content string
				if err := json.Unmarshal(profile.Content, &content); err != nil {
					t.Fatal(err)
				}
				if profile.ID == "claude-code" {
					var settings struct{ Model string }
					if err := json.Unmarshal([]byte(content), &settings); err != nil || settings.Model != model {
						t.Fatalf("Claude selected model changed: %q err=%v", settings.Model, err)
					}
				} else {
					// This line is a TOML basic string, whose supported Unicode and
					// quote escapes also decode through strconv.Unquote.
					line := strings.Split(content, "\n")[1]
					decoded, err := strconv.Unquote(strings.TrimPrefix(line, "model = "))
					if err != nil || decoded != model {
						t.Fatalf("Codex selected model changed: %q err=%v", decoded, err)
					}
				}
			}
			for _, query := range []string{"model=", "model=a&model=b", "model=bad%0Aname", "model=re%3A.*", "model=star*", "model=" + strings.Repeat("a", 201)} {
				response = doGet(t, r, "/api/downstream-keys/"+strconv.Itoa(entity.Item.ID)+"/export?"+query)
				if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "sk-fixture") {
					t.Fatalf("invalid model query returned %d or exposed key: %s", response.Code, query)
				}
			}
		})
	}
}

func TestResolveExportBaseURL(t *testing.T) {
	rt := &config.RuntimeSettings{}
	config.SetRuntime(rt)
	t.Cleanup(func() { config.SetRuntime(nil) })

	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "http://internal.metapi:4000/api/downstream-keys/1/export", nil)
		return req
	}

	t.Run("direct client host wins and forwarding headers are ignored", func(t *testing.T) {
		req := newRequest()
		req.Host = "api.example.com"
		req.Header.Set("X-Forwarded-Host", "evil.example")
		req.Header.Set("X-Forwarded-Proto", "https")
		if got := resolveExportBaseURL(req); got != "http://api.example.com" {
			t.Fatalf("baseURL = %q, want the request Host with direct-client scheme", got)
		}
	})

	t.Run("direct TLS uses https", func(t *testing.T) {
		req := newRequest()
		req.Host = "api.example.com"
		req.TLS = &tls.ConnectionState{}
		if got := resolveExportBaseURL(req); got != "https://api.example.com" {
			t.Fatalf("baseURL = %q", got)
		}
	})

	t.Run("trusted proxy proto is honored but Host still wins over X-Forwarded-Host", func(t *testing.T) {
		req := newRequest()
		req.Host = "api.example.com"
		req.Header.Set("X-Forwarded-Host", "evil.example")
		req.Header.Set("X-Forwarded-Proto", "https")
		req = req.WithContext(trustedproxy.WithTrust(req.Context()))
		if got := resolveExportBaseURL(req); got != "https://api.example.com" {
			t.Fatalf("baseURL = %q, want Host with trusted https", got)
		}
	})

	t.Run("trusted proxy may supply the host when Host was rewritten empty", func(t *testing.T) {
		req := newRequest()
		req.Host = ""
		req.Header.Set("X-Forwarded-Host", "gw.example.com")
		req.Header.Set("X-Forwarded-Proto", "https")
		req = req.WithContext(trustedproxy.WithTrust(req.Context()))
		if got := resolveExportBaseURL(req); got != "https://gw.example.com" {
			t.Fatalf("baseURL = %q, want trusted X-Forwarded-Host", got)
		}
	})

	t.Run("untrusted X-Forwarded-Host is ignored", func(t *testing.T) {
		req := newRequest()
		req.Host = ""
		req.Header.Set("X-Forwarded-Host", "evil.example")
		if got := resolveExportBaseURL(req); got != "http://localhost:4000" {
			t.Fatalf("baseURL = %q, want the localhost fallback", got)
		}
	})

	t.Run("malformed Host cannot smuggle a path", func(t *testing.T) {
		req := newRequest()
		req.Host = "evil.example/path"
		if got := resolveExportBaseURL(req); got != "http://localhost:4000" {
			t.Fatalf("baseURL = %q, want fallback for malformed Host", got)
		}
	})

	t.Run("SERVER_ADDRESS overrides request-derived values", func(t *testing.T) {
		config.SetRuntime(&config.RuntimeSettings{ServerAddress: "https://public.example/"})
		req := newRequest()
		req.Host = "api.example.com"
		if got := resolveExportBaseURL(req); got != "https://public.example" {
			t.Fatalf("baseURL = %q, want SERVER_ADDRESS", got)
		}
	})
}
