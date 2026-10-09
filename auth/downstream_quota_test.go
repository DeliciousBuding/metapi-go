package auth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDownstreamQuotaAdmissionAndIncompleteUsage(t *testing.T) {
	setupTestDB(t)
	db := testDB(t)
	requests := int64(2)
	tokens := int64(100)
	cost := 0.5
	q := &store.DownstreamQuota{Requests: &requests, TotalTokens: &tokens, Cost: &cost, Period: store.DownstreamQuotaPeriod{Type: "past_duration", PastDuration: &store.DownstreamQuotaDuration{Value: 1, Unit: "hour"}}}
	policy, _ := json.Marshal(store.DownstreamAccessPolicy{Quota: q})
	var id int64
	if err := db.QueryRowx(`INSERT INTO downstream_api_keys(name,key,supported_models,access_policy) VALUES ('quota','sk-quota','["*"]',?) RETURNING id`, string(policy)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	rt := &config.RuntimeSettings{}
	check := func(want bool, reason string) {
		t.Helper()
		r := AuthorizeDownstreamToken("sk-quota", rt)
		if r.OK != want || (!want && r.Reason != reason) {
			t.Fatalf("authorization OK=%v reason=%s", r.OK, r.Reason)
		}
	}
	check(true, "")
	if _, err := db.Exec(`INSERT INTO downstream_quota_usage(key_id,event_key,occurred_at,total_tokens,cost) VALUES (?,'outside',?,999,99)`, id, time.Now().Add(-2*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	check(true, "")
	a := int64(20)
	c := 0.1
	RecordManagedKeyQuotaUsage(id, &a, &c)
	check(true, "")
	RecordManagedKeyQuotaUsage(id, &a, &c)
	check(false, "over_quota")
	_, _ = db.Exec(`UPDATE downstream_api_keys SET enabled=? WHERE id=?`, false, id)
	check(false, "disabled")
	_, _ = db.Exec(`UPDATE downstream_api_keys SET enabled=? WHERE id=?`, true, id)
	_, _ = db.Exec(`DELETE FROM downstream_quota_usage WHERE key_id=?`, id)
	RecordManagedKeyQuotaUsage(id, nil, nil)
	check(false, "quota_usage_unknown")
	_, _ = db.Exec(`DELETE FROM downstream_quota_usage WHERE key_id=?`, id)
	gap := time.Now().Add(-30 * time.Minute).UnixMilli()
	q.HistoryMissingBefore = &gap
	policy, _ = json.Marshal(store.DownstreamAccessPolicy{Quota: q})
	_, _ = db.Exec(`UPDATE downstream_api_keys SET access_policy=? WHERE id=?`, string(policy), id)
	check(false, "quota_history_missing")
	gap = time.Now().Add(-2 * time.Hour).UnixMilli()
	policy, _ = json.Marshal(store.DownstreamAccessPolicy{Quota: q})
	_, _ = db.Exec(`UPDATE downstream_api_keys SET access_policy=? WHERE id=?`, string(policy), id)
	check(true, "")
	// Invalid persisted JSON must deny a matching global token too.
	_, _ = db.Exec(`UPDATE downstream_api_keys SET access_policy='{"quota":true}' WHERE id=?`, id)
	r := AuthorizeDownstreamToken("sk-quota", &config.RuntimeSettings{ProxyToken: "sk-quota"})
	if r.OK || r.Reason != "access_policy" {
		t.Fatal("invalid managed policy fell through to global token")
	}
}
