package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestUserKeyRotationPreservesState(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			expires, maxCost, maxRequests := "2000-01-01T00:00:00Z", 2.5, int64(7)
			key, err := f.s.CreateOwnKey(ctx, f.user.ID, "Restricted", "fixture-original-restricted", []string{"model-a"}, &expires, &maxCost, &maxRequests)
			if err != nil {
				t.Fatal(err)
			}
			// Expired, disabled and exhausted Keys can replace their secret, but
			// rotation must not reactivate them or replenish any budget.
			if _, err := f.db.Exec(`UPDATE downstream_api_keys SET enabled=FALSE,used_cost=3,used_requests=7,
				max_rpm=2,max_tpm=50,ip_allowlist='192.0.2.0/24',ip_blocklist='192.0.2.8',
				allowed_route_ids='[1]',access_policy='{"version":1}',last_used_at='2001-01-01T00:00:00Z'
				WHERE id=?`, key.ID); err != nil {
				t.Fatal(err)
			}
			var before, after DownstreamAPIKey
			if err := f.db.Get(&before, `SELECT * FROM downstream_api_keys WHERE id=?`, key.ID); err != nil {
				t.Fatal(err)
			}
			now := gatewayUserNow()
			if err := InsertGatewayRequest(ctx, f.db, GatewayRequest{ID: "rotation-history", OwnerUserID: &f.user.ID, DownstreamAPIKeyID: &key.ID, Transport: "http", TerminalStatus: "completed", StartedAt: now, EndedAt: now}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`INSERT INTO downstream_quota_usage (key_id,event_key,occurred_at,requests,total_tokens,cost) VALUES (?,'rotation-history',1,7,100,3)`, key.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.SetUserBudget(ctx, f.user.ID, 1, false); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.ReserveUserBudget(ctx, "req_rotation_pending", f.user.ID, f.groupA.ID, 100); err != nil {
				t.Fatal(err)
			}
			budgetBefore, err := f.s.GetUserBudget(ctx, f.user.ID)
			if err != nil {
				t.Fatal(err)
			}
			rotated, err := f.s.RotateOwnKey(ctx, f.user.ID, key.ID, "fixture-replacement-restricted")
			if err != nil {
				t.Fatal(err)
			}
			if rotated.Enabled || rotated.ID != key.ID || rotated.UsedRequests != 7 || rotated.UsedCost != 3 {
				t.Fatalf("rotation reset restricted Key: %+v", rotated)
			}
			if err := f.db.Get(&after, `SELECT * FROM downstream_api_keys WHERE id=?`, key.ID); err != nil {
				t.Fatal(err)
			}
			if after.Key != "fixture-replacement-restricted" || after.UpdatedAt == before.UpdatedAt {
				t.Fatal("secret or update timestamp was not replaced")
			}
			after.Key, after.UpdatedAt = before.Key, before.UpdatedAt
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rotation changed persistent Key identity, restrictions or usage")
			}
			if _, err := f.s.OwnRequest(ctx, f.user.ID, "rotation-history"); err != nil {
				t.Fatalf("rotation lost request history: %v", err)
			}
			var quotaRequests int64
			if err := f.db.Get(&quotaRequests, `SELECT requests FROM downstream_quota_usage WHERE key_id=? AND event_key='rotation-history'`, key.ID); err != nil || quotaRequests != 7 {
				t.Fatalf("rotation lost durable key quota: %v", err)
			}
			budgetAfter, err := f.s.GetUserBudget(ctx, f.user.ID)
			if err != nil || !reflect.DeepEqual(budgetBefore, budgetAfter) {
				t.Fatalf("rotation changed shared user budget: %v", err)
			}
		})
	}
}

func TestUserKeyRotationFailureIsAtomic(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			key, err := f.s.CreateOwnKey(ctx, f.user.ID, "Original", "fixture-original-atomic", []string{"*"}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertUnchanged := func() {
				t.Helper()
				var secret string
				if err := f.db.Get(&secret, `SELECT key FROM downstream_api_keys WHERE id=?`, key.ID); err != nil || secret != "fixture-original-atomic" {
					t.Fatalf("failed rotation changed existing secret: %v", err)
				}
			}
			for _, target := range []struct {
				table string
				id    int64
			}{{"gateway_users", f.user.ID}, {"gateway_user_groups", f.groupA.ID}} {
				if _, err := f.db.Exec("UPDATE "+target.table+" SET enabled=FALSE WHERE id=?", target.id); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.RotateOwnKey(ctx, f.user.ID, key.ID, "fixture-denied-replacement"); !errors.Is(err, sql.ErrNoRows) {
					t.Errorf("disabled %s rotation: %v", target.table, err)
				}
				assertUnchanged()
				if _, err := f.db.Exec("UPDATE "+target.table+" SET enabled=TRUE WHERE id=?", target.id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.s.CreateOwnKey(ctx, f.user.ID, "Collision", "fixture-collision-atomic", []string{"*"}, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.RotateOwnKey(ctx, f.user.ID, key.ID, "fixture-collision-atomic"); err == nil {
				t.Fatal("duplicate secret accepted")
			}
			assertUnchanged()
			if _, err := f.db.Exec(`UPDATE downstream_api_keys SET supported_models='invalid stored policy' WHERE id=?`, key.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.RotateOwnKey(ctx, f.user.ID, key.ID, "fixture-unreadable-replacement"); err == nil {
				t.Fatal("invalid response projection accepted")
			}
			assertUnchanged()
		})
	}
}

func TestUserKeyRotationConcurrentUsage(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			key, err := f.s.CreateOwnKey(ctx, f.user.ID, "Concurrent", "fixture-concurrent-original", []string{"*"}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			const rotations = 8
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := range rotations {
				wg.Go(func() {
					<-start
					if _, err := f.s.RotateOwnKey(ctx, f.user.ID, key.ID, fmt.Sprintf("fixture-concurrent-replacement-%d", i)); err != nil {
						t.Errorf("concurrent rotation: %v", err)
					}
				})
				wg.Go(func() {
					<-start
					if _, err := f.db.Exec(`UPDATE downstream_api_keys SET used_requests=used_requests+1,used_cost=used_cost+0.5 WHERE id=?`, key.ID); err != nil {
						t.Errorf("concurrent usage: %v", err)
					}
				})
			}
			close(start)
			wg.Wait()
			items, err := f.s.ListOwnKeys(ctx, f.user.ID)
			if err != nil || len(items) != 1 || items[0].ID != key.ID || items[0].UsedRequests != rotations || items[0].UsedCost != rotations*0.5 {
				t.Fatalf("rotation lost concurrent usage: %+v %v", items, err)
			}
			var active int
			if err := f.db.Get(&active, `SELECT COUNT(*) FROM downstream_api_keys WHERE key='fixture-concurrent-original'`); err != nil || active != 0 {
				t.Fatalf("original secret remains: count=%d err=%v", active, err)
			}
		})
	}
}
