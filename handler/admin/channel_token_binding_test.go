package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestChannelTokenBinding(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var router chi.Router
			if dialect == "postgres" {
				db, router = setupTokenRoutesPostgresTest(t)
			} else {
				db, router = setupTokenRoutesTest(t)
			}
			routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
			_, otherAccountID, otherTokenID := seedRouteChannelRefs(t, db)
			path := "/api/routes/" + itoa(routeID) + "/channels"
			for _, badToken := range []int64{0, -1, otherTokenID, otherTokenID + 1000000} {
				result := doPostJSON(t, router, path, map[string]any{"accountId": accountID, "tokenId": badToken})
				if result.Code != http.StatusBadRequest {
					t.Fatalf("token %d: status=%d body=%s", badToken, result.Code, result.Body.String())
				}
			}
			result := doPostJSON(t, router, path+"/batch", map[string]any{"channels": []map[string]any{
				{"accountId": accountID, "tokenId": otherTokenID},
				{"accountId": accountID, "tokenId": tokenID},
				{"accountId": accountID, "tokenId": tokenID},
				{"accountId": otherAccountID},
			}})
			if result.Code != http.StatusOK {
				t.Fatalf("batch: %d %s", result.Code, result.Body.String())
			}
			var batch struct {
				Created int      `json:"created"`
				Skipped int      `json:"skipped"`
				Errors  []string `json:"errors"`
			}
			if err := json.Unmarshal(result.Body.Bytes(), &batch); err != nil {
				t.Fatal(err)
			}
			if batch.Created != 2 || batch.Skipped != 1 || len(batch.Errors) != 1 {
				t.Fatalf("unexpected partial result: %+v", batch)
			}
			var savedTokenID int64
			if err := db.Get(&savedTokenID, "SELECT token_id FROM route_channels WHERE route_id = ? AND account_id = ?", routeID, accountID); err != nil {
				t.Fatal(err)
			}
			if savedTokenID != tokenID {
				t.Fatalf("bound token=%d, want %d", savedTokenID, tokenID)
			}
			for _, binding := range []map[string]any{
				{"accountId": accountID, "tokenId": tokenID},
				{"accountId": otherAccountID},
			} {
				duplicate := doPostJSON(t, router, path, binding)
				if duplicate.Code != http.StatusBadRequest {
					t.Fatalf("duplicate: %d %s", duplicate.Code, duplicate.Body.String())
				}
			}
		})
	}
}

func TestChannelTokenBindingReadFailure(t *testing.T) {
	db, router := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	if _, err := db.Exec("ALTER TABLE account_tokens RENAME TO unavailable_tokens"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("ALTER TABLE unavailable_tokens RENAME TO account_tokens") })
	result := doPostJSON(t, router, "/api/routes/"+itoa(routeID)+"/channels", map[string]any{"accountId": accountID, "tokenId": tokenID})
	if result.Code != http.StatusInternalServerError {
		t.Fatalf("read failure: %d %s", result.Code, result.Body.String())
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM route_channels WHERE route_id = ?", routeID); err != nil || count != 0 {
		t.Fatalf("write after read failure: count=%d err=%v", count, err)
	}
}
