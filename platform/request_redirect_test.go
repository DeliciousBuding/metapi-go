package platform

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestRequestScopedRedirectRestrictionKeepsLegacyClientBehavior(t *testing.T) {
	var targetCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			targetCalls.Add(1)
			w.WriteHeader(200)
			return
		}
		http.Redirect(w, r, "/target", http.StatusFound)
	}))
	defer server.Close()
	client := &http.Client{CheckRedirect: RejectCrossOriginRedirect}
	for _, restricted := range []bool{false, true, false} {
		ctx := context.Background()
		if restricted {
			ctx = WithoutRequestRedirects(ctx)
		}
		request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/start", nil)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		want := 200
		if restricted {
			want = 302
		}
		if response.StatusCode != want {
			t.Fatalf("request restriction mutated shared redirect behavior: %d", response.StatusCode)
		}
	}
	if targetCalls.Load() != 2 {
		t.Fatal("restricted request escaped its target or changed legacy controls")
	}
}
