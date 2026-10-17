package admin

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestClearRouteCooldownFailureAndRecovery(t *testing.T) {
	for name, setup := range map[string]func(*testing.T) (*store.DB, chi.Router){
		"sqlite": setupTokenRoutesTest, "postgres": setupTokenRoutesPostgresTest,
	} {
		t.Run(name, func(t *testing.T) {
			db, r := setup(t)
			routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
			otherRouteID, otherAccountID, otherTokenID := seedRouteChannelRefs(t, db)
			future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
			for _, refs := range [][3]int64{{routeID, accountID, tokenID}, {otherRouteID, otherAccountID, otherTokenID}} {
				for range 2 {
					_, err := db.Exec(db.Rebind(`INSERT INTO route_channels
						(route_id, account_id, token_id, source_model, enabled, cooldown_until,
						 consecutive_fail_count, cooldown_level, cooldown_reason_code, cooldown_reason, cooldown_reason_at)
						VALUES (?, ?, ?, 'gpt-4o', TRUE, ?, 3, 2, 'upstream_5xx', 'fixture failure', ?)`),
						refs[0], refs[1], refs[2], future, future)
					if err != nil {
						t.Fatal(err)
					}
				}
			}

			// Make the real UPDATE fail without closing the database, then retry
			// the same HTTP operation after storage becomes available again.
			if _, err := db.Exec(`ALTER TABLE route_channels RENAME TO route_channels_unavailable`); err != nil {
				t.Fatal(err)
			}
			renamed := true
			t.Cleanup(func() {
				if renamed {
					if _, err := db.Exec(`ALTER TABLE route_channels_unavailable RENAME TO route_channels`); err != nil {
						t.Error(err)
					}
				}
			})
			path := "/api/routes/" + itoa(routeID) + "/cooldown/clear"
			failed := doPostJSON(t, r, path, nil)
			if failed.Code != http.StatusInternalServerError || !strings.Contains(failed.Body.String(), `"error"`) || strings.Contains(failed.Body.String(), `"success":true`) {
				t.Fatalf("failed write reported as success: %d %s", failed.Code, failed.Body.String())
			}
			if _, err := db.Exec(`ALTER TABLE route_channels_unavailable RENAME TO route_channels`); err != nil {
				t.Fatal(err)
			}
			renamed = false

			assertCooled := func(route int64, want int) {
				t.Helper()
				var count int
				if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM route_channels WHERE route_id = ?
					AND cooldown_until = ? AND consecutive_fail_count = 3 AND cooldown_level = 2
					AND cooldown_reason_code = 'upstream_5xx' AND cooldown_reason = 'fixture failure' AND cooldown_reason_at = ?`), route, future, future); err != nil {
					t.Fatal(err)
				}
				if count != want {
					t.Fatalf("route %d: cooled rows = %d, want %d", route, count, want)
				}
			}
			assertCooled(routeID, 2)
			assertCooled(otherRouteID, 2)
			for range 2 {
				out := doPostJSON(t, r, path, nil)
				if out.Code != http.StatusOK || !strings.Contains(out.Body.String(), `"success":true`) {
					t.Fatalf("retry: %d %s", out.Code, out.Body.String())
				}
				var cleared int
				if err := db.Get(&cleared, db.Rebind(`SELECT COUNT(*) FROM route_channels WHERE route_id = ?
					AND cooldown_until IS NULL AND consecutive_fail_count = 0 AND cooldown_level = 0
					AND cooldown_reason_code IS NULL AND cooldown_reason IS NULL AND cooldown_reason_at IS NULL`), routeID); err != nil {
					t.Fatal(err)
				}
				if cleared != 2 {
					t.Fatalf("cleared rows = %d, want 2", cleared)
				}
				assertCooled(otherRouteID, 2)
			}
		})
	}
}
