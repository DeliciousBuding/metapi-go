package alert

import (
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Enrichment fixtures ----

func seedEnrichSite(t *testing.T, db *store.DB, siteName, accountName string) (siteID, accountID int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES (?, ?, 'openai', 'active', ?, ?)`,
		siteName, "https://"+siteName+".example.com", now, now,
	)
	if err != nil {
		t.Fatalf("insert site %s: %v", siteName, err)
	}
	siteID, _ = res.LastInsertId()
	res, err = db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, ?, 'tok', 'active', TRUE, ?, ?)`,
		siteID, accountName, now, now,
	)
	if err != nil {
		t.Fatalf("insert account %s: %v", accountName, err)
	}
	accountID, _ = res.LastInsertId()
	return siteID, accountID
}

func seedEnrichAccount(t *testing.T, db *store.DB, siteID int64, accountName string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, ?, 'tok', 'active', TRUE, ?, ?)`,
		siteID, accountName, now, now,
	)
	if err != nil {
		t.Fatalf("insert account %s: %v", accountName, err)
	}
	accountID, _ := res.LastInsertId()
	return accountID
}

func seedEnrichRoute(t *testing.T, db *store.DB, pattern string, enabled bool) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO token_routes (model_pattern, route_mode, routing_strategy, enabled, created_at, updated_at)
		 VALUES (?, 'pattern', 'weighted', ?, ?, ?)`,
		pattern, enabled, now, now,
	)
	if err != nil {
		t.Fatalf("insert route %s: %v", pattern, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedEnrichChannel(t *testing.T, db *store.DB, routeID, accountID int64, enabled bool) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO route_channels (route_id, account_id, priority, weight, enabled, manual_override)
		 VALUES (?, ?, 0, 10, ?, FALSE)`,
		routeID, accountID, enabled,
	)
	if err != nil {
		t.Fatalf("insert channel route %d account %d: %v", routeID, accountID, err)
	}
}

// ---- enrichAlertMessage unit tests ----

func TestEnrichAlertMessage_AccountScope(t *testing.T) {
	db := setupAlertTestDB(t)
	_, failingAccount := seedEnrichSite(t, db, "acme", "alice")
	_, betaAccount := seedEnrichSite(t, db, "beta", "bob")
	_, gammaAccount := seedEnrichSite(t, db, "gamma", "carol")

	gptRoute := seedEnrichRoute(t, db, "gpt-*", true)
	claudeRoute := seedEnrichRoute(t, db, "claude-*", true)

	seedEnrichChannel(t, db, gptRoute, failingAccount, true)
	seedEnrichChannel(t, db, claudeRoute, failingAccount, true)
	seedEnrichChannel(t, db, gptRoute, betaAccount, true)
	seedEnrichChannel(t, db, gptRoute, gammaAccount, true)
	seedEnrichChannel(t, db, claudeRoute, gammaAccount, true)

	message := enrichAlertMessage(db.DB,
		"alice @ acme token is invalid or expired (jwt expired)",
		alertEnrichmentScope{accountID: &failingAccount})

	want := strings.Join([]string{
		"alice @ acme token is invalid or expired (jwt expired)",
		"Affected routes: gpt-*, claude-*",
		"Alternative sites: gamma(2), beta(1)",
		"Panel: /observability?section=health",
	}, "\n")
	if message != want {
		t.Fatalf("message mismatch:\n got: %q\nwant: %q", message, want)
	}
}

func TestEnrichAlertMessage_ExcludesOwnSiteAndDisabledChannels(t *testing.T) {
	db := setupAlertTestDB(t)
	failingSite, failingAccount := seedEnrichSite(t, db, "acme", "alice")
	sameSiteAccount := seedEnrichAccount(t, db, failingSite, "mallory")
	_, betaAccount := seedEnrichSite(t, db, "beta", "bob")

	route := seedEnrichRoute(t, db, "gpt-*", true)
	seedEnrichChannel(t, db, route, failingAccount, true)
	// Same-site sibling account must NOT count as an alternative site.
	seedEnrichChannel(t, db, route, sameSiteAccount, true)
	// Disabled channel on another site must NOT count.
	seedEnrichChannel(t, db, route, betaAccount, false)

	message := enrichAlertMessage(db.DB, "base line", alertEnrichmentScope{accountID: &failingAccount})
	if !strings.Contains(message, "Affected routes: gpt-*") {
		t.Fatalf("missing affected routes: %q", message)
	}
	if !strings.Contains(message, "Alternative sites: none") {
		t.Fatalf("expected no alternative sites (same site + disabled channel), got: %q", message)
	}
}

func TestEnrichAlertMessage_NoWiring(t *testing.T) {
	db := setupAlertTestDB(t)
	_, orphanAccount := seedEnrichSite(t, db, "acme", "alice")

	message := enrichAlertMessage(db.DB, "base line", alertEnrichmentScope{accountID: &orphanAccount})
	want := strings.Join([]string{
		"base line",
		"Affected routes: none",
		"Alternative sites: none",
		"Panel: /observability?section=health",
	}, "\n")
	if message != want {
		t.Fatalf("message mismatch:\n got: %q\nwant: %q", message, want)
	}
}

func TestEnrichAlertMessage_Truncation(t *testing.T) {
	db := setupAlertTestDB(t)
	_, failingAccount := seedEnrichSite(t, db, "acme", "alice")

	var routes []int64
	for _, pattern := range []string{"r1", "r2", "r3", "r4", "r5"} {
		routes = append(routes, seedEnrichRoute(t, db, pattern, true))
		seedEnrichChannel(t, db, routes[len(routes)-1], failingAccount, true)
	}
	for _, siteName := range []string{"beta", "delta", "gamma", "zeta"} {
		_, altAccount := seedEnrichSite(t, db, siteName, siteName+"-user")
		seedEnrichChannel(t, db, routes[0], altAccount, true)
	}

	message := enrichAlertMessage(db.DB, "base line", alertEnrichmentScope{accountID: &failingAccount})
	if !strings.Contains(message, "Affected routes: r1, r2, r3 (5 routes in total)") {
		t.Fatalf("expected truncated route list, got: %q", message)
	}
	// Sites are ordered by count DESC then name ASC; all have count 1.
	if !strings.Contains(message, "Alternative sites: beta(1), delta(1) (4 sites in total)") {
		t.Fatalf("expected truncated site list, got: %q", message)
	}
	if lines := strings.Split(message, "\n"); len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %q", len(lines), message)
	}
}

func TestEnrichAlertMessage_ModelScope(t *testing.T) {
	db := setupAlertTestDB(t)
	_, betaAccount := seedEnrichSite(t, db, "beta", "bob")

	globRoute := seedEnrichRoute(t, db, "gpt-*", true)
	seedEnrichRoute(t, db, "gpt-4", true)         // exact — must not match model "gpt-4o"
	seedEnrichRoute(t, db, "claude-*", true)      // unrelated — must not match
	seedEnrichRoute(t, db, "gpt-disabled", false) // disabled — excluded from dispatch surface
	seedEnrichChannel(t, db, globRoute, betaAccount, true)

	message := enrichAlertMessage(db.DB, "model=gpt-4o, reason=all channels exhausted",
		alertEnrichmentScope{model: "gpt-4o"})

	want := strings.Join([]string{
		"model=gpt-4o, reason=all channels exhausted",
		"Affected routes: gpt-*",
		"Alternative sites: beta(1)",
		"Panel: /observability?section=health",
	}, "\n")
	if message != want {
		t.Fatalf("message mismatch:\n got: %q\nwant: %q", message, want)
	}
}

func TestEnrichAlertMessage_ModelScopeNoMatch(t *testing.T) {
	db := setupAlertTestDB(t)
	seedEnrichRoute(t, db, "gpt-*", true)

	message := enrichAlertMessage(db.DB, "base line", alertEnrichmentScope{model: "claude-3-opus"})
	if !strings.Contains(message, "Affected routes: none") {
		t.Fatalf("expected no matching routes, got: %q", message)
	}
	if !strings.Contains(message, "Alternative sites: none") {
		t.Fatalf("expected no alternative sites, got: %q", message)
	}
}

func TestEnrichAlertMessage_NilDBDegrades(t *testing.T) {
	base := "原始消息"
	if got := enrichAlertMessage(nil, base, alertEnrichmentScope{model: "gpt-4o"}); got != base {
		t.Fatalf("nil db must degrade to base, got %q", got)
	}
}

func TestEnrichAlertMessage_QueryFailureDegrades(t *testing.T) {
	db := setupAlertTestDB(t)
	db.Close() // closed pool forces query errors

	base := "原始消息"
	got := enrichAlertMessage(db.DB, base, alertEnrichmentScope{model: "gpt-4o"})
	if got != base {
		t.Fatalf("query failure must degrade to base, got %q", got)
	}
}

// ---- Report* integration tests (enriched message lands in events) ----

func TestReportLowBalance_EnrichedEventMessage(t *testing.T) {
	db := setupAlertTestDB(t)
	cfg := &config.Config{AuthToken: "a", ProxyToken: "p"}
	_, accountID := seedEnrichSite(t, db, "acme", "alice")
	routeID := seedEnrichRoute(t, db, "gpt-*", true)
	seedEnrichChannel(t, db, routeID, accountID, true)

	uname := "alice"
	site := "acme"
	ReportLowBalance(cfg, db.DB, LowBalanceParams{
		AccountID: accountID, Username: &uname, SiteName: &site,
		Balance: 0.42, Threshold: 1.0,
	})

	var message string
	if err := db.Get(&message,
		`SELECT message FROM events WHERE type = 'balance' AND related_id = ?`, accountID); err != nil {
		t.Fatalf("load event: %v", err)
	}
	if !strings.Contains(message, "alice @ acme low balance: current $0.42 (threshold $1.00)") {
		t.Fatalf("missing base message: %q", message)
	}
	if !strings.Contains(message, "Affected routes: gpt-*") {
		t.Fatalf("missing affected routes: %q", message)
	}
	if !strings.Contains(message, "Alternative sites: none") {
		t.Fatalf("expected no alternative sites: %q", message)
	}
	if !strings.Contains(message, "Panel: /observability?section=health") {
		t.Fatalf("missing panel link: %q", message)
	}
	if lines := strings.Split(message, "\n"); len(lines) > 6 {
		t.Fatalf("message has %d lines, want <= 6: %q", len(lines), message)
	}
}

func TestReportTokenExpired_EnrichedEventMessage(t *testing.T) {
	db := setupAlertTestDB(t)
	cfg := &config.Config{AuthToken: "a", ProxyToken: "p"}
	_, accountID := seedEnrichSite(t, db, "acme", "alice")
	routeID := seedEnrichRoute(t, db, "gpt-*", true)
	seedEnrichChannel(t, db, routeID, accountID, true)

	uname := "alice"
	site := "acme"
	ReportTokenExpired(cfg, db.DB, TokenExpiredParams{
		AccountID: accountID, Username: &uname, SiteName: &site,
		Detail: "jwt expired",
	})

	var message string
	if err := db.Get(&message,
		`SELECT message FROM events WHERE type = 'token' AND related_id = ?`, accountID); err != nil {
		t.Fatalf("load event: %v", err)
	}
	if !strings.Contains(message, "alice @ acme token is invalid or expired") {
		t.Fatalf("missing base message: %q", message)
	}
	if !strings.Contains(message, "Affected routes: gpt-*") {
		t.Fatalf("missing affected routes: %q", message)
	}
	if !strings.Contains(message, "Panel: /observability?section=health") {
		t.Fatalf("missing panel link: %q", message)
	}
}

func TestReportProxyAllFailed_EnrichedEventMessage(t *testing.T) {
	db := setupAlertTestDB(t)
	cfg := &config.Config{AuthToken: "a", ProxyToken: "p"}
	_, betaAccount := seedEnrichSite(t, db, "beta", "bob")
	globRoute := seedEnrichRoute(t, db, "gpt-*", true)
	seedEnrichChannel(t, db, globRoute, betaAccount, true)

	ReportProxyAllFailed(cfg, db.DB, ProxyAllFailedParams{
		Model: "gpt-4o", Reason: "all channels exhausted",
	})

	var message string
	if err := db.Get(&message,
		`SELECT message FROM events WHERE type = 'proxy' ORDER BY id DESC LIMIT 1`); err != nil {
		t.Fatalf("load event: %v", err)
	}
	if !strings.Contains(message, "model=gpt-4o, reason=all channels exhausted") {
		t.Fatalf("missing base message: %q", message)
	}
	if !strings.Contains(message, "Affected routes: gpt-*") {
		t.Fatalf("missing affected routes: %q", message)
	}
	if !strings.Contains(message, "Alternative sites: beta(1)") {
		t.Fatalf("missing alternative sites: %q", message)
	}
	if !strings.Contains(message, "Panel: /observability?section=health") {
		t.Fatalf("missing panel link: %q", message)
	}
	if lines := strings.Split(message, "\n"); len(lines) > 6 {
		t.Fatalf("message has %d lines, want <= 6: %q", len(lines), message)
	}
}
