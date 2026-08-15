package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCliProxyApiAdapter_Detect(t *testing.T) {
	a := &CliProxyApiAdapter{StandardAdapter: &StandardAdapter{
		BaseAdapter:               NewBaseAdapter("cliproxyapi"),
		LoginUnsupportedMessage:   "CLIProxyAPI does not support login",
		CheckinUnsupportedMessage: "CLIProxyAPI does not support checkin",
	}}

	ctx := context.Background()

	// Keyword tests (no HTTP needed)
	tests := []struct {
		url     string
		matches bool
	}{
		{"http://127.0.0.1:8317/", true},
		{"http://localhost:8317/v1/models", true},
		{"https://cliproxy.example.com", true},
		{"https://CLIPROXY.example.com", true},
		{"https://cli-proxy-api.example.com", true}, // hyphenated alias now matched
		{"https://api.openai.com", false},
		{"https://example.com:8080", false},
	}
	for _, tt := range tests {
		ok, err := a.Detect(ctx, tt.url)
		if err != nil {
			t.Errorf("Detect(%q) error: %v", tt.url, err)
			continue
		}
		if ok != tt.matches {
			t.Errorf("Detect(%q) = %v, want %v", tt.url, ok, tt.matches)
		}
	}
}

func TestCliProxyApiAdapter_PlatformName(t *testing.T) {
	a := &CliProxyApiAdapter{StandardAdapter: &StandardAdapter{
		BaseAdapter:               NewBaseAdapter("cliproxyapi"),
		LoginUnsupportedMessage:   "msg",
		CheckinUnsupportedMessage: "msg",
	}}
	if a.PlatformName() != "cliproxyapi" {
		t.Errorf("PlatformName: %q", a.PlatformName())
	}
}

func TestCliProxyApiAdapter_CustomMessages(t *testing.T) {
	a := &CliProxyApiAdapter{StandardAdapter: &StandardAdapter{
		BaseAdapter:               NewBaseAdapter("cliproxyapi"),
		LoginUnsupportedMessage:   "CLIProxyAPI does not support login",
		CheckinUnsupportedMessage: "CLIProxyAPI does not support checkin",
	}}
	ctx := context.Background()

	lr, _ := a.Login(ctx, "http://x", "u", "p", nil, nil)
	if lr.Message != "CLIProxyAPI does not support login" {
		t.Errorf("Login message: %q", lr.Message)
	}

	cr, _ := a.Checkin(ctx, "http://x", "t", nil, nil)
	if cr.Message != "CLIProxyAPI does not support checkin" {
		t.Errorf("Checkin message: %q", cr.Message)
	}
}

func TestCliProxyApiAdapter_DetectPort8317(t *testing.T) {
	a := &CliProxyApiAdapter{StandardAdapter: &StandardAdapter{
		BaseAdapter: NewBaseAdapter("cliproxyapi"),
	}}
	ctx := context.Background()

	// Port 8317 should match immediately (no HTTP call)
	ok, err := a.Detect(ctx, "http://192.168.1.1:8317/v1/models")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("should match port 8317")
	}

	// Needs trailing slash or end
	ok2, _ := a.Detect(ctx, "http://192.168.1.1:8317")
	if !ok2 {
		t.Error("should match port 8317 without path")
	}
}

func newCliProxyApiVerifyTestAdapter() *CliProxyApiAdapter {
	return &CliProxyApiAdapter{StandardAdapter: &StandardAdapter{BaseAdapter: NewBaseAdapter("cliproxyapi")}}
}

func TestCliProxyApiAdapter_VerifyToken_Models(t *testing.T) {
	// Provider API key path: /v1/models serves models for the key.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"gpt-4o"},{"id":"claude-opus"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a := newCliProxyApiVerifyTestAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := a.VerifyToken(ctx, srv.URL, "any-key", nil, nil)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if result.TokenType != "apikey" {
		t.Fatalf("TokenType = %q, want apikey", result.TokenType)
	}
	if len(result.Models) != 2 {
		t.Fatalf("Models = %v, want 2 models", result.Models)
	}
}

func TestCliProxyApiAdapter_VerifyToken_ManagementKey(t *testing.T) {
	// Management key path: /v1/models is empty (unauthenticated on
	// CLIProxyAPI), but the management probe accepts the key. VerifyToken
	// must report "apikey" for the valid key and "unknown" for anything else.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[],"object":"list"}`)
		case "/v0/management/auth-files":
			if r.Header.Get("Authorization") != "Bearer mgmt-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"files":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := newCliProxyApiVerifyTestAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := a.VerifyToken(ctx, srv.URL, "mgmt-key", nil, nil)
	if err != nil {
		t.Fatalf("VerifyToken (valid key): %v", err)
	}
	if result.TokenType != "apikey" {
		t.Fatalf("TokenType = %q, want apikey (valid management key)", result.TokenType)
	}

	invalid, err := a.VerifyToken(ctx, srv.URL, "wrong-key", nil, nil)
	if err != nil {
		t.Fatalf("VerifyToken (invalid key): %v", err)
	}
	if invalid.TokenType != "unknown" {
		t.Fatalf("TokenType = %q, want unknown (invalid management key)", invalid.TokenType)
	}
}

func TestCliProxyApiAdapter_VerifyToken_Unknown(t *testing.T) {
	// Empty models + management probe 401: the credential is not valid.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[],"object":"list"}`)
		case "/v0/management/auth-files":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := newCliProxyApiVerifyTestAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := a.VerifyToken(ctx, srv.URL, "garbage-token", nil, nil)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if result.TokenType != "unknown" {
		t.Fatalf("TokenType = %q, want unknown", result.TokenType)
	}
}
