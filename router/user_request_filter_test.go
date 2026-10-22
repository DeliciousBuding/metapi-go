package router

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestUserRequestFiltersHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newPortalFixture(t, dialect)
			cookie := f.activate(f.issue())
			now, model := time.Now().UTC().Format(time.RFC3339Nano), "fixture-model"
			if err := store.InsertGatewayRequest(context.Background(), f.db, store.GatewayRequest{ID: "filter-http", OwnerUserID: &f.user.ID, ModelRequested: &model, Transport: "http", TerminalStatus: "upstream_failed", StartedAt: now, EndedAt: now}); err != nil {
				t.Fatal(err)
			}
			body := portalJSON(t, f.call("GET", "/api/user/requests?search=fixture&status=upstream_failed&quotaState=missing", "", cookie, nil), 200)
			if body["total"] != float64(1) || len(body["items"].([]any)) != 1 {
				t.Fatal("combined filters not applied")
			}
			for _, query := range []string{"search=absent", "status=completed", "quotaState=unknown"} {
				body := portalJSON(t, f.call("GET", "/api/user/requests?"+query, "", cookie, nil), 200)
				if body["total"] != float64(0) {
					t.Fatalf("wrong filtered total: %+v", body)
				}
			}
			for _, query := range []string{"status=invalid", "quotaState=invalid", "search=" + strings.Repeat("x", 201), "search=a&search=b", "status=completed&status=upstream_failed", "page=1&page=2", "pageSize=20&pageSize=1"} {
				portalJSON(t, f.call("GET", "/api/user/requests?"+query, "", cookie, nil), 400)
			}
		})
	}
}
