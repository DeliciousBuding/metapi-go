package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestUserBudgetSharedPoolAndSnapshot(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			if _, err := f.s.ReserveUserBudget(ctx, "req_zero", f.user.ID, f.groupA.ID, 1); !errors.Is(err, ErrUserBudgetExceeded) {
				t.Fatalf("zero default: %v", err)
			}
			if _, err := f.s.SetUserBudget(ctx, f.user.ID, 1, false); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.SetUserGroupPricing(ctx, f.groupA.ID, 2); err != nil {
				t.Fatal(err)
			}
			first, err := f.s.ReserveUserBudget(ctx, "req_first", f.user.ID, f.groupA.ID, 100000)
			if err != nil {
				t.Fatal(err)
			}
			if first.ReservedUnits != 200000 {
				t.Fatalf("hold %+v", first)
			}
			if _, err = f.s.ReserveUserBudget(ctx, "req_second", f.user.ID, f.groupA.ID, 100000); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.ReserveUserBudget(ctx, "req_first", f.user.ID, f.groupA.ID, 900000); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.SetUserGroupPricing(ctx, f.groupA.ID, 8); err != nil {
				t.Fatal(err)
			}
			if err = f.s.UpdateUser(ctx, f.user.ID, "Moved", f.groupB.ID, true); err != nil {
				t.Fatal(err)
			}
			tokens := int64(50000)
			if err = f.s.SettleUserBudget(ctx, "req_first", &tokens, true); err != nil {
				t.Fatal(err)
			}
			if err = f.s.SettleUserBudget(ctx, "req_first", &tokens, true); err != nil {
				t.Fatal(err)
			}
			if err = f.s.SettleUserBudget(ctx, "req_second", nil, false); err != nil {
				t.Fatal(err)
			}
			budget, err := f.s.GetUserBudget(ctx, f.user.ID)
			if err != nil || budget.Used != 0.1 || budget.Reserved != 0 {
				t.Fatalf("snapshot budget %+v %v", budget, err)
			}
			if _, err = f.s.ReserveUserBudget(ctx, "req_old_group", f.user.ID, f.groupA.ID, 10); !errors.Is(err, ErrUserBudgetConflict) {
				t.Fatalf("stale group: %v", err)
			}
			if _, err = f.s.ReserveUserBudget(ctx, "req_new_group", f.user.ID, f.groupB.ID, 200000); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.SetUserBudget(ctx, f.user.ID, 0, true); err != nil {
				t.Fatal(err)
			}
			budget, err = f.s.GetUserBudget(ctx, f.user.ID)
			if err != nil || budget.Used != 0.1 || budget.Reserved != 0.2 || !budget.Unlimited {
				t.Fatalf("budget edit reset usage %+v %v", budget, err)
			}
			if err = f.s.UpdateUser(ctx, f.user.ID, "Disabled", f.groupB.ID, false); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.ReserveUserBudget(ctx, "req_disabled", f.user.ID, f.groupB.ID, 1); err == nil {
				t.Fatal("disabled user allowed")
			}
		})
	}
}

func TestUserBudgetUnknownAndReconciliation(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			if _, err := f.s.SetUserBudget(ctx, f.user.ID, 1, false); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.ReserveUserBudget(ctx, "req_unknown", f.user.ID, f.groupA.ID, 100); err != nil {
				t.Fatal(err)
			}
			if err := f.s.SettleUserBudget(ctx, "req_unknown", nil, true); err != nil {
				t.Fatal(err)
			}
			budget, err := f.s.GetUserBudget(ctx, f.user.ID)
			if err != nil || budget.Reserved != 0.0001 || !budget.Frozen || budget.UnknownRequests != 1 {
				t.Fatalf("unknown %+v %v", budget, err)
			}
			if _, err = f.s.SetUserBudget(ctx, f.user.ID, 2, true); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.ReserveUserBudget(ctx, "req_frozen", f.user.ID, f.groupA.ID, 10); !errors.Is(err, ErrUserBudgetUnknown) {
				t.Fatalf("frozen unlimited: %v", err)
			}
			holds, err := f.s.ListUserBudgetHolds(ctx, f.user.ID, 0)
			if err != nil || len(holds) != 1 || holds[0].State != "unknown" {
				t.Fatalf("holds %+v %v", holds, err)
			}
			if err = f.s.ReconcileUserBudget(ctx, f.user.ID, "req_unknown", 250); err != nil {
				t.Fatal(err)
			}
			if err = f.s.ReconcileUserBudget(ctx, f.user.ID, "req_unknown", 250); err != nil {
				t.Fatal(err)
			}
			if err = f.s.ReconcileUserBudget(ctx, f.user.ID, "req_unknown", 0); !errors.Is(err, ErrUserBudgetConflict) {
				t.Fatalf("rewrite settled: %v", err)
			}
			budget, err = f.s.GetUserBudget(ctx, f.user.ID)
			if err != nil || budget.Frozen || budget.Reserved != 0 || budget.Used != 0.00025 {
				t.Fatalf("reconciled %+v %v", budget, err)
			}
			for i, tokens := range []int64{-1, math.MaxInt64} {
				id := fmt.Sprintf("req_bad_%d", i)
				if _, err = f.s.ReserveUserBudget(ctx, id, f.user.ID, f.groupA.ID, 10); err != nil {
					t.Fatal(err)
				}
				if err = f.s.SettleUserBudget(ctx, id, &tokens, true); err != nil {
					t.Fatal(err)
				}
				if err = f.s.ReconcileUserBudget(ctx, f.user.ID, id, 1); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = f.s.SetUserBudget(ctx, f.user.ID, 0.001, false); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.ReserveUserBudget(ctx, "req_overreported", f.user.ID, f.groupA.ID, 10); err != nil {
				t.Fatal(err)
			}
			over := int64(2000)
			if err = f.s.SettleUserBudget(ctx, "req_overreported", &over, true); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.ReserveUserBudget(ctx, "req_debt", f.user.ID, f.groupA.ID, 1); !errors.Is(err, ErrUserBudgetExceeded) {
				t.Fatalf("overreported debt not gated: %v", err)
			}
		})
	}
}

func TestUserBudgetConcurrentSharedPool(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			// Unlike the directory fixture, this test permits several PG sessions,
			// all with the same privately-owned schema.
			if dialect == DialectPostgres {
				var schema string
				if err := f.db.Get(&schema, `SELECT current_schema()`); err != nil {
					t.Fatal(err)
				}
				dsn := pgDSN()
				if strings.Contains(dsn, "?") {
					dsn += "&search_path=" + schema
				} else {
					dsn += "?search_path=" + schema
				}
				pool, err := Open(dialect, dsn, false)
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				f.s = NewGatewayUserStore(pool)
			}
			if _, err := f.s.SetUserBudget(ctx, f.user.ID, 0.001, false); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			var admitted atomic.Int64
			ids := make(chan string, 32)
			for i := range 32 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					id := fmt.Sprintf("req_concurrent_%d", i)
					_, err := f.s.ReserveUserBudget(ctx, id, f.user.ID, f.groupA.ID, 100)
					if err == nil {
						admitted.Add(1)
						ids <- id
					} else if !errors.Is(err, ErrUserBudgetExceeded) {
						t.Errorf("reserve: %v", err)
					}
				}()
			}
			close(start)
			wg.Wait()
			close(ids)
			if admitted.Load() != 10 {
				t.Fatalf("admitted %d want 10", admitted.Load())
			}
			budget, err := f.s.GetUserBudget(ctx, f.user.ID)
			if err != nil || budget.Reserved != 0.001 {
				t.Fatalf("reserved %+v %v", budget, err)
			}
			settleStart := make(chan struct{})
			tokens := int64(50)
			for id := range ids {
				for range 3 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-settleStart
						if err := f.s.SettleUserBudget(ctx, id, &tokens, true); err != nil {
							t.Errorf("settle: %v", err)
						}
					}()
				}
			}
			close(settleStart)
			wg.Wait()
			budget, err = f.s.GetUserBudget(ctx, f.user.ID)
			if err != nil || budget.Reserved != 0 || budget.Used != 0.0005 {
				t.Fatalf("settled %+v %v", budget, err)
			}
		})
	}
}

func TestUserBudgetSQLiteRestartKeepsHold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget.db")
	db, err := Open(DialectSQLite, path, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	s := NewGatewayUserStore(db)
	ctx := context.Background()
	group, err := s.CreateUserGroup(ctx, "recovery", "Recovery", true)
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.CreateUser(ctx, "restart", "", group.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetUserBudget(ctx, user.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	id := "req_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = s.ReserveUserBudget(ctx, id, user.ID, group.ID, 500); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(DialectSQLite, path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = NewGatewayUserStore(db)
	budget, err := s.GetUserBudget(ctx, user.ID)
	if err != nil || budget.Reserved != 0.0005 {
		t.Fatalf("restart released hold %+v %v", budget, err)
	}
	if err = s.ReconcileUserBudget(ctx, user.ID, id, 100); err != nil {
		t.Fatal(err)
	}
	budget, err = s.GetUserBudget(ctx, user.ID)
	if err != nil || budget.Reserved != 0 || budget.Used != 0.0001 {
		t.Fatalf("recovery %+v %v", budget, err)
	}
}

func TestUserBudgetPostgresReconnectKeepsHold(t *testing.T) {
	f := newGatewayUserFixture(t, DialectPostgres)
	ctx := context.Background()
	if _, err := f.s.SetUserBudget(ctx, f.user.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReserveUserBudget(ctx, "req_pg_restart", f.user.ID, f.groupA.ID, 500); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := f.db.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatal(err)
	}
	dsn := pgDSN()
	if strings.Contains(dsn, "?") {
		dsn += "&search_path=" + schema
	} else {
		dsn += "?search_path=" + schema
	}
	second, err := Open(DialectPostgres, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	users := NewGatewayUserStore(second)
	budget, err := users.GetUserBudget(ctx, f.user.ID)
	if err != nil || budget.Reserved != 0.0005 {
		t.Fatalf("reconnect %+v %v", budget, err)
	}
	if err = users.ReconcileUserBudget(ctx, f.user.ID, "req_pg_restart", 100); err != nil {
		t.Fatal(err)
	}
	budget, err = f.s.GetUserBudget(ctx, f.user.ID)
	if err != nil || budget.Reserved != 0 || budget.Used != 0.0001 {
		t.Fatalf("recovery %+v %v", budget, err)
	}
}

func TestUserBudgetSQLiteMultipleConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.db")
	first, err := Open(DialectSQLite, path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err = AutoMigrate(first); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	users := NewGatewayUserStore(first)
	g, err := users.CreateUserGroup(ctx, "multi", "Multi", true)
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.CreateUser(ctx, "multi-user", "", g.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = users.SetUserBudget(ctx, u.ID, 0.001, false); err != nil {
		t.Fatal(err)
	}
	second, err := Open(DialectSQLite, path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	stores := []*GatewayUserStore{users, NewGatewayUserStore(second)}
	var wg sync.WaitGroup
	start := make(chan struct{})
	var admitted atomic.Int64
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := stores[i%2].ReserveUserBudget(ctx, fmt.Sprintf("req_multiconn_%d", i), u.ID, g.ID, 100)
			if err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, ErrUserBudgetExceeded) {
				t.Errorf("reserve %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if admitted.Load() != 10 {
		t.Fatalf("admitted %d want10", admitted.Load())
	}
}
