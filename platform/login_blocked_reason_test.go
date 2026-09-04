package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A login answer that is not a JSON object used to be reported with one fixed
// sentence, "shield challenge blocked login", no matter what the site actually
// said. Observed against a real new-api: HTTP 429 on /api/user/login (its own
// access log, at exactly the moments Metapi reported a shield challenge) after
// the documented re-bind path was used a few times in a row. The message named a
// cause that did not exist and gave the operator nothing to do about the one
// that did.
//
// These tests pin the two halves of the fix: the observed status/content type
// decide the wording, and the upstream body is never echoed — a challenge or
// error page can carry markup, tokens or internal URLs, and a login failure
// message travels back to whoever called the admin API.

// loginAnswerServer answers /api/user/login with an exact status, content type
// and body, which is the whole input to the blocked-login message.
func loginAnswerServer(t *testing.T, status int, contentType, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/login" {
			http.NotFound(w, r)
			return
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// loginCapable is the shared Login shape of the two adapters that fetch a
// dashboard login through fetchLoginResponse.
type loginCapable interface {
	Login(ctx context.Context, baseURL, username, password string, platformUserId *int, proxy *ProxyConfig) (*LoginResult, error)
}

func TestLoginBlockedMessageReportsTheObservedUpstreamAnswer(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantAll     []string
		wantNone    []string
	}{
		{
			name:        "the site rate-limited the login",
			status:      http.StatusTooManyRequests,
			contentType: "text/plain; charset=utf-8",
			body:        "Too Many Requests",
			wantAll:     []string{"429", "rate-limit", "retry"},
			// A 429 must not be dressed up as anti-bot protection: that is the
			// exact misdiagnosis this fix exists to remove.
			wantNone: []string{"shield challenge blocked login", "WAF/shield challenge", "Too Many Requests"},
		},
		{
			name:        "a WAF/shield challenge page on HTTP 200",
			status:      http.StatusOK,
			contentType: "text/html",
			body:        "<html><script>var acw_sc__v2='challenge-secret-do-not-leak';</script></html>",
			wantAll:     []string{"200", "text/html", "WAF/shield challenge"},
			wantNone:    []string{"challenge-secret-do-not-leak", "<script>"},
		},
		{
			name:        "a proxy or gateway error page",
			status:      http.StatusServiceUnavailable,
			contentType: "text/html; charset=utf-8",
			body:        "<html><body>503 upstream connect error, internal-host-9.internal</body></html>",
			wantAll:     []string{"503", "text/html"},
			wantNone:    []string{"internal-host-9.internal", "upstream connect error"},
		},
		{
			name:        "no content type at all",
			status:      http.StatusBadGateway,
			contentType: "",
			body:        "",
			wantAll:     []string{"502"},
			wantNone:    []string{"shield challenge blocked login"},
		},
	}

	adapters := map[string]loginCapable{
		"new-api": &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")},
		"one-api": &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")},
	}

	for _, tc := range cases {
		for name, adapter := range adapters {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				srv := loginAnswerServer(t, tc.status, tc.contentType, tc.body)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				result, err := adapter.Login(ctx, srv.URL, "root", "metapi123", nil, nil)
				if err != nil {
					t.Fatalf("Login returned an error instead of a failed result: %v", err)
				}
				if result.Success {
					t.Fatalf("Login.Success = true for a non-JSON HTTP %d answer", tc.status)
				}
				msg := result.Message
				if strings.TrimSpace(msg) == "" {
					t.Fatal("Login.Message is empty: the operator gets no reason at all")
				}
				for _, want := range tc.wantAll {
					if !strings.Contains(msg, want) {
						t.Errorf("Login.Message = %q, want it to name %q", msg, want)
					}
				}
				for _, notWant := range tc.wantNone {
					if strings.Contains(msg, notWant) {
						t.Errorf("Login.Message = %q, must not contain %q", msg, notWant)
					}
				}
			})
		}
	}
}

// The accurate message must not cost the accurate success paths: a JSON answer
// still decides everything, including the legacy cookie-only login.
func TestLoginStillTrustsAJsonAnswerOverTheBlockedMessage(t *testing.T) {
	srv := loginAnswerServer(t, http.StatusOK, "application/json",
		`{"success":true,"message":"","data":{"access_token":"sk-durable-token"}}`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := (&NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}).Login(ctx, srv.URL, "root", "metapi123", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !result.Success || result.AccessToken != "sk-durable-token" {
		t.Fatalf("Login = %+v, want success with the JSON token", result)
	}
	if strings.Contains(result.Message, "login blocked") {
		t.Errorf("a successful JSON login carried the blocked message: %q", result.Message)
	}
}
