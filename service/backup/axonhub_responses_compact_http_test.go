package backup_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAxonHubCompactImportHTTP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/prefix/responses/compact" || !strings.Contains(string(body), `"model":"provider-model"`) || !strings.Contains(string(body), "9007199254740993") {
			t.Errorf("compact wire changed: path=%s body=%s", r.URL.Path, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"cmp-fixture","object":"response.compaction","output":[{"type":"compaction","encrypted_content":"opaque-fixture"}],"usage":{"input_tokens":2,"output_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)
	router := installAxonHubHTTPFixture(t, "qiniu", upstream.URL+"/prefix", "openai/responses_compact", "/ignored-by-source-compact", "chat", nil)
	out := requestAxonHubNativeHTTP(router, "POST", "/v1/responses/compact", `{"model":"client-alias","input":[{"role":"user","content":"hello","opaque":9007199254740993}]}`)
	if out.Code != 200 || !strings.Contains(out.Body.String(), `"object":"response.compaction"`) {
		t.Fatalf("compact import not executable: %d %s", out.Code, out.Body.String())
	}
}
