package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Regression for the double-prefix routing bug: RegisterDownstreamPricingRoutes
// registered absolute "/v1/..." paths while production mounts it inside the
// router group at /v1, so the documented GET /v1/pricing 404'd and only
// /v1/v1/pricing reached the handler (an unauthenticated probe could not see
// this: group middleware answers 401 before matching). Paths must be relative
// to the mount group.
func TestRegisterDownstreamPricingRoutes_MountsUnderV1(t *testing.T) {
	db := setupBackupTestDB(t)

	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		RegisterDownstreamPricingRoutes(r, db.DB)
	})

	for _, path := range []string{"/v1/pricing", "/v1/models/price-compare"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s status = 404: route unreachable (double-prefix regression)", path)
		}
	}

	// The doubled prefix must no longer resolve.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/v1/pricing", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/v1/v1/pricing status = %d, want 404", rec.Code)
	}
}
