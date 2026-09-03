package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// New API v1 masks token keys in its list response. Persisting that display
// value creates an account_tokens row with value_status=masked_pending; route
// rebuild then has no usable credential and the proxy correctly reports
// "令牌不可用". The upstream exposes one ownership-checked batch endpoint for
// retrieving the full keys, so hydrate them before the adapter returns tokens.
func TestNewApiAdapter_GetAPITokens_HydratesMaskedKeysWithBatchEndpoint(t *testing.T) {
	var batchCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if got := r.Header.Get("Authorization"); got != "Bearer dashboard-pat" {
			http.Error(w, `{"message":"bad auth"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
			fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":7,"name":"relay","key":"abcd****wxyz","status":1,"group":"default"}]}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/batch/keys":
			batchCalls.Add(1)
			var body struct {
				IDs []int `json:"ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode batch body: %v", err)
			}
			if len(body.IDs) != 1 || body.IDs[0] != 7 {
				t.Errorf("batch ids = %v, want [7]", body.IDs)
			}
			fmt.Fprint(w, `{"success":true,"data":{"keys":{"7":"full-relay-key"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
	if err != nil {
		t.Fatalf("GetAPITokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Key != "full-relay-key" || !tokens[0].Enabled {
		t.Fatalf("tokens = %+v, want one enabled token with the full key", tokens)
	}
	if batchCalls.Load() != 1 {
		t.Fatalf("batch key calls = %d, want 1", batchCalls.Load())
	}
}

func TestNewApiAdapter_GetAPITokens_UnmaskedLegacyKeysNeedNoHydration(t *testing.T) {
	var unexpected atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/token/" {
			fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":7,"name":"relay","key":"legacy-full-key","status":1}]}}`)
			return
		}
		unexpected.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-token", nil, nil)
	if err != nil {
		t.Fatalf("GetAPITokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Key != "legacy-full-key" {
		t.Fatalf("tokens = %+v, want unchanged legacy key", tokens)
	}
	if unexpected.Load() != 0 {
		t.Fatalf("unmasked list made %d hydration requests, want 0", unexpected.Load())
	}
}

func TestNewApiAdapter_GetAPITokens_MaskedKeyHydrationFailureIsLoud(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			http.Error(w, `{"error":"not a relay key"}`, http.StatusUnauthorized)
		case "/api/user/self":
			fmt.Fprint(w, `{"success":true,"data":{"id":1,"username":"root"}}`)
		case "/api/token/":
			fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":7,"name":"relay","key":"abcd****wxyz","status":1}]}}`)
		case "/api/token/batch/keys":
			http.Error(w, `{"success":false,"message":"full key access denied"}`, http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
	if err == nil {
		t.Fatalf("GetAPITokens error = nil, tokens=%+v; want masked-key hydration failure to be explicit", tokens)
	}
	if len(tokens) != 0 {
		t.Fatalf("tokens = %+v, want none rather than a masked value that routing cannot use", tokens)
	}
	if token, err := newApiAdapterUnderTest().GetAPIToken(ctx, srv.URL, "dashboard-pat", nil, nil); err == nil {
		t.Fatalf("GetAPIToken error = nil, token=%v; want the hydration error propagated", token)
	}
	if result, err := newApiAdapterUnderTest().VerifyToken(ctx, srv.URL, "dashboard-pat", nil, nil); err == nil {
		t.Fatalf("VerifyToken error = nil, result=%+v; want token hydration failure propagated", result)
	}
}
