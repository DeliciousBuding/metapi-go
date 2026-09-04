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

// newTokenTestServer spins up an httptest server whose handler decides what to
// return based on method + path. It is used to drive the NewApi token CRUD
// methods without any real upstream.
func newTokenTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)
	return srv
}

// tokenJSONResponse writes a JSON body with the standard content type.
func tokenJSONResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, body)
}

// newApiTokenAdapter builds a fresh NewApiAdapter for tests.
func newApiTokenAdapter() *NewApiAdapter {
	return &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
}

// TestNewApiAdapter_GetAPITokens_Success verifies the happy path: the Bearer
// request returns a populated token list and the adapter short-circuits before
// any cookie fallback.
func TestNewApiAdapter_GetAPITokens_Success(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token/" && r.Method == http.MethodGet {
			tokenJSONResponse(w, `{"success":true,"data":[`+
				`{"key":"sk-success-123","name":"primary","status":1},`+
				`{"key":"sk-disabled-456","name":"secondary","status":2}`+
				`]}`)
			return
		}
		http.NotFound(w, r)
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokens, err := n.GetAPITokens(ctx, srv.URL, "bearer-token", &uid, nil)
	if err != nil {
		t.Fatalf("GetAPITokens: unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("GetAPITokens: got %d tokens, want 2", len(tokens))
	}
	if tokens[0].Key != "sk-success-123" {
		t.Errorf("tokens[0].Key = %q, want sk-success-123", tokens[0].Key)
	}
	if !tokens[0].Enabled {
		t.Errorf("tokens[0].Enabled = false, want true (status=1)")
	}
	if tokens[1].Enabled {
		t.Errorf("tokens[1].Enabled = true, want false (status=2)")
	}
}

// TestNewApiAdapter_GetAPITokens_EmptyList verifies that an empty data array
// yields an empty (non-nil) token slice with no error. The adapter falls
// through the cookie and probe fallbacks, all of which return nothing.
func TestNewApiAdapter_GetAPITokens_EmptyList(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/token/" && r.Method == http.MethodGet:
			tokenJSONResponse(w, `{"success":true,"data":[]}`)
		case r.URL.Path == "/api/user/self" && r.Method == http.MethodGet:
			tokenJSONResponse(w, `{"success":false,"message":"unauthorized"}`)
		default:
			http.NotFound(w, r)
		}
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tokens, err := n.GetAPITokens(ctx, srv.URL, "bearer-token", &uid, nil)
	if err != nil {
		t.Fatalf("GetAPITokens: unexpected error: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("GetAPITokens: got %d tokens, want 0", len(tokens))
	}
}

// TestNewApiAdapter_GetAPITokens_ErrorResponse verifies that when the upstream
// returns HTTP errors for every request, the adapter swallows the errors and
// returns an empty token slice rather than surfacing an error.
func TestNewApiAdapter_GetAPITokens_ErrorResponse(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"success":false,"message":"internal server error"}`)
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tokens, err := n.GetAPITokens(ctx, srv.URL, "bearer-token", &uid, nil)
	if err != nil {
		t.Fatalf("GetAPITokens: unexpected error: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("GetAPITokens: got %d tokens, want 0 on error response", len(tokens))
	}
}

// TestNewApiAdapter_GetAPIToken_Success verifies the singular GetAPIToken
// returns the first enabled token key.
func TestNewApiAdapter_GetAPIToken_Success(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token/" && r.Method == http.MethodGet {
			tokenJSONResponse(w, `{"success":true,"data":[`+
				`{"key":"sk-disabled","name":"disabled","status":2},`+
				`{"key":"sk-enabled-789","name":"enabled","status":1}`+
				`]}`)
			return
		}
		http.NotFound(w, r)
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tok, err := n.GetAPIToken(ctx, srv.URL, "bearer-token", &uid, nil)
	if err != nil {
		t.Fatalf("GetAPIToken: unexpected error: %v", err)
	}
	if tok == nil {
		t.Fatal("GetAPIToken: got nil, want non-nil token")
	}
	if *tok != "sk-enabled-789" {
		t.Fatalf("GetAPIToken: got %q, want sk-enabled-789", *tok)
	}
}

// TestNewApiAdapter_GetAPIToken_NotFound verifies that when no tokens are
// returned, GetAPIToken yields a nil pointer without error.
func TestNewApiAdapter_GetAPIToken_NotFound(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/token/" && r.Method == http.MethodGet:
			tokenJSONResponse(w, `{"success":true,"data":[]}`)
		case r.URL.Path == "/api/user/self" && r.Method == http.MethodGet:
			tokenJSONResponse(w, `{"success":false,"message":"unauthorized"}`)
		default:
			http.NotFound(w, r)
		}
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tok, err := n.GetAPIToken(ctx, srv.URL, "bearer-token", &uid, nil)
	if err != nil {
		t.Fatalf("GetAPIToken: unexpected error: %v", err)
	}
	if tok != nil {
		t.Fatalf("GetAPIToken: got %q, want nil", *tok)
	}
}

// TestNewApiAdapter_CreateAPIToken_Success verifies that a successful POST
// returns created=true.
func TestNewApiAdapter_CreateAPIToken_Success(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token/" && r.Method == http.MethodPost {
			tokenJSONResponse(w, `{"success":true,"data":{"key":"sk-new"}}`)
			return
		}
		http.NotFound(w, r)
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := n.CreateAPIToken(ctx, srv.URL, "bearer-token", &uid, &CreateAPITokenOptions{Name: "my-token"}, nil)
	if err != nil {
		t.Fatalf("CreateAPIToken: unexpected error: %v", err)
	}
	if !created {
		t.Fatal("CreateAPIToken: got created=false, want true")
	}
}

// TestNewApiAdapter_CreateAPIToken_ValidationError verifies that a failed POST
// (success:false from upstream) returns created=false without error.
func TestNewApiAdapter_CreateAPIToken_ValidationError(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token/" && r.Method == http.MethodPost {
			tokenJSONResponse(w, `{"success":false,"message":"token name too long"}`)
			return
		}
		http.NotFound(w, r)
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := n.CreateAPIToken(ctx, srv.URL, "bearer-token", &uid, &CreateAPITokenOptions{Name: "x"}, nil)
	if err != nil {
		t.Fatalf("CreateAPIToken: unexpected error: %v", err)
	}
	if created {
		t.Fatal("CreateAPIToken: got created=true, want false")
	}
}

// TestNewApiAdapter_CreateAPIToken_Unreachable pins the other half of the create
// contract: no attempt reached the upstream, so `false, nil` ("the upstream
// refused") would be a statement about a call that never happened.
func TestNewApiAdapter_CreateAPIToken_Unreachable(t *testing.T) {
	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := n.CreateAPIToken(ctx, unreachableBaseURL(t), "bearer-token", &uid, &CreateAPITokenOptions{Name: "x"}, nil)
	if err == nil {
		t.Fatal("CreateAPIToken must report an unreachable upstream instead of a silent false")
	}
	if created {
		t.Fatal("created = true for an upstream nobody reached")
	}
}

// TestNewApiAdapter_DeleteAPIToken_Unreachable pins that "we could not read the
// listing" is not the same answer as "the listing answered without this key". The
// caller deletes the local row when this returns nil, so conflating them removed
// the row while the upstream key stayed live.
func TestNewApiAdapter_DeleteAPIToken_Unreachable(t *testing.T) {
	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := n.DeleteAPIToken(ctx, unreachableBaseURL(t), "bearer-token", "sk-delete-me", &uid, nil)
	if err == nil {
		t.Fatal("DeleteAPIToken reported a completed delete for a call that never reached the upstream")
	}
	if !strings.Contains(err.Error(), "list upstream tokens") {
		t.Fatalf("error should say which step failed: %v", err)
	}
}

// TestNewApiAdapter_DeleteAPIToken_Success verifies that when the token key is
// found in the list and the DELETE succeeds, the adapter returns nil.
func TestNewApiAdapter_DeleteAPIToken_Success(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/token/" && r.Method == http.MethodGet:
			tokenJSONResponse(w, `{"success":true,"data":[{"key":"sk-delete-me","id":42,"status":1}]}`)
		case r.URL.Path == "/api/token/42" && r.Method == http.MethodDelete:
			tokenJSONResponse(w, `{"success":true}`)
		default:
			http.NotFound(w, r)
		}
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := n.DeleteAPIToken(ctx, srv.URL, "bearer-token", "sk-delete-me", &uid, nil); err != nil {
		t.Fatalf("DeleteAPIToken: unexpected error: %v", err)
	}
}

// TestNewApiAdapter_DeleteAPIToken_NotFound verifies that when the token key is
// absent from the list, the delete is treated as idempotent (nil error).
func TestNewApiAdapter_DeleteAPIToken_NotFound(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/token/" && r.Method == http.MethodGet:
			tokenJSONResponse(w, `{"success":true,"data":[{"key":"sk-other","id":99,"status":1}]}`)
		default:
			http.NotFound(w, r)
		}
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := n.DeleteAPIToken(ctx, srv.URL, "bearer-token", "sk-nonexistent", &uid, nil); err != nil {
		t.Fatalf("DeleteAPIToken: unexpected error: %v", err)
	}
}

// TestNewApiAdapter_DeleteAPIToken_EmptyKey verifies that an empty token key is
// a no-op (returns nil immediately without any HTTP traffic).
func TestNewApiAdapter_DeleteAPIToken_EmptyKey(t *testing.T) {
	srv := newTokenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request for empty-key delete: %s %s", r.Method, r.URL.Path)
	})

	n := newApiTokenAdapter()
	uid := 1
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := n.DeleteAPIToken(ctx, srv.URL, "bearer-token", "", &uid, nil); err != nil {
		t.Fatalf("DeleteAPIToken empty key: unexpected error: %v", err)
	}
}
