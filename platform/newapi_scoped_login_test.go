package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

var newAPIMetapiScopes = []string{
	"api_key:read", "api_key:reveal", "api_key:write", "profile:read", "wallet:read", "wallet:write",
}

const scopedTestPAT = "nap_durable-dashboard-pat"

func scopedLoginServer(t *testing.T, existing string, existingScopes []string, listingStatus int) (*httptest.Server, func() []string) {
	t.Helper()
	encodedScopes := mustJSON(t, existingScopes)
	var mu sync.Mutex
	var calls []string
	var proofIssued atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step := r.Method + " " + r.URL.Path
		mu.Lock()
		calls = append(calls, step)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if step != "POST /api/user/login" && (r.Header.Get("Authorization") != "Bearer "+stepUpJWT || r.Header.Get("X-Auth-Session") != stepUpSID) {
			t.Error("scoped PAT flow lost the browser session")
		}
		switch step {
		case "POST /api/user/login":
			fmt.Fprintf(w, `{"success":true,"data":{"access_token":%q,"access_expires_at":1893456000,"session":{"sid":%q}}}`, stepUpJWT, stepUpSID)
		case "GET /api/user/access_tokens/catalog":
			fmt.Fprint(w, `{"success":true,"data":{"groups":[{"group":"personal","resources":[]}]}}`)
		case "GET /api/user/access_tokens":
			if listingStatus != 0 {
				w.WriteHeader(listingStatus)
				fmt.Fprint(w, `{"success":false,"message":"unavailable"}`)
				return
			}
			ref := sha256.Sum256([]byte(existing))
			fmt.Fprintf(w, `{"success":true,"data":{"items":[{"token_ref":%q,"scopes":%s,"expires_at":0}]}}`, hex.EncodeToString(ref[:]), encodedScopes)
		case "POST /api/user/access_tokens":
			var body struct {
				Name      string   `json:"name"`
				Scopes    []string `json:"scopes"`
				ExpiresAt int64    `json:"expires_at"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error("invalid PAT creation JSON")
			}
			if body.Name != "Metapi" || !reflect.DeepEqual(body.Scopes, newAPIMetapiScopes) || body.ExpiresAt != 0 {
				t.Error("PAT request did not use the exact least-privilege grant")
			}
			if !proofIssued.Load() {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"success":false,"code":"SECURITY_PROOF_REQUIRED"}`)
				return
			}
			if r.Header.Get("X-Security-Proof") != stepUpProof {
				t.Error("PAT creation did not use the single-use proof")
			}
			fmt.Fprintf(w, `{"success":true,"data":{"token":%q}}`, scopedTestPAT)
		case "GET /api/verify/methods":
			if r.URL.RawQuery != "scope=access_token.generate" {
				t.Error("wrong verification scope")
			}
			fmt.Fprint(w, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"password","available":true}],"password_encryption_enabled":false}}`)
		case "POST /api/verify":
			var body struct {
				Scope    string `json:"scope"`
				Method   string `json:"method"`
				Password string `json:"password"`
				Context  struct {
					Scopes    []string `json:"scopes"`
					ExpiresAt int64    `json:"expires_at"`
				} `json:"context"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error("invalid proof request JSON")
			}
			if body.Scope != "access_token.generate" || body.Method != "password" || body.Password != stepUpPassword || !reflect.DeepEqual(body.Context.Scopes, newAPIMetapiScopes) || body.Context.ExpiresAt != 0 {
				t.Error("proof was not bound to the exact PAT grant and expiry")
			}
			proofIssued.Store(true)
			fmt.Fprint(w, stepUpProofBody(stepUpProof, "password", "access_token.generate", 1893456000))
		case "POST /api/user/auth/logout":
			fmt.Fprint(w, `{"success":true}`)
		default:
			t.Errorf("unexpected request: %s", step)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestNewAPIScopedLoginCreatesDurablePATAfterBoundProof(t *testing.T) {
	srv, calls := scopedLoginServer(t, "", nil, 0)
	result, err := newApiAdapterUnderTest().Login(context.Background(), srv.URL, stepUpUsername, stepUpPassword, nil, nil)
	if err != nil || result == nil || !result.Success || result.AccessToken != scopedTestPAT {
		t.Fatalf("scoped login did not produce a PAT: result=%+v err=%v", result, err)
	}
	want := []string{"POST /api/user/login", "GET /api/user/access_tokens/catalog", "POST /api/user/access_tokens", "GET /api/verify/methods", "POST /api/verify", "POST /api/user/access_tokens", "POST /api/user/auth/logout"}
	if !reflect.DeepEqual(calls(), want) {
		t.Fatalf("calls=%v, want %v", calls(), want)
	}
}

func TestNewAPIScopedLoginReusesOwnedPATWithoutMinting(t *testing.T) {
	const existing = "nap_existing-credential"
	srv, calls := scopedLoginServer(t, existing, newAPIMetapiScopes, 0)
	result, err := LoginReusingCredential(newApiAdapterUnderTest(), context.Background(), srv.URL, stepUpUsername, stepUpPassword, nil, nil, existing)
	if err != nil || result == nil || !result.Success || result.AccessToken != existing {
		t.Fatalf("relogin did not reuse PAT: result=%+v err=%v", result, err)
	}
	want := []string{"POST /api/user/login", "GET /api/user/access_tokens/catalog", "GET /api/user/access_tokens", "POST /api/user/auth/logout"}
	if !reflect.DeepEqual(calls(), want) {
		t.Fatalf("calls=%v, want %v", calls(), want)
	}
}

func TestNewAPIScopedLoginDoesNotReplaceActiveInsufficientPAT(t *testing.T) {
	const existing = "nap_existing-credential"
	srv, calls := scopedLoginServer(t, existing, []string{"profile:read"}, 0)
	result, err := LoginReusingCredential(newApiAdapterUnderTest(), context.Background(), srv.URL, stepUpUsername, stepUpPassword, nil, nil, existing)
	if err != nil || result == nil || result.Success || result.AccessToken != "" || !strings.Contains(result.Message, "scope") {
		t.Fatalf("insufficient grant must fail without minting: result=%+v err=%v", result, err)
	}
	want := []string{"POST /api/user/login", "GET /api/user/access_tokens/catalog", "GET /api/user/access_tokens", "POST /api/user/auth/logout"}
	if !reflect.DeepEqual(calls(), want) {
		t.Fatalf("calls=%v, want %v", calls(), want)
	}
}

func TestNewAPIScopedLoginDoesNotMintWhenReuseCannotBeChecked(t *testing.T) {
	const existing = "nap_existing-credential"
	srv, calls := scopedLoginServer(t, existing, newAPIMetapiScopes, http.StatusServiceUnavailable)
	result, err := LoginReusingCredential(newApiAdapterUnderTest(), context.Background(), srv.URL, stepUpUsername, stepUpPassword, nil, nil, existing)
	if err != nil || result == nil || result.Success || result.AccessToken != "" || !strings.Contains(result.Message, "could not be reused") {
		t.Fatalf("unavailable listing must fail closed: result=%+v err=%v", result, err)
	}
	want := []string{"POST /api/user/login", "GET /api/user/access_tokens/catalog", "GET /api/user/access_tokens", "POST /api/user/auth/logout"}
	if !reflect.DeepEqual(calls(), want) {
		t.Fatalf("calls=%v, want %v", calls(), want)
	}
}

func TestNewAPIScopedLoginDoesNotGuessAfterCatalogFailure(t *testing.T) {
	logs := captureStepUpLogs(t)
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /api/user/login":
			fmt.Fprintf(w, `{"success":true,"data":{"access_token":%q,"access_expires_at":1893456000,"session":{"sid":%q}}}`, stepUpJWT, stepUpSID)
		case "GET /api/user/access_tokens/catalog":
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, stepUpSecretError("AUTH_INTERNAL_ERROR"))
		case "POST /api/user/auth/logout":
			fmt.Fprint(w, `{"success":true}`)
		default:
			t.Error("failed catalog must not trigger a legacy probe or PAT creation")
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	result, err := newApiAdapterUnderTest().Login(context.Background(), srv.URL, stepUpUsername, stepUpPassword, nil, nil)
	if err != nil || result == nil || result.Success || result.AccessToken != "" || !strings.Contains(result.Message, "capability could not be checked") {
		t.Fatalf("catalog failure must reject login: result=%+v err=%v", result, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(calls, []string{"POST /api/user/login", "GET /api/user/access_tokens/catalog", "POST /api/user/auth/logout"}) {
		t.Fatalf("requests=%v, want only login, catalog and logout", calls)
	}
	assertStepUpNoSecrets(t, result.Message+logs.String())
}
