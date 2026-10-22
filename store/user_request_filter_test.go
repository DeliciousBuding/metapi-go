package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestUserRequestFilters(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			now := time.Now().UTC().Format(time.RFC3339Nano)
			model, private := "Alpha_%!模型", "private-diagnostic"
			other, err := f.s.CreateUser(ctx, "filter-other", "Other", f.groupA.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range []GatewayRequest{
				{ID: "req_filter-a", OwnerUserID: &f.user.ID, ModelRequested: &model, TerminalStatus: "completed", ReasonCode: &private, ExternalRequestID: &private},
				{ID: "req_filter-b", OwnerUserID: &f.user.ID, ModelRequested: &model, TerminalStatus: "upstream_failed"},
				{ID: "filter-c", OwnerUserID: &f.user.ID, TerminalStatus: "completed"},
				{ID: "filter-foreign", OwnerUserID: &other.ID, TerminalStatus: "upstream_failed", ModelRequested: &model},
				{ID: "filter-unowned", TerminalStatus: "upstream_failed", ModelRequested: &model},
			} {
				entry.Transport, entry.StartedAt, entry.EndedAt = "http", now, now
				if err := InsertGatewayRequest(ctx, f.db, entry); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.s.SetUserBudget(ctx, f.user.ID, 1, false); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.ReserveUserBudget(ctx, "req_filter-a", f.user.ID, f.groupA.ID, 1); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.ReserveUserBudget(ctx, "req_filter-b", f.user.ID, f.groupA.ID, 1); err != nil {
				t.Fatal(err)
			}
			if err := f.s.SettleUserBudget(ctx, "req_filter-b", nil, true); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				filter UserRequestFilter
				count  int64
			}{
				{UserRequestFilter{}, 3},
				{UserRequestFilter{Search: "alpha"}, 2},
				{UserRequestFilter{Search: "_%!"}, 2},
				{UserRequestFilter{Search: "filter-b"}, 1},
				{UserRequestFilter{Search: private}, 0},
				{UserRequestFilter{Search: "' OR 1=1 --"}, 0},
				{UserRequestFilter{Status: "completed"}, 2},
				{UserRequestFilter{Search: "Alpha", Status: "upstream_failed", QuotaState: "unknown"}, 1},
				{UserRequestFilter{QuotaState: "pending"}, 1},
				{UserRequestFilter{QuotaState: "missing"}, 1},
			} {
				items, total, err := f.s.ListOwnRequestsFiltered(ctx, f.user.ID, 1, 1, tc.filter)
				if err != nil || total != tc.count || len(items) != int(min(tc.count, 1)) {
					t.Fatalf("filter %+v: total=%d items=%d err=%v", tc.filter, total, len(items), err)
				}
			}
			for _, filter := range []UserRequestFilter{{Status: "invalid"}, {QuotaState: "invalid"}, {Search: strings.Repeat("模", 201)}} {
				if _, _, err := f.s.ListOwnRequestsFiltered(ctx, f.user.ID, 1, 20, filter); err == nil {
					t.Fatalf("invalid filter accepted: %+v", filter)
				}
			}
		})
	}
}
