package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestOAuthMemberCooldownSQLite(t *testing.T) { testOAuthMemberCooldown(t, sqlitetest.Open(t)) }

func TestOAuthMemberCooldownPostgres(t *testing.T) {
	db, _ := setupSitesPostgresTest(t)
	testOAuthMemberCooldown(t, db)
}

func testOAuthMemberCooldown(t *testing.T, db *store.DB) {
	t.Helper()
	for _, query := range []string{
		`INSERT INTO sites (id, name, url, platform) VALUES (1, 'Fixture', 'https://fixture.example', 'codex')`,
		`INSERT INTO accounts (id, site_id, access_token) VALUES (1, 1, 'fixture-access'), (2, 1, 'fixture-access-two')`,
		`INSERT INTO oauth_route_units (id, site_id, provider, name) VALUES (7, 1, 'codex', 'Pool'), (8, 1, 'codex', 'Other pool')`,
		`INSERT INTO token_routes (id, model_pattern, enabled) VALUES (1, 'pool-model', TRUE)`,
		`INSERT INTO route_channels (id, route_id, account_id, oauth_route_unit_id, source_model, enabled) VALUES (1, 1, 1, 7, 'pool-model', TRUE)`,
		`INSERT INTO oauth_route_unit_members (id, unit_id, account_id, cooldown_until, consecutive_fail_count, cooldown_level, cooldown_reason_code, cooldown_reason, cooldown_reason_at, success_count, fail_count, last_selected_at, last_fail_at, created_at, updated_at)
		VALUES (11, 7, 1, '2999-01-01T00:00:00Z', 3, 2, 'rate_limited', 'fixture reason', '2026-10-18T00:00:00Z', 20, 4, '2026-10-17T00:00:00Z', '2026-10-18T00:00:00Z', '', ''),
		(12, 7, 2, '2999-01-01T00:00:00Z', 3, 2, 'rate_limited', 'fixture reason', '2026-10-18T00:00:00Z', 20, 4, '2026-10-17T00:00:00Z', '2026-10-18T00:00:00Z', '', '')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"sites", "accounts", "oauth_route_units", "token_routes"} {
		if _, err := db.Exec(`UPDATE ` + table + ` SET created_at='2026-10-18T00:00:00Z', updated_at='2026-10-18T00:00:00Z'`); err != nil {
			t.Fatal(err)
		}
	}
	previous, previousRuntime := store.GetDB(), config.RuntimeSafe()
	store.OverrideDB(db)
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { store.OverrideDB(previous); config.SetRuntime(previousRuntime); routing.SetGlobalCache(nil) })
	engine := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
	r := chi.NewRouter()
	RegisterOauthRoutes(r, db.DB)
	RegisterTokenRoutesWithDeps(r, db.DB, TokenRoutesDeps{Router: engine})
	check := func(want string) {
		t.Helper()
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/routes/decision?model=pool-model&endpoint=/v1/chat/completions", nil))
		if resp.Code != 200 {
			t.Fatalf("decision: %d %s", resp.Code, resp.Body.String())
		}
		var body struct {
			Decision routing.RouteDecisionExplanation `json:"decision"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Decision.Candidates) != 1 || len(body.Decision.Candidates[0].OAuthMembers) != 2 {
			t.Fatalf("member wire projection missing: %s", resp.Body.String())
		}
		member := body.Decision.Candidates[0].OAuthMembers[0]
		if body.Decision.Candidates[0].AccountID != 0 || body.Decision.SelectedAccountID != nil {
			t.Fatal("pool preview selected an account")
		}
		if member.MemberID != 11 || member.Eligibility != want {
			t.Fatalf("member diagnostics: %+v", member)
		}
		if strings.Contains(resp.Body.String(), "fixture-access") || strings.Contains(resp.Body.String(), "fixture reason") {
			t.Fatal("private data exposed")
		}
	}
	check("blocked")
	for _, tc := range []struct {
		unit, member string
		status       int
	}{
		{"bad", "11", 400}, {"7", "0", 400}, {"-1", "11", 400}, {"7", "bad", 400},
		{"8", "11", 404}, {"7", "99", 404}, {"7", "11", 200}, {"7", "11", 200},
	} {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/oauth/route-units/%s/members/%s/cooldown/clear", tc.unit, tc.member), nil))
		if resp.Code != tc.status {
			t.Fatalf("%+v: status=%d body=%s", tc, resp.Code, resp.Body.String())
		}
	}
	check("eligible") // Must observe invalidated routing cache, not the pre-clear member snapshot.
	wide, err := engine.ExplainSelectionRouteWide(context.Background(), 1, routing.DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat})
	if err != nil {
		t.Fatal(err)
	}
	if len(wide.Candidates) != 1 || len(wide.Candidates[0].OAuthMembers) != 2 {
		t.Fatalf("route-wide members: %+v", wide)
	}
	if members := wide.Candidates[0].OAuthMembers; members[0].Eligibility != "notEvaluated" || members[1].Eligibility != "blocked" {
		t.Fatalf("route-wide status: %+v", members)
	}
	var member, sibling store.OAuthRouteUnitMember
	if err := db.Get(&member, `SELECT * FROM oauth_route_unit_members WHERE id = 11`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&sibling, `SELECT * FROM oauth_route_unit_members WHERE id = 12`); err != nil {
		t.Fatal(err)
	}
	if member.CooldownUntil != nil || member.CooldownLevel != 0 || member.ConsecutiveFailCount != 0 || member.CooldownReason != nil || member.CooldownReasonCode != nil || member.CooldownReasonAt != nil {
		t.Fatalf("not cleared: %+v", member)
	}
	if member.SuccessCountOrZero() != 20 || member.FailCountOrZero() != 4 || member.LastSelectedAt == nil || *member.LastSelectedAt != "2026-10-17T00:00:00Z" || member.LastFailAt == nil || *member.LastFailAt != "2026-10-18T00:00:00Z" {
		t.Fatalf("history lost: %+v", member)
	}
	if sibling.CooldownUntil == nil || sibling.ConsecutiveFailCount != 3 || sibling.CooldownLevel != 2 {
		t.Fatalf("sibling changed: %+v", sibling)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/oauth/route-units/7/members/11/cooldown/clear", nil))
	if resp.Code != 500 {
		t.Fatalf("closed DB: %d %s", resp.Code, resp.Body.String())
	}
}
