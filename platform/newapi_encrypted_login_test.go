package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewApiAdapter_Login_EncryptedPasswordFailureExplainsManualPAT(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/login":
			fmt.Fprint(w, `{"success":false,"message":"Invalid parameters"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/login/encryption-key":
			fmt.Fprint(w, `{"success":true,"data":{"enabled":true,"kid":"test-key","public_key":"test-public-key"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	result, err := newApiAdapterUnderTest().Login(context.Background(), srv.URL, "encrypted-user", "plaintext-password", nil, nil)
	if err != nil {
		t.Fatalf("Login returned transport error: %v", err)
	}
	if result == nil || result.Success || result.AccessToken != "" {
		t.Fatalf("encrypted-password login must fail closed: %+v", result)
	}
	if !strings.Contains(result.Message, "encrypted-password") || !strings.Contains(result.Message, "manually") {
		t.Fatalf("message must name the unsupported variant and manual PAT recovery: %q", result.Message)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests = %d, want failed login plus encryption-key diagnosis", calls.Load())
	}
	assertStepUpNoSecrets(t, result.Message)
}

func TestNewApiAdapter_Login_InvalidParametersWithoutEncryptionKeepsMessage(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/login":
			fmt.Fprint(w, `{"success":false,"message":"Invalid parameters"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/login/encryption-key":
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	result, err := newApiAdapterUnderTest().Login(context.Background(), srv.URL, "legacy-user", "wrong-password", nil, nil)
	if err != nil {
		t.Fatalf("Login returned transport error: %v", err)
	}
	if result == nil || result.Success || !strings.Contains(result.Message, "Invalid parameters") {
		t.Fatalf("legacy invalid-parameters message was not preserved: %+v", result)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests = %d, want failed login plus legacy 404 diagnosis", calls.Load())
	}
}

func TestNewApiAdapter_Login_HealthyLegacyLoginSkipsEncryptionProbe(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/api/user/login" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"access_token":"legacy-durable-token"}}`)
	}))
	t.Cleanup(srv.Close)

	result, err := newApiAdapterUnderTest().Login(context.Background(), srv.URL, "legacy-user", "plaintext-password", nil, nil)
	if err != nil {
		t.Fatalf("Login returned transport error: %v", err)
	}
	if result == nil || !result.Success || result.AccessToken != "legacy-durable-token" {
		t.Fatalf("legacy plaintext login was not preserved: %+v", result)
	}
	if calls.Load() != 1 {
		t.Fatalf("requests = %d, want only the successful legacy login", calls.Load())
	}
}
