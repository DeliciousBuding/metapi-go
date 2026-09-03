package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// New API v1 dashboard access JWTs expire after 15 minutes. Persisting that JWT
// as an account credential makes model discovery and balance refresh work at
// bind time, then fail later with an empty model list. A login session also
// counts against New API's active-session limit, so periodically logging in
// again leaks sessions until the upstream rejects every login with
// AUTH_SESSION_LIMIT. The smallest durable credential is New API's dashboard
// personal access token: mint it while the fresh session JWT is valid, revoke
// that short-lived login session immediately, and persist only the PAT.
func TestNewApiAdapter_Login_V1PromotesSessionJWTToDurablePATAndLogsOut(t *testing.T) {
	var patCalls atomic.Int32
	var logoutCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/login":
			w.Header().Set("Set-Cookie", "new_api_refresh=sess-123.refresh-secret; Path=/api/user/auth; HttpOnly")
			fmt.Fprint(w, `{"data":{"access_token":"short-lived-jwt","access_expires_at":1893456000,"session":{"sid":"sess-123"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/token":
			patCalls.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer short-lived-jwt" {
				t.Errorf("PAT Authorization = %q, want fresh session JWT", got)
			}
			fmt.Fprint(w, `{"success":true,"data":"durable-dashboard-pat"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/auth/logout":
			logoutCalls.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer short-lived-jwt" {
				t.Errorf("logout Authorization = %q, want the live session JWT", got)
			}
			if got := r.Header.Get("Cookie"); got != "new_api_refresh=sess-123.refresh-secret" {
				t.Errorf("logout Cookie = %q", got)
			}
			if got := r.Header.Get("X-Auth-Session"); got != "sess-123" {
				t.Errorf("logout X-Auth-Session = %q", got)
			}
			fmt.Fprint(w, `{"success":true}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			http.Error(w, `{"error":"not a relay API key"}`, http.StatusUnauthorized)
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/self":
			if got := r.Header.Get("Authorization"); got != "Bearer durable-dashboard-pat" {
				t.Errorf("self Authorization = %q, want promoted PAT", got)
			}
			fmt.Fprint(w, `{"success":true,"data":{"id":1,"username":"root"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/models":
			if got := r.Header.Get("Authorization"); got != "Bearer durable-dashboard-pat" {
				t.Errorf("models Authorization = %q, want promoted PAT", got)
			}
			fmt.Fprint(w, `{"success":true,"data":["gpt-4o-mini"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := newApiAdapterUnderTest().Login(ctx, srv.URL, "root", "metapi123", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !result.Success {
		t.Fatalf("Login.Success = false, message=%q", result.Message)
	}
	if result.AccessToken != "durable-dashboard-pat" {
		t.Fatalf("AccessToken = %q, want durable-dashboard-pat (never persist the 15-minute JWT)", result.AccessToken)
	}
	if patCalls.Load() != 1 || logoutCalls.Load() != 1 {
		t.Fatalf("PAT calls=%d logout calls=%d, want 1/1", patCalls.Load(), logoutCalls.Load())
	}

	models, err := newApiAdapterUnderTest().GetModels(ctx, srv.URL, result.AccessToken, nil, nil)
	if err != nil {
		t.Fatalf("GetModels with promoted PAT: %v", err)
	}
	if len(models) != 1 || models[0] != "gpt-4o-mini" {
		t.Fatalf("models = %v, want [gpt-4o-mini] from the durable PAT", models)
	}
}

func TestNewApiAdapter_Login_V1LogoutDoesNotDependOnRefreshCookie(t *testing.T) {
	var logoutCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/login":
			// Some proxies strip Set-Cookie. The session JWT still identifies the
			// exact login session and must be enough to revoke it.
			fmt.Fprint(w, `{"data":{"access_token":"short-lived-jwt","access_expires_at":1893456000,"session":{"sid":"sess-no-cookie"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/token":
			fmt.Fprint(w, `{"success":true,"data":"durable-dashboard-pat"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/auth/logout":
			logoutCalls.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer short-lived-jwt" {
				t.Errorf("logout Authorization = %q", got)
			}
			if got := r.Header.Get("Cookie"); got != "" {
				t.Errorf("logout Cookie = %q, want none", got)
			}
			if got := r.Header.Get("X-Auth-Session"); got != "sess-no-cookie" {
				t.Errorf("logout X-Auth-Session = %q", got)
			}
			fmt.Fprint(w, `{"success":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := newApiAdapterUnderTest().Login(ctx, srv.URL, "root", "metapi123", nil, nil)
	if err != nil || !result.Success || result.AccessToken != "durable-dashboard-pat" {
		t.Fatalf("Login = %+v, err=%v", result, err)
	}
	if logoutCalls.Load() != 1 {
		t.Fatalf("logout calls = %d, want 1 without a refresh cookie", logoutCalls.Load())
	}
}

func TestNewApiAdapter_Login_V1FailsLoudWhenDurablePATCannotBeIssued(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/login":
			w.Header().Set("Set-Cookie", "new_api_refresh=sess-456.refresh-secret; Path=/api/user/auth; HttpOnly")
			fmt.Fprint(w, `{"data":{"access_token":"short-lived-jwt","access_expires_at":1893456000,"session":{"sid":"sess-456"}}}`)
		case "/api/user/token":
			http.Error(w, `{"success":false,"message":"personal token disabled"}`, http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := newApiAdapterUnderTest().Login(ctx, srv.URL, "root", "metapi123", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Success {
		t.Fatalf("Login.Success = true with ephemeral credential %q; want fail loud", result.AccessToken)
	}
	if result.AccessToken != "" {
		t.Fatalf("AccessToken = %q, want empty on durable-token failure", result.AccessToken)
	}
}

func TestNewApiAdapter_Login_LegacyTokenDoesNotUseV1Promotion(t *testing.T) {
	var unexpected atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/user/login" {
			fmt.Fprint(w, `{"success":true,"data":{"access_token":"legacy-dashboard-token"}}`)
			return
		}
		unexpected.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := newApiAdapterUnderTest().Login(ctx, srv.URL, "root", "metapi123", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !result.Success || result.AccessToken != "legacy-dashboard-token" {
		t.Fatalf("legacy login result = %+v, want unchanged success", result)
	}
	if unexpected.Load() != 0 {
		t.Fatalf("legacy login made %d v1 promotion requests, want 0", unexpected.Load())
	}
}
