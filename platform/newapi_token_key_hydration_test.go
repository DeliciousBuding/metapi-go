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

// Deletion must identify the actual owned token, not mistake a masked list key
// (or a refused key lookup) for proof that the upstream token is absent.
func TestNewApiAdapter_DeleteAPIToken_ResolvesMaskedIdentityBeforeDeleting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cookie     bool
		list       string
		batch      string
		batchCode  int
		wantError  bool
		wantDelete int32
		targetKey  string
	}{
		{name: "bare_stored_key", targetKey: "full-relay-key", batch: `{"success":true,"data":{"keys":{"7":"full-relay-key"}}}`, wantDelete: 1},
		{name: "bearer", batch: `{"success":true,"data":{"keys":{"7":"full-relay-key"}}}`, wantDelete: 1},
		{name: "cookie", cookie: true, batch: `{"success":true,"data":{"keys":{"7":"full-relay-key"}}}`, wantDelete: 1},
		{name: "actually_absent", batch: `{"success":true,"data":{"keys":{"7":"another-relay-key"}}}`},
		{name: "lookup_denied", batchCode: http.StatusForbidden, batch: `{"success":false,"message":"key lookup refused"}`, wantError: true},
		{name: "lookup_missing_key", batch: `{"success":true,"data":{"keys":{}}}`, wantError: true},
		{name: "lookup_refused_with_data", batch: `{"success":false,"data":{"keys":{"7":"full-relay-key"}}}`, wantError: true},
		{name: "unmasked_missing_id", list: `{"success":true,"data":{"items":[{"key":"full-relay-key"}]}}`, wantError: true},
		{name: "missing_id", list: `{"success":true,"data":{"items":[{"key":"abcd****wxyz"}]}}`, wantError: true},
		{name: "empty_legacy", list: `{"success":true,"data":[]}`},
		{name: "empty_v1", list: `{"success":true,"data":{"items":[],"total":0}}`},
		{name: "listing_malformed", list: `{"success":true}`, wantError: true},
		{name: "listing_missing_key", list: `{"success":true,"data":[{"id":7}]}`, wantError: true},
		{name: "listing_refused", list: `{"success":false,"message":"list refused"}`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var deleted atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if tc.cookie && r.Header.Get("Cookie") == "" {
					http.Error(w, `{"success":false}`, http.StatusUnauthorized)
					return
				}
				if r.Header.Get("New-Api-User") != "1" {
					t.Error("listing, key lookup and deletion must keep the owner identity")
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
					body := tc.list
					if body == "" {
						body = `{"success":true,"data":{"items":[{"id":7,"key":"abcd****wxyz","status":1}]}}`
					}
					fmt.Fprint(w, body)
				case r.Method == http.MethodPost && r.URL.Path == "/api/token/batch/keys":
					var body struct {
						IDs []int `json:"ids"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) != 1 || body.IDs[0] != 7 {
						t.Errorf("unexpected key lookup IDs: %v, error: %v", body.IDs, err)
					}
					if tc.batchCode != 0 {
						w.WriteHeader(tc.batchCode)
					}
					fmt.Fprint(w, tc.batch)
				case r.Method == http.MethodDelete && r.URL.Path == "/api/token/7":
					deleted.Add(1)
					fmt.Fprint(w, `{"success":true}`)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			uid := 1
			target := tc.targetKey
			if target == "" {
				target = "sk-full-relay-key"
			}
			err := newApiAdapterUnderTest().DeleteAPIToken(ctx, srv.URL, "dashboard-pat", target, &uid, nil)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tc.wantError)
			}
			if got := deleted.Load(); got != tc.wantDelete {
				t.Fatalf("DELETE count = %d, want %d", got, tc.wantDelete)
			}
		})
	}
}

func TestNewApiAdapter_DeleteAPIToken_PartialPageCannotProveAbsence(t *testing.T) {
	for _, tc := range []struct {
		name         string
		count, total int
		present      bool
	}{
		{"reported_more", 1, 2, false},
		{"capped_page", UpstreamTokenListPageLimit, 0, false},
		{"target_on_full_page", UpstreamTokenListPageLimit, UpstreamTokenListPageLimit + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var deleted atomic.Int32
			items := make([]map[string]interface{}, tc.count)
			for i := range items {
				items[i] = map[string]interface{}{"id": i + 1, "key": fmt.Sprintf("other-%d", i), "status": 1}
			}
			if tc.present {
				items[0]["key"] = "target-key"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet && r.URL.Path == "/api/token/" {
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{"items": items, "total": tc.total}})
					return
				}
				if r.Method == http.MethodDelete && r.URL.Path == "/api/token/1" {
					deleted.Add(1)
					fmt.Fprint(w, `{"success":true}`)
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(srv.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			uid := 1
			err := newApiAdapterUnderTest().DeleteAPIToken(ctx, srv.URL, "dashboard-pat", "target-key", &uid, nil)
			if tc.present {
				if err != nil || deleted.Load() != 1 {
					t.Fatalf("present target: error=%v deletes=%d", err, deleted.Load())
				}
			} else if err == nil || deleted.Load() != 0 {
				t.Fatalf("partial absence: error=%v deletes=%d, want failure without deletion", err, deleted.Load())
			}
		})
	}
}
