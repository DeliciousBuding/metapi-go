package admin

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestRequestAttemptBoundary(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var mux chi.Router
			if dialect == store.DialectPostgres {
				db, mux = setupStatsPostgresTest(t)
			} else {
				db, mux = setupStatsSQLiteTest(t)
			}
			fixture := seedRequestAttemptsSite(t, db)
			legacyID := insertAttemptRow(t, db, nil, 0, "success", 200, fixture.accountID, "")
			legacy := getRequestAttempts(t, mux, legacyID)
			if value, exists := legacy["lastAttempt"]; !exists || value != nil {
				t.Fatalf("uncorrelated row cannot establish the last attempt: %#v", legacy)
			}

			first := insertAttemptRow(t, db, "boundary-request", 0, "failed", 502, fixture.accountID, "first")
			last := insertAttemptRow(t, db, "boundary-request", 2, "failed", 429, fixture.accountID, "limited")
			view := getRequestAttempts(t, mux, first)
			attempt, ok := view["lastAttempt"].(map[string]any)
			if !ok || attempt["logId"] != float64(last) || attempt["httpStatus"] != float64(429) || view["partial"] != true {
				t.Fatalf("last recorded attempt must retain upstream status and known gaps: %#v", view)
			}

			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			failed := (&statsHandler{db: db.DB}).buildRequestAttempts(map[string]any{
				"id": first, "requestId": "boundary-request", "status": "failed", "httpStatus": 502,
			})
			if value, exists := failed["lastAttempt"]; !exists || value != nil || failed["partial"] != true {
				t.Fatalf("query failure cannot promote the viewed row to last attempt: %#v", failed)
			}
		})
	}
}
