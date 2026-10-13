package upstream_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

// Catalog refresh contract (route-decision E batch): discovery diff with
// source attribution — manual rows preserved, stale discovery rows disabled
// (never deleted), added rows get no grants, and a stale preview basis is
// refused with a conflict.

func refreshChannelFixture(t *testing.T, dialect string, models *[]string) (*store.DB, int64) {
	t.Helper()
	db := connectDatabase(t, dialect)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := make([]string, 0, len(*models))
		for _, m := range *models {
			parts = append(parts, `{"id":"`+m+`"}`)
		}
		fmt.Fprint(w, `{"data":[`+strings.Join(parts, ",")+`]}`)
	}))
	t.Cleanup(server.Close)

	out, err := upstream.Connect(t.Context(), db.DB, connectInput(server.URL))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return db, out.ID
}

func strPtr(s string) *string { return &s }

func TestCatalogRefreshPreviewApplyRoundTrip(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			live := []string{"m1", "m2"}
			db, channelID := refreshChannelFixture(t, dialect, &live)

			// Operator-added native row; must survive every refresh.
			if _, err := upstream.CreateModels(t.Context(), db.DB, channelID, upstream.ModelsCreate{Name: strPtr("manual-keep")}); err != nil {
				t.Fatalf("manual model: %v", err)
			}
			// Upstream drops m1, adds m4.
			live = []string{"m2", "m4"}

			preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, nil)
			if err != nil {
				t.Fatalf("preview: %v", err)
			}
			if strings.Join(preview.Added, ",") != "m4" {
				t.Fatalf("added: %#v", preview.Added)
			}
			if len(preview.Stale) != 1 || preview.Stale[0].Name != "m1" {
				t.Fatalf("stale: %#v", preview.Stale)
			}
			if preview.Stale[0].GrantCount != 1 || len(preview.Stale[0].RouteIDs) != 1 {
				t.Fatalf("stale impact: %#v (connect creates one grant + route per model)", preview.Stale[0])
			}
			if strings.Join(preview.Kept, ",") != "m2" {
				t.Fatalf("kept: %#v", preview.Kept)
			}
			if strings.Join(preview.ManualOnly, ",") != "manual-keep" || preview.LegacyNativeCount != 1 {
				t.Fatalf("manual: only=%#v legacy=%d", preview.ManualOnly, preview.LegacyNativeCount)
			}

			applied, err := upstream.ApplyCatalogRefresh(t.Context(), db.DB, channelID, preview.BasisToken, nil)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if applied.AddedCount != 1 || applied.StaleCount != 1 {
				t.Fatalf("applied: %#v", applied)
			}

			// m1 disabled, still present (rollback anchor + history).
			var m1Enabled bool
			if err := db.Get(&m1Enabled, "SELECT enabled FROM upstream_models WHERE channel_id=? AND name='m1'", channelID); err != nil {
				t.Fatal(err)
			}
			if m1Enabled {
				t.Fatal("stale model must be disabled, not deleted")
			}
			// manual-keep untouched.
			var manualEnabled bool
			if err := db.Get(&manualEnabled, "SELECT enabled FROM upstream_models WHERE channel_id=? AND name='manual-keep'", channelID); err != nil {
				t.Fatal(err)
			}
			if !manualEnabled {
				t.Fatal("manual model must never be touched by refresh")
			}
			// m4 exists with discovery origin and NO grant (no auto-authorization).
			var origin string
			if err := db.Get(&origin, "SELECT origin_key FROM upstream_models WHERE channel_id=? AND name='m4'", channelID); err != nil {
				t.Fatal(err)
			}
			if origin != upstream.DiscoveryOriginForChannel(channelID) {
				t.Fatalf("m4 origin: %q", origin)
			}
			var grantCount int
			if err := db.Get(&grantCount, "SELECT COUNT(*) FROM upstream_grants g JOIN upstream_models m ON m.id=g.model_id WHERE m.channel_id=? AND m.name='m4'", channelID); err != nil {
				t.Fatal(err)
			}
			if grantCount != 0 {
				t.Fatal("refresh must not auto-grant newly discovered models")
			}

			// A fresh preview after apply: m1 is history (offline), not actionable stale.
			after, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, nil)
			if err != nil {
				t.Fatalf("second preview: %v", err)
			}
			if len(after.Stale) != 0 {
				t.Fatalf("disabled stale rows must leave the actionable set: %#v", after.Stale)
			}
			if strings.Join(after.Offline, ",") != "m1" {
				t.Fatalf("offline: %#v", after.Offline)
			}
			if strings.Join(after.Kept, ",") != "m2,m4" {
				t.Fatalf("kept after apply: %#v", after.Kept)
			}

			// Same token twice: the basis moved, so the second apply is refused.
			if _, err := upstream.ApplyCatalogRefresh(t.Context(), db.DB, channelID, preview.BasisToken, nil); err == nil {
				t.Fatal("re-apply with a spent basis token must fail")
			} else {
				var domain *upstream.Error
				if !errors.As(err, &domain) || domain.Status != 409 {
					t.Fatalf("re-apply: got %v, want 409", err)
				}
			}
		})
	}
}

func TestCatalogRefreshApplyRejectsStaleBasis(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			live := []string{"m1"}
			db, channelID := refreshChannelFixture(t, dialect, &live)

			preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, nil)
			if err != nil {
				t.Fatalf("preview: %v", err)
			}
			// Concurrent catalog edit after the preview invalidates the basis.
			if _, err := upstream.CreateModels(t.Context(), db.DB, channelID, upstream.ModelsCreate{Name: strPtr("concurrent-edit")}); err != nil {
				t.Fatal(err)
			}
			_, err = upstream.ApplyCatalogRefresh(t.Context(), db.DB, channelID, preview.BasisToken, nil)
			var domain *upstream.Error
			if !errors.As(err, &domain) || domain.Status != 409 {
				t.Fatalf("stale basis: got %v, want 409", err)
			}
		})
	}
}

func TestCatalogRefreshDiscoveryEmptyCatalog(t *testing.T) {
	live := []string{"m1"}
	db, channelID := refreshChannelFixture(t, store.DialectSQLite, &live)
	live = nil // httptest now serves an empty catalog
	// An empty authoritative catalog is a valid observation: everything
	// discovered is stale, manual rows stay untouched.
	preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, nil)
	if err != nil {
		t.Fatalf("empty catalog preview: %v", err)
	}
	if preview.Discovery.Status != "empty" {
		t.Fatalf("discovery: %#v", preview.Discovery)
	}
	if len(preview.Stale) != 1 || preview.Stale[0].Name != "m1" {
		t.Fatalf("stale: %#v", preview.Stale)
	}
}

func TestCatalogRefreshGCPCredentialResolution(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			setup := func(t *testing.T, seenAuth *atomic.Value) (*store.DB, int64) {
				db := connectDatabase(t, dialect)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					seenAuth.Store(r.Header.Get("Authorization"))
					fmt.Fprint(w, `{"data":[{"id":"m1"}]}`)
				}))
				t.Cleanup(server.Close)
				out, err := upstream.Connect(t.Context(), db.DB, connectInput(server.URL))
				if err != nil {
					t.Fatalf("connect: %v", err)
				}
				// gcp_service_account credentials hold no token until exchanged.
				if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET kind=?, secret='' WHERE channel_id=?`), store.DirectCredentialGCPServiceAccount, out.ID); err != nil {
					t.Fatal(err)
				}
				return db, out.ID
			}

			t.Run("resolves token for discovery", func(t *testing.T) {
				var seenAuth atomic.Value
				db, channelID := setup(t, &seenAuth)
				resolve := func(ctx context.Context, credentialID int64, proxyURL *string) (string, error) {
					return "fixture-gcp-token", nil
				}
				if _, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, resolve); err != nil {
					t.Fatalf("preview: %v", err)
				}
				if got, _ := seenAuth.Load().(string); got != "Bearer fixture-gcp-token" {
					t.Fatalf("upstream authorization: %q", got)
				}
			})

			t.Run("exchange failure is explicit", func(t *testing.T) {
				var seenAuth atomic.Value
				db, channelID := setup(t, &seenAuth)
				resolve := func(ctx context.Context, credentialID int64, proxyURL *string) (string, error) {
					return "", errors.New("material rejected")
				}
				_, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, resolve)
				if err == nil || !strings.Contains(err.Error(), "token exchange failed") {
					t.Fatalf("preview error: %v", err)
				}
			})

			t.Run("no resolver reports no usable credential", func(t *testing.T) {
				var seenAuth atomic.Value
				db, channelID := setup(t, &seenAuth)
				_, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, nil)
				if err == nil || !strings.Contains(err.Error(), "no enabled credential") {
					t.Fatalf("preview error: %v", err)
				}
			})
		})
	}
}
