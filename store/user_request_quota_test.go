package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestUserRequestQuotaSnapshots(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := f.s.SetUserBudget(ctx, f.user.ID, 1, false)
			must(err)
			_, err = f.s.SetUserGroupPricing(ctx, f.groupA.ID, 2)
			must(err)
			key, err := f.s.CreateOwnKey(ctx, f.user.ID, "Fixture", "fixture-quota-key", []string{"*"}, nil, nil, nil)
			must(err)
			_, err = f.db.Exec(`UPDATE downstream_api_keys SET used_cost=3,used_requests=31 WHERE id=?`, key.ID)
			must(err)
			for _, id := range []string{"req_pending", "req_settled", "req_zero", "req_released", "req_unknown"} {
				_, err := f.s.ReserveUserBudget(ctx, id, f.user.ID, f.groupA.ID, 3000)
				must(err)
			}
			_, err = f.s.SetUserGroupPricing(ctx, f.groupA.ID, 0.5)
			must(err)
			_, err = f.s.ReserveUserBudget(ctx, "req_rounded", f.user.ID, f.groupA.ID, 1)
			must(err)
			// Changing both group and pricing after admission must not rewrite any
			// request's snapshot. A newly admitted request uses the new policy.
			_, err = f.s.SetUserGroupPricing(ctx, f.groupA.ID, 9)
			must(err)
			must(f.s.UpdateUser(ctx, f.user.ID, "Moved", f.groupB.ID, true))
			_, err = f.s.SetUserGroupPricing(ctx, f.groupB.ID, 7)
			must(err)
			_, err = f.s.ReserveUserBudget(ctx, "req_new_group", f.user.ID, f.groupB.ID, 3000)
			must(err)
			tokens, zero, one := int64(14), int64(0), int64(1)
			must(f.s.SettleUserBudget(ctx, "req_settled", &tokens, true))
			must(f.s.SettleUserBudget(ctx, "req_zero", &zero, true))
			must(f.s.SettleUserBudget(ctx, "req_released", nil, false))
			must(f.s.SettleUserBudget(ctx, "req_rounded", &one, true))
			must(f.s.SettleUserBudget(ctx, "req_new_group", &tokens, true))
			must(f.s.SettleUserBudget(ctx, "req_unknown", nil, true))
			charge, rounded, newCharge, free := 0.000028, 0.000001, 0.000098, 0.0
			want := map[string]*UserRequestQuota{
				"req_pending":   {"token_credits", "pending", nil, 2, 0.006, nil},
				"req_settled":   {"token_credits", "settled", &tokens, 2, 0.006, &charge},
				"req_zero":      {"token_credits", "settled", &zero, 2, 0.006, &free},
				"req_released":  {"token_credits", "released", nil, 2, 0.006, &free},
				"req_unknown":   {"token_credits", "unknown", nil, 2, 0.006, nil},
				"req_rounded":   {"token_credits", "settled", &one, 0.5, 0.000001, &rounded},
				"req_new_group": {"token_credits", "settled", &tokens, 7, 0.021, &newCharge},
				"req_legacy":    nil,
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			for id := range want {
				must(InsertGatewayRequest(ctx, f.db, GatewayRequest{ID: id, OwnerUserID: &f.user.ID, DownstreamAPIKeyID: &key.ID, Transport: "http", TerminalStatus: "completed", StartedAt: now, EndedAt: now}))
			}
			check := func() {
				t.Helper()
				// Exercise multiple pages including an empty trailing page. The
				// total counts requests, regardless of whether a ledger exists.
				seen := map[string]bool{}
				for page := 1; page <= 5; page++ {
					items, total, err := f.s.ListOwnRequests(ctx, f.user.ID, page, 2)
					must(err)
					if total != int64(len(want)) || len(items) > 2 || page == 5 && len(items) != 0 {
						t.Fatalf("page=%d items=%+v total=%d", page, items, total)
					}
					for _, item := range items {
						expected, ok := want[item.ID]
						if !ok || seen[item.ID] || !reflect.DeepEqual(item.Quota, expected) {
							t.Fatalf("request %s: quota=%+v want=%+v duplicate=%v", item.ID, item.Quota, expected, seen[item.ID])
						}
						seen[item.ID] = true
						detail, err := f.s.OwnRequest(ctx, f.user.ID, item.ID)
						must(err)
						if !reflect.DeepEqual(item, detail) {
							t.Fatalf("list/detail differ: %+v %+v", item, detail)
						}
					}
				}
				if len(seen) != len(want) {
					t.Fatalf("saw %d requests, want %d", len(seen), len(want))
				}
				usage, err := f.s.OwnUsage(ctx, f.user.ID)
				must(err)
				if usage.RecordedRequests != int64(len(want)) {
					t.Fatalf("retained requests: %+v", usage)
				}
			}
			check()
			usage, err := f.s.OwnUsage(ctx, f.user.ID)
			must(err)
			if usage.UsedRequests != 31 || usage.UsedCost != 3 || usage.KeyCount != 1 {
				t.Fatalf("changed legacy usage semantics: %+v", usage)
			}
			_, err = f.s.RotateOwnKey(ctx, f.user.ID, key.ID, "fixture-quota-rotated")
			must(err)
			check()
			must(f.s.DeleteOwnKey(ctx, f.user.ID, key.ID))
			check()
			usage, err = f.s.OwnUsage(ctx, f.user.ID)
			must(err)
			if usage.UsedRequests != 0 || usage.UsedCost != 0 || usage.KeyCount != 0 {
				t.Fatalf("changed deleted-Key usage semantics: %+v", usage)
			}
			_, err = f.db.Exec(`DELETE FROM gateway_requests WHERE id=?`, "req_legacy")
			must(err)
			usage, err = f.s.OwnUsage(ctx, f.user.ID)
			must(err)
			if usage.RecordedRequests != int64(len(want)-1) {
				t.Fatalf("retention not reflected: %+v", usage)
			}
		})
	}
}

func TestUserRequestQuotaOwnerIsolation(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			other, err := f.s.CreateUser(ctx, "bob", "Other", f.groupA.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			for i, owners := range []struct {
				request *int64
				ledger  int64
			}{
				{&f.user.ID, other.ID}, // Own request must not expose a foreign ledger.
				{&other.ID, f.user.ID}, // Own ledger cannot grant access to a request.
				{nil, f.user.ID},       // Nor can it establish missing request ownership.
				{&other.ID, other.ID},
			} {
				id := fmt.Sprintf("req_owner_%d", i)
				if err := InsertGatewayRequest(ctx, f.db, GatewayRequest{ID: id, OwnerUserID: owners.request, Transport: "http", TerminalStatus: "completed", StartedAt: now, EndedAt: now}); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`INSERT INTO gateway_user_budget_ledger(request_id,user_id,group_id,multiplier_units,reserved_units,charged_units,total_tokens,state,created_at) VALUES(?,?,?,2000000,6000,28,14,'settled',?)`, id, owners.ledger, f.groupA.ID, now); err != nil {
					t.Fatal(err)
				}
				if i > 0 {
					if _, err := f.s.OwnRequest(ctx, f.user.ID, id); !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("foreign/ownerless request %q: %v", id, err)
					}
				}
			}
			items, total, err := f.s.ListOwnRequests(ctx, f.user.ID, 1, 100)
			if err != nil || total != 1 || len(items) != 1 || items[0].ID != "req_owner_0" || items[0].Quota != nil {
				t.Fatalf("owner list=%+v total=%d err=%v", items, total, err)
			}
			detail, err := f.s.OwnRequest(ctx, f.user.ID, "req_owner_0")
			if err != nil || !reflect.DeepEqual(detail, items[0]) {
				t.Fatalf("mismatched ledger detail=%+v err=%v", detail, err)
			}
			usage, err := f.s.OwnUsage(ctx, f.user.ID)
			if err != nil || usage.RecordedRequests != 1 {
				t.Fatalf("owner usage=%+v err=%v", usage, err)
			}
		})
	}
}

func TestUserRequestQuotaReadErrors(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if err := InsertGatewayRequest(ctx, f.db, GatewayRequest{ID: "req_errors", OwnerUserID: &f.user.ID, Transport: "http", TerminalStatus: "completed", StartedAt: now, EndedAt: now}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`INSERT INTO gateway_user_budget_ledger(request_id,user_id,group_id,multiplier_units,reserved_units,state,created_at) VALUES('req_errors',?,?,1000000,0,'pending',?)`, f.user.ID, f.groupA.ID, now); err != nil {
				t.Fatal(err)
			}
			readFails := func() {
				t.Helper()
				if _, err := f.s.OwnRequest(ctx, f.user.ID, "req_errors"); err == nil || errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("detail hid read failure: %v", err)
				}
				if items, total, err := f.s.ListOwnRequests(ctx, f.user.ID, 1, 20); err == nil || items != nil || total != 0 {
					t.Fatalf("list hid read failure: %+v %d %v", items, total, err)
				}
			}
			for _, column := range []string{"total_tokens", "charged_units"} {
				if _, err := f.db.Exec(`UPDATE gateway_user_budget_ledger SET ` + column + `=-1`); err != nil {
					t.Fatal(err)
				}
				readFails()
				if _, err := f.db.Exec(`UPDATE gateway_user_budget_ledger SET ` + column + `=NULL`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.db.Exec(`ALTER TABLE gateway_user_budget_ledger RENAME TO unavailable_quota_ledger`); err != nil {
				t.Fatal(err)
			}
			readFails()
			if _, err := f.db.Exec(`ALTER TABLE gateway_requests RENAME TO unavailable_requests`); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.OwnUsage(ctx, f.user.ID); err == nil {
				t.Fatal("usage hid retained-record count failure")
			}
		})
	}
}
