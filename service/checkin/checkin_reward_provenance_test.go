package checkin

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/balance"
	"github.com/deliciousbuding/metapi-go/store"
)

// #1275: a newly password-bound account has balance DEFAULT 0, but upstream
// quota is already 10,000,000 ($20). New API adds 1000 quota ($0.002), not $20.002.
// These tests use the real rc.34 response shape and the runner's real DB writes.
func TestCheckinAccount_RewardProvenance(t *testing.T) {
	const nativeReward = `{"success":true,"message":"签到成功","data":{"quota_awarded":1000,"checkin_date":"2026-09-07"}}`
	const missingReward = `{"success":true,"message":"签到成功"}`
	cases := []struct {
		name               string
		initialQuota       int64
		quotaDelta         int64
		response           string
		refreshBefore      bool
		nullBaseline       bool
		emptyRefreshMarker bool
		failPostBalance    bool
		wantReward         string
	}{
		{name: "new_account_native_reward", initialQuota: 10_000_000, quotaDelta: 1000, response: nativeReward, wantReward: "0.002"},
		{name: "new_account_unknown_reward_not_total_balance_or_zero", initialQuota: 10_000_000, quotaDelta: 1000, response: missingReward, wantReward: ""},
		{name: "null_baseline_is_unknown", initialQuota: 10_000_000, quotaDelta: 1000, response: missingReward, nullBaseline: true, wantReward: ""},
		{name: "empty_refresh_marker_is_not_proof", initialQuota: 10_000_000, quotaDelta: 1000, response: missingReward, emptyRefreshMarker: true, wantReward: ""},
		{name: "confirmed_balance_delta", initialQuota: 10_000_000, quotaDelta: 1000, response: missingReward, refreshBefore: true, wantReward: "0.002"},
		{name: "confirmed_zero_balance_is_valid", initialQuota: 0, quotaDelta: 1000, response: missingReward, refreshBefore: true, wantReward: "0.002"},
		{name: "one_quota_delta_keeps_decimal_units", initialQuota: 10_000_000, quotaDelta: 1, response: missingReward, refreshBefore: true, wantReward: "0.000002"},
		{name: "native_reward_wins_over_unrelated_balance_increase", initialQuota: 10_000_000, quotaDelta: 501000, response: nativeReward, refreshBefore: true, wantReward: "0.002"},
		{name: "native_reward_survives_failed_balance_refresh", initialQuota: 10_000_000, quotaDelta: 1000, response: nativeReward, failPostBalance: true, wantReward: "0.002"},
		{name: "native_zero_is_authoritative", initialQuota: 10_000_000, quotaDelta: 0, response: `{"success":true,"message":"签到成功","data":{"quota_awarded":0}}`, wantReward: "0"},
		{name: "native_zero_is_not_overwritten_by_other_income", initialQuota: 10_000_000, quotaDelta: 500000, response: `{"success":true,"message":"签到成功","data":{"quota_awarded":0}}`, refreshBefore: true, wantReward: "0"},
		{name: "balance_and_date_in_response_are_not_rewards", initialQuota: 10_000_000, quotaDelta: 1000, response: `{"success":true,"message":"balance: 20.002; check-in date: 2026-09-07","data":{"quota":10001000,"balance":20.002}}`, wantReward: ""},
		{name: "already_checked_in_success_with_zero_gain", initialQuota: 10_000_000, quotaDelta: 0, response: `{"success":false,"message":"今日已签到"}`, wantReward: ""},
		{name: "already_checked_in_date_is_not_a_reward", initialQuota: 10_000_000, quotaDelta: 0, response: `{"success":false,"message":"already checked in today (2026-09-07)"}`, refreshBefore: true, wantReward: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var quota atomic.Int64
			quota.Store(tc.initialQuota)
			var checkinCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/user/checkin":
					checkinCalls.Add(1)
					quota.Add(tc.quotaDelta)
					fmt.Fprint(w, tc.response)
				case r.Method == http.MethodGet && r.URL.Path == "/api/user/self":
					if tc.failPostBalance && checkinCalls.Load() > 0 {
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(w, `{"success":false,"message":"balance temporarily unavailable"}`)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"success": true, "data": map[string]any{"id": 17, "username": "reward-user", "quota": quota.Load(), "used_quota": 0},
					})
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)
			db, err := store.Open(store.DialectSQLite, ":memory:", false)
			if err != nil {
				t.Fatalf("open sqlite: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if err := store.AutoMigrate(db); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			now := time.Now().UTC().Format(time.RFC3339)
			siteRes, err := db.Exec("INSERT INTO sites (name,url,platform,status,created_at,updated_at) VALUES (?,?,'new-api','active',?,?)", "reward upstream", srv.URL, now, now)
			if err != nil {
				t.Fatalf("insert site: %v", err)
			}
			siteID, _ := siteRes.LastInsertId()
			// Deliberately omit balance and last_balance_refresh: this is the
			// real new-account DEFAULT 0, not an invented confirmed baseline.
			accountRes, err := db.Exec(`INSERT INTO accounts (site_id,username,access_token,status,checkin_enabled,extra_config,created_at,updated_at) VALUES (?,?,'test-dashboard-pat','active',TRUE,'{"platformUserId":17}',?,?)`, siteID, "reward-user", now, now)
			if err != nil {
				t.Fatalf("insert account: %v", err)
			}
			accountID, _ := accountRes.LastInsertId()
			var initialBalance *float64
			var initialRefresh *string
			if err := db.QueryRow("SELECT balance,last_balance_refresh FROM accounts WHERE id=?", accountID).Scan(&initialBalance, &initialRefresh); err != nil {
				t.Fatalf("read default account: %v", err)
			}
			if initialBalance == nil || *initialBalance != 0 || initialRefresh != nil {
				t.Fatal("fixture must reproduce the unrefreshed DEFAULT 0 account")
			}
			if tc.refreshBefore {
				// Obtain a trustworthy baseline through the same balance
				// service that owns the last_balance_refresh marker.
				if _, err := balance.RefreshBalance(&config.Config{}, db.DB, accountID); err != nil {
					t.Fatalf("confirm baseline: %v", err)
				}
			}
			if tc.nullBaseline {
				if _, err := db.Exec("UPDATE accounts SET balance=NULL WHERE id=?", accountID); err != nil {
					t.Fatal(err)
				}
			}
			if tc.emptyRefreshMarker {
				if _, err := db.Exec("UPDATE accounts SET last_balance_refresh='' WHERE id=?", accountID); err != nil {
					t.Fatal(err)
				}
			}
			result := CheckinAccount(&config.Config{}, db.DB, accountID, &CheckinOptions{ScheduleMode: "manual"})
			if !result.Success || result.Status != CheckinSuccess || result.Skipped {
				t.Fatalf("CheckinAccount = %+v; want success, never SKIP", result)
			}
			if result.Reward != tc.wantReward {
				t.Errorf("result.Reward = %q, want %q", result.Reward, tc.wantReward)
			}
			var logStatus string
			var logReward *string
			var failureReason *string
			if err := db.QueryRow("SELECT status,reward,failure_reason FROM checkin_logs WHERE account_id=? ORDER BY id DESC LIMIT 1", accountID).Scan(&logStatus, &logReward, &failureReason); err != nil {
				t.Fatalf("read checkin log: %v", err)
			}
			if logStatus != "success" || failureReason != nil || logReward == nil || *logReward != tc.wantReward {
				t.Errorf("persisted log status=%q reward=%v failure=%v; want success/reward %q/no failure", logStatus, logReward, failureReason, tc.wantReward)
			}
			if tc.wantReward == "0.002" || tc.wantReward == "0.000002" {
				if ParseCheckinRewardAmount(result.Reward) != float64(1000)/500000 && tc.wantReward == "0.002" {
					t.Error("monetary reward parser changed the native quota units")
				}
				if tc.wantReward == "0.000002" && ParseCheckinRewardAmount(result.Reward) != float64(1)/500000 {
					t.Error("small quota reward must not become a whole-dollar reward")
				}
			}
			if checkinCalls.Load() != 1 || quota.Load()-tc.initialQuota != tc.quotaDelta {
				t.Fatalf("upstream checkin calls=%d quotaDelta=%d, want 1/%d", checkinCalls.Load(), quota.Load()-tc.initialQuota, tc.quotaDelta)
			}
			if !tc.failPostBalance {
				var savedBalance float64
				var refreshedAt *string
				if err := db.QueryRow("SELECT balance,last_balance_refresh FROM accounts WHERE id=?", accountID).Scan(&savedBalance, &refreshedAt); err != nil {
					t.Fatal(err)
				}
				if math.Abs(savedBalance-float64(quota.Load())/500000) > 1e-9 || refreshedAt == nil || strings.TrimSpace(*refreshedAt) == "" {
					t.Fatal("post-checkin balance must still be refreshed and stamped independently of reward")
				}
			}
		})
	}
}
