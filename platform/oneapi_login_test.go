package platform

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// oneApiAdapterUnderTest builds the adapter under test, mirroring
// newApiAdapterUnderTest in newapi_login_test.go.
func oneApiAdapterUnderTest() *OneApiAdapter {
	return &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
}

// one-api v0.6.10 login: success:true with an EMPTY data.access_token; the
// real credential is the Set-Cookie session cookie, which must become the
// stored AccessToken.
func TestOneApiAdapter_Login_V0610SessionCookie(t *testing.T) {
	srv := loginFixtureServer(t, `{
		"data": {"id": 1, "username": "root", "access_token": ""},
		"message": "",
		"success": true
	}`, 0, "session=abc123; Path=/; HttpOnly")
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := o.Login(ctx, srv.URL, "root", "123456", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !result.Success {
		t.Fatalf("Login.Success = false, want true (message: %q)", result.Message)
	}
	if result.AccessToken != "session=abc123" {
		t.Errorf("AccessToken = %q, want session=abc123", result.AccessToken)
	}
	if result.Username != "root" {
		t.Errorf("Username = %q, want root", result.Username)
	}
}

// Legacy one-api (v0.5) login: success:true with a non-empty data.access_token
// must keep returning the token.
func TestOneApiAdapter_Login_LegacyAccessToken(t *testing.T) {
	srv := loginFixtureServer(t, `{"success":true,"message":"","data":{"access_token":"sk-legacy-token"}}`, 0, "")
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := o.Login(ctx, srv.URL, "root", "123456", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !result.Success {
		t.Fatalf("Login.Success = false, want true (message: %q)", result.Message)
	}
	if result.AccessToken != "sk-legacy-token" {
		t.Errorf("AccessToken = %q, want sk-legacy-token", result.AccessToken)
	}
}

// Failure: explicit success:false must stay a failure with the upstream message.
func TestOneApiAdapter_Login_Failure(t *testing.T) {
	srv := loginFixtureServer(t, `{"message":"Username or password is incorrect.","success":false}`, 0, "")
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := o.Login(ctx, srv.URL, "root", "wrong-password", nil, nil)
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

// success:true with neither a token nor a usable session cookie must FAIL
// (BaseAdapter used to return Success:true with an empty token here, which
// surfaced as a generic "login failed" after the empty-token rejection).
func TestOneApiAdapter_Login_NoUsableCredentialFails(t *testing.T) {
	srv := loginFixtureServer(t, `{"data":{"id":1,"username":"root","access_token":""},"message":"","success":true}`, 0, "acw_tc=waf-only; Path=/")
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := o.Login(ctx, srv.URL, "root", "123456", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Success {
		t.Fatalf("Login.Success = true, want false (AccessToken: %q)", result.AccessToken)
	}
}

// selfFixtureServer serves /api/user/self rejecting Bearer auth and accepting
// the session cookie, so cookie-fallback tests can assert both the fallback
// path and the exact Cookie header sent.
func selfFixtureServer(t *testing.T, selfDataJSON string) (*httptest.Server, *string) {
	t.Helper()
	var cookieHeaderSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/self":
			if r.Header.Get("Authorization") != "" {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"success":false,"message":"Invalid access token"}`)
				return
			}
			cookieHeaderSeen = r.Header.Get("Cookie")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"success":true,"message":"","data":%s}`, selfDataJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &cookieHeaderSeen
}

func TestOneApiAdapter_GetUserInfo_CookieFallback(t *testing.T) {
	srv, cookieSeen := selfFixtureServer(t,
		`{"id":1,"username":"root","display_name":"Root Admin","email":"root@example.com"}`)
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	info, err := o.GetUserInfo(ctx, srv.URL, "session=abc123", nil, nil)
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}
	if info == nil {
		t.Fatal("GetUserInfo returned nil user info for a cookie credential")
	}
	if info.Username != "root" {
		t.Errorf("Username = %q, want root", info.Username)
	}
	if info.DisplayName != "Root Admin" {
		t.Errorf("DisplayName = %q, want Root Admin", info.DisplayName)
	}
	if *cookieSeen != "session=abc123" {
		t.Errorf("Cookie header = %q, want session=abc123", *cookieSeen)
	}
}

func TestOneApiAdapter_GetUserInfo_BearerPreferred(t *testing.T) {
	// Bearer succeeds: the Cookie fallback must never be attempted.
	var cookieHeaderSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" {
			http.NotFound(w, r)
			return
		}
		cookieHeaderSeen = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"message":"","data":{"id":1,"username":"root"}}`)
	}))
	t.Cleanup(srv.Close)
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	info, err := o.GetUserInfo(ctx, srv.URL, "sk-legacy-token", nil, nil)
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}
	if info == nil || info.Username != "root" {
		t.Fatalf("GetUserInfo = %+v, want root", info)
	}
	if cookieHeaderSeen != "" {
		t.Errorf("unexpected Cookie header on Bearer path: %q", cookieHeaderSeen)
	}
}

func TestOneApiAdapter_GetBalance_CookieFallback(t *testing.T) {
	srv, cookieSeen := selfFixtureServer(t,
		`{"id":1,"username":"root","quota":1000000,"used_quota":500000}`)
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	balance, err := o.GetBalance(ctx, srv.URL, "session=abc123", nil, nil)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.Balance != 1.0 {
		t.Errorf("Balance = %f, want 1.0 ((1000000-500000)/500000)", balance.Balance)
	}
	if balance.Quota != 2.0 {
		t.Errorf("Quota = %f, want 2.0", balance.Quota)
	}
	if *cookieSeen != "session=abc123" {
		t.Errorf("Cookie header = %q, want session=abc123", *cookieSeen)
	}
}

// checkinFixtureServer rejects Bearer POST /api/user/checkin and accepts the
// Cookie header, recording what was sent.
func checkinFixtureServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var cookieHeaderSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/checkin" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"success":false,"message":"Invalid access token"}`)
			return
		}
		cookieHeaderSeen = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"message":"Check-in successful"}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &cookieHeaderSeen
}

func TestOneApiAdapter_Checkin_CookieFallback(t *testing.T) {
	srv, cookieSeen := checkinFixtureServer(t)
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := o.Checkin(ctx, srv.URL, "session=abc123", nil, nil)
	if err != nil {
		t.Fatalf("Checkin: %v", err)
	}
	if !result.Success {
		t.Fatalf("Checkin.Success = false, want true (message: %q)", result.Message)
	}
	if *cookieSeen != "session=abc123" {
		t.Errorf("Cookie header = %q, want session=abc123", *cookieSeen)
	}
}

// VerifyToken must dispatch to the cookie-aware overrides (BaseAdapter's
// version statically calls its Bearer-only GetUserInfo and would return
// TokenType "unknown" for a cookie credential).
func TestOneApiAdapter_VerifyToken_CookieCredential(t *testing.T) {
	srv, _ := selfFixtureServer(t,
		`{"id":1,"username":"root","quota":1000000,"used_quota":0}`)
	o := oneApiAdapterUnderTest()
	ctx, cancel := loginContext(t)
	defer cancel()

	result, err := o.VerifyToken(ctx, srv.URL, "session=abc123", nil, nil)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if result.TokenType != "session" {
		t.Fatalf("TokenType = %q, want session", result.TokenType)
	}
	if result.UserInfo == nil || result.UserInfo.Username != "root" {
		t.Errorf("UserInfo = %+v, want root", result.UserInfo)
	}
	if result.Balance == nil || result.Balance.Balance != 2.0 {
		t.Errorf("Balance = %+v, want quota 2.0 via cookie path", result.Balance)
	}
}

func TestIsCookieCredential(t *testing.T) {
	tests := []struct {
		name       string
		credential string
		want       bool
	}{
		{"session cookie", "session=abc123", true},
		{"multi-pair cookie header", "session=abc123; token=xyz", true},
		{"cookie with whitespace between pairs", "session=abc123 ; token=xyz", true},
		{"bearer-prefixed cookie string", "Bearer session=abc123", true},
		{"sk token", "sk-v0-token", false},
		{"bare token without equals", "abc123", false},
		{"jwt segments", "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6MX0.sig", false},
		{"jwt with base64 padding", "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6MX0.si==", false},
		{"empty credential", "", false},
		{"empty value pair", "session=", false},
		{"pair with space before equals", "session =abc123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCookieCredential(tt.credential); got != tt.want {
				t.Errorf("isCookieCredential(%q) = %v, want %v", tt.credential, got, tt.want)
			}
		})
	}
}
