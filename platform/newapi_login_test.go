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

// loginFixtureServer serves a canned POST /api/user/login response.
func loginFixtureServer(t *testing.T, body string, status int, setCookie string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/login" {
			http.NotFound(w, r)
			return
		}
		if setCookie != "" {
			w.Header().Set("Set-Cookie", setCookie)
		}
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newApiAdapterUnderTest() *NewApiAdapter {
	return &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
}

func loginContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// new-api v0 login: top-level success with data.access_token (sk- key).
func TestNewApiAdapter_Login_V0Success(t *testing.T) {
	srv := loginFixtureServer(t, `{"success":true,"message":"","data":{"access_token":"sk-v0-token"}}`, 0, "")
	n := newApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := n.Login(ctx, srv.URL, "root", "metapi123", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !result.Success {
		t.Fatalf("Login.Success = false, want true (message: %q)", result.Message)
	}
	if result.AccessToken != "sk-v0-token" {
		t.Errorf("AccessToken = %q, want sk-v0-token", result.AccessToken)
	}
}

// new-api v1 failure: explicit success:false with a message must stay a failure.
func TestNewApiAdapter_Login_V1Failure(t *testing.T) {
	srv := loginFixtureServer(t, `{"message":"Username or password is incorrect.","success":false}`, 0, "")
	n := newApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := n.Login(ctx, srv.URL, "root", "wrong-password", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Success {
		t.Fatal("Login.Success = true, want false")
	}
	if !strings.Contains(result.Message, "incorrect") {
		t.Errorf("Message = %q, want the upstream error text", result.Message)
	}
}

// Legacy cookie login: success:true without a data token must still succeed
// via a usable session cookie.
func TestNewApiAdapter_Login_CookieOnlySuccess(t *testing.T) {
	srv := loginFixtureServer(t, `{"success":true,"message":""}`, 0, "session=abc123; Path=/")
	n := newApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := n.Login(ctx, srv.URL, "root", "metapi123", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !result.Success {
		t.Fatalf("Login.Success = false, want true (message: %q)", result.Message)
	}
	if result.AccessToken != "session=abc123" {
		t.Errorf("AccessToken = %q, want session=abc123", result.AccessToken)
	}
}

// BaseAdapter.Login must also tolerate the v1 shape (no top-level success).
func TestBaseAdapter_Login_V1SuccessNoTopLevelSuccess(t *testing.T) {
	srv := loginFixtureServer(t, `{
		"data": {
			"access_token": "v1-jwt-token",
			"access_expires_at": 1893456000,
			"session": {"sid": "sess-123"}
		}
	}`, 0, "")
	base := NewBaseAdapter("test-platform")
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := base.Login(ctx, srv.URL, "root", "metapi123", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !result.Success {
		t.Fatalf("Login.Success = false, want true (message: %q)", result.Message)
	}
	if result.AccessToken != "v1-jwt-token" {
		t.Errorf("AccessToken = %q, want v1-jwt-token", result.AccessToken)
	}
}

// BaseAdapter.Login failure path must surface the upstream message.
func TestBaseAdapter_Login_V1Failure(t *testing.T) {
	srv := loginFixtureServer(t, `{"message":"Username or password is incorrect.","success":false}`, 0, "")
	base := NewBaseAdapter("test-platform")
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := base.Login(ctx, srv.URL, "root", "wrong-password", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Success {
		t.Fatal("Login.Success = true, want false")
	}
	if !strings.Contains(result.Message, "incorrect") {
		t.Errorf("Message = %q, want the upstream error text", result.Message)
	}
}
