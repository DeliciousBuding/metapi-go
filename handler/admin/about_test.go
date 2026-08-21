package admin

import (
	"encoding/json"
	"net/http"
	"runtime"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/version"
	"github.com/go-chi/chi/v5"
)

func setupAboutTest(t *testing.T) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	RegisterAboutRoutes(r)
	return r
}

// decodeAboutBody asserts the endpoint answered 200 and decodes the payload as
// a string map — the About page reads every field as a string.
func decodeAboutBody(t *testing.T, r chi.Router) map[string]string {
	t.Helper()
	resp := doGet(t, r, "/api/about")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", resp.Code, resp.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, resp.Body.String())
	}
	return body
}

// setInjectedBuildMetadata simulates an ldflags-injected binary and restores
// the package defaults afterwards so sibling tests still see the real values.
func setInjectedBuildMetadata(t *testing.T, commit, buildTime string) {
	t.Helper()
	originalCommit := version.Commit
	originalBuildTime := version.BuildTime
	t.Cleanup(func() {
		version.Commit = originalCommit
		version.BuildTime = originalBuildTime
	})
	version.Commit = commit
	version.BuildTime = buildTime
}

func TestAboutInfo_ReturnsAllCamelCaseFields(t *testing.T) {
	body := decodeAboutBody(t, setupAboutTest(t))

	for _, field := range []string{"version", "commit", "buildTime", "goVersion"} {
		if _, ok := body[field]; !ok {
			t.Fatalf("missing field %q in %v", field, body)
		}
	}
	if len(body) != 4 {
		t.Fatalf("payload=%v, want exactly the four about fields", body)
	}
}

func TestAboutInfo_GoVersionComesFromRuntime(t *testing.T) {
	body := decodeAboutBody(t, setupAboutTest(t))

	if body["goVersion"] != runtime.Version() {
		t.Fatalf("goVersion=%q, want runtime.Version()=%q", body["goVersion"], runtime.Version())
	}
}

func TestAboutInfo_VersionMirrorsInjectedBinaryVersion(t *testing.T) {
	body := decodeAboutBody(t, setupAboutTest(t))

	if body["version"] != version.Version {
		t.Fatalf("version=%q, want %q", body["version"], version.Version)
	}
}

// Uninjected builds must not fabricate provenance: empty stays empty on the
// wire so the About page renders an em-dash instead of a fake SHA/timestamp.
func TestAboutInfo_UninjectedProvenanceStaysEmpty(t *testing.T) {
	setInjectedBuildMetadata(t, "", "")

	body := decodeAboutBody(t, setupAboutTest(t))

	if body["commit"] != "" {
		t.Fatalf("commit=%q, want empty for an uninjected build", body["commit"])
	}
	if body["buildTime"] != "" {
		t.Fatalf("buildTime=%q, want empty for an uninjected build", body["buildTime"])
	}
}

func TestAboutInfo_InjectedProvenancePassesThrough(t *testing.T) {
	setInjectedBuildMetadata(t, "0123456789abcdef0123456789abcdef01234567", "2026-08-21T10:11:12Z")

	body := decodeAboutBody(t, setupAboutTest(t))

	if body["commit"] != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("commit=%q, want the injected SHA", body["commit"])
	}
	if body["buildTime"] != "2026-08-21T10:11:12Z" {
		t.Fatalf("buildTime=%q, want the injected timestamp", body["buildTime"])
	}
}
