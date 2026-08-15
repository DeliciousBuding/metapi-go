package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newSub2ApiVerifyTestAdapter() *Sub2ApiAdapter {
	return &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
}

func TestSub2ApiAdapter_VerifyToken_SessionJWT(t *testing.T) {
	// Session JWT path: /api/v1/auth/me resolves the user, /api/v1/keys
	// provides the first API key. VerifyToken must report "session" with the
	// resolved user info, balance and discovered API token.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/me":
			if r.Header.Get("Authorization") != "Bearer jwt-token" {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"code":"UNAUTHORIZED","message":"invalid token"}`)
				return
			}
			fmt.Fprint(w, `{"code":0,"message":"success","data":{"id":1,"email":"admin@testbed.local","username":"admin","balance":2.5}}`)
		case "/api/v1/keys":
			fmt.Fprint(w, `{"code":0,"message":"success","data":{"items":[{"id":7,"key":"sk-test-key-7","name":"metapi","status":"active"}],"total":1}}`)
		case "/api/v1/api-keys":
			fmt.Fprint(w, `{"code":0,"message":"success","data":{"items":[]}}`)
		case "/api/v1/subscriptions/summary":
			fmt.Fprint(w, `{"code":0,"message":"success","data":{"items":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := newSub2ApiVerifyTestAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := a.VerifyToken(ctx, srv.URL, "jwt-token", nil, nil)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if result.TokenType != "session" {
		t.Fatalf("TokenType = %q, want session", result.TokenType)
	}
	if result.UserInfo == nil || result.UserInfo.Username != "admin" {
		t.Fatalf("UserInfo = %+v, want username admin", result.UserInfo)
	}
	if result.APIToken != "sk-test-key-7" {
		t.Fatalf("APIToken = %q, want sk-test-key-7", result.APIToken)
	}
	if result.Balance == nil {
		t.Fatal("Balance is nil for a verified session JWT")
	}
}

func TestSub2ApiAdapter_VerifyToken_APIKeyFallback(t *testing.T) {
	// API key path: /api/v1/auth/me rejects the key, /v1/models accepts it.
	// VerifyToken must fall back to "apikey" with the model list.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/me":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"code":"UNAUTHORIZED","message":"invalid token"}`)
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"gpt-4o-mini"},{"id":"claude-3-5-sonnet"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := newSub2ApiVerifyTestAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := a.VerifyToken(ctx, srv.URL, "sk-api-key", nil, nil)
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

func TestSub2ApiAdapter_VerifyToken_Unknown(t *testing.T) {
	// Neither /api/v1/auth/me nor /v1/models accepts the credential:
	// VerifyToken must report "unknown" without error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/me":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"code":"UNAUTHORIZED","message":"invalid token"}`)
		case "/v1/models":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"code":"INVALID_API_KEY","message":"Invalid API key"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := newSub2ApiVerifyTestAdapter()
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
