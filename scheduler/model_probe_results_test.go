package scheduler

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func seedProbeAccount(t *testing.T, db *store.DB, siteName, username string) (siteID, accountID int64) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'openai', 'active', ?, ?)`, siteName, "https://"+siteName+".example.com", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ = res.LastInsertId()
	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, 'tok', 'active', 1, ?, ?)`, siteID, username, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ = res.LastInsertId()
	return siteID, accountID
}

func TestProbeOnePersistsModelProbeResults(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	site1, account3 := seedProbeAccount(t, db, "s1", "u3")
	site2, account4 := seedProbeAccount(t, db, "s2", "u4")

	rec := &fakeRecorder{}
	s := NewModelProbeScheduler(&config.Config{})
	s.SetProbeExecutor(&fakeProbe{outcomes: map[int64]ProbeOutcome{
		7: {Status: "failure", HTTPStatus: 502, ErrorText: "bad gateway"},
		8: {Status: "success", LatencyMs: 31},
	}})
	s.SetHealthRecorder(rec)

	targetFail := ProbeTarget{ChannelID: 7, AccountID: account3, SiteID: site1, ModelName: "gpt-4o"}
	targetOK := ProbeTarget{ChannelID: 8, AccountID: account4, SiteID: site2, ModelName: "gpt-4o-mini"}

	if got := s.probeOne(targetFail, 3000, db); got != "failure" {
		t.Fatalf("failure probe status = %s", got)
	}
	if got := s.probeOne(targetOK, 3000, db); got != "success" {
		t.Fatalf("success probe status = %s", got)
	}

	var failStatus string
	var failErr string
	if err := db.Get(&failStatus, `SELECT status FROM model_probe_results WHERE account_id = ?`, account3); err != nil {
		t.Fatalf("load failure row: %v", err)
	}
	_ = db.Get(&failErr, `SELECT COALESCE(error_text, '') FROM model_probe_results WHERE account_id = ?`, account3)
	if failStatus != "failure" {
		t.Fatalf("persisted failure status = %q", failStatus)
	}
	if failErr != "bad gateway" {
		t.Fatalf("persisted failure error = %q", failErr)
	}

	var okStatus string
	var okLatency *float64
	if err := db.QueryRowx(`SELECT status, latency_ms FROM model_probe_results WHERE account_id = ?`, account4).Scan(&okStatus, &okLatency); err != nil {
		t.Fatalf("load success row: %v", err)
	}
	if okStatus != "success" {
		t.Fatalf("persisted success status = %q", okStatus)
	}
	if okLatency == nil || *okLatency != 31 {
		t.Fatalf("persisted latency = %v, want 31", okLatency)
	}
}

func TestProbeOneWithoutDBDoesNotPersist(t *testing.T) {
	rec := &fakeRecorder{}
	s := NewModelProbeScheduler(&config.Config{})
	s.SetProbeExecutor(&fakeProbe{outcomes: map[int64]ProbeOutcome{
		9: {Status: "success", LatencyMs: 5},
	}})
	s.SetHealthRecorder(rec)

	if got := s.probeOne(ProbeTarget{ChannelID: 9, AccountID: 1, SiteID: 1, ModelName: "m"}, 3000); got != "success" {
		t.Fatalf("status = %s", got)
	}
	if len(rec.successCalls) != 1 {
		t.Fatalf("success calls = %d, want 1", len(rec.successCalls))
	}
}
