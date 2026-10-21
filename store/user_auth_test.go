package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestUserAuthLifecycle(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			s := NewUserAuthStore(f.db)
			ctx := context.Background()
			now := time.Now().UTC()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			denied := func(err error) {
				t.Helper()
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("expected denied, got %v", err)
				}
			}
			must(s.IssueActivation(ctx, f.user.ID, "code-1", "v1", now, now.Add(time.Minute)))
			denied(s.Activate(ctx, "alice", "wrong", "hash", "v2", now))
			must(s.Activate(ctx, "alice", "code-1", "hash", "v2", now))
			denied(s.Activate(ctx, "alice", "code-1", "hash", "v3", now))
			credential, err := s.Credential(ctx, "alice")
			must(err)
			if credential.PasswordHash != "hash" || credential.Version != "v2" {
				t.Fatal("bad credential")
			}
			must(s.CreateSession(ctx, credential, "session-1", now, now.Add(time.Hour), now.Add(time.Minute)))
			_, err = s.ValidateSession(ctx, "session-1", now.Add(30*time.Second), time.Minute)
			must(err)
			_, err = s.ValidateSession(ctx, "session-1", now.Add(2*time.Minute), time.Minute)
			denied(err)
			must(s.CreateSession(ctx, credential, "session-2", now, now.Add(time.Minute), now.Add(time.Hour)))
			_, err = s.ValidateSession(ctx, "session-2", now.Add(time.Minute), time.Minute)
			denied(err)
			must(s.CreateSession(ctx, credential, "session-3", now, now.Add(time.Hour), now.Add(time.Hour)))
			must(s.ChangePassword(ctx, f.user.ID, "v2", "newhash", "v3", now))
			_, err = s.ValidateSession(ctx, "session-3", now, time.Minute)
			denied(err)
			denied(s.CreateSession(ctx, credential, "stale-login", now, now.Add(time.Hour), now.Add(time.Hour)))
			denied(s.ChangePassword(ctx, f.user.ID, "v2", "hash", "v4", now))
			credential, err = s.Credential(ctx, "alice")
			must(err)
			must(s.CreateSession(ctx, credential, "session-4", now, now.Add(time.Hour), now.Add(time.Hour)))
			for _, target := range []string{"gateway_users", "gateway_user_groups"} {
				id := f.user.ID
				if target == "gateway_user_groups" {
					id = f.groupA.ID
				}
				_, err = f.db.Exec("UPDATE "+target+" SET enabled=FALSE WHERE id=?", id)
				must(err)
				_, err = s.ValidateSession(ctx, "session-4", now, time.Minute)
				denied(err)
				_, err = s.Credential(ctx, "alice")
				denied(err)
				_, err = f.db.Exec("UPDATE "+target+" SET enabled=TRUE WHERE id=?", id)
				must(err)
			}
			must(s.IssueActivation(ctx, f.user.ID, "reset", "v5", now, now.Add(time.Minute)))
			_, err = s.ValidateSession(ctx, "session-4", now, time.Minute)
			denied(err)
			credential, err = s.Credential(ctx, "alice")
			must(err)
			if credential.PasswordHash != "" {
				t.Fatal("reset preserved password")
			}
			denied(s.Activate(ctx, "alice", "reset", "hash", "v6", now.Add(time.Minute)))
			for _, table := range []string{"gateway_user_credentials", "gateway_user_sessions"} {
				if BackupExcludedTables()[table] == "" {
					t.Errorf("%s included in portable backup", table)
				}
			}
		})
	}
}

func TestUserPortalOwnerQueries(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			other, err := f.s.CreateUser(ctx, "bob", "Other", f.groupA.ID, true)
			if err != nil {
				t.Fatal(err)
			}
	key, err := f.s.CreateOwnKey(ctx, f.user.ID, "Alice", "sk-alice-secret", []string{"*"}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
	otherKey, err := f.s.CreateOwnKey(ctx, other.ID, "Bob", "sk-bob-secret", []string{"*"}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if key.KeyMasked == "" || strings.Contains(key.KeyMasked, "alice-secret") {
				t.Fatal("plaintext key")
			}
			if err := f.s.DeleteOwnKey(ctx, f.user.ID, otherKey.ID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("foreign delete: %v", err)
			}
			items, err := f.s.ListOwnKeys(ctx, f.user.ID)
			if err != nil || len(items) != 1 || items[0].ID != key.ID {
				t.Fatalf("owner keys: %+v %v", items, err)
			}
			for _, pair := range [][2]int64{{key.ID, 3}, {otherKey.ID, 100}} {
				if _, err := f.db.Exec(`UPDATE downstream_api_keys SET used_cost=?,used_requests=? WHERE id=?`, float64(pair[1]), pair[1], pair[0]); err != nil {
					t.Fatal(err)
				}
			}
			usage, err := f.s.OwnUsage(ctx, f.user.ID)
			if err != nil || usage.UsedCost != 3 || usage.UsedRequests != 3 || usage.KeyCount != 1 {
				t.Fatalf("usage=%+v %v", usage, err)
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			for i, owner := range []*int64{&f.user.ID, &other.ID, nil} {
				if err := InsertGatewayRequest(ctx, f.db, GatewayRequest{ID: fmt.Sprint(i), OwnerUserID: owner, Transport: "http", TerminalStatus: "completed", StartedAt: now, EndedAt: now}); err != nil {
					t.Fatal(err)
				}
			}
			requests, total, err := f.s.ListOwnRequests(ctx, f.user.ID, 1, 1)
			if err != nil || total != 1 || len(requests) != 1 || requests[0].ID != "0" {
				t.Fatalf("requests=%+v total=%d %v", requests, total, err)
			}
			requests, total, err = f.s.ListOwnRequests(ctx, f.user.ID, 2, 1)
			if err != nil || total != 1 || len(requests) != 0 {
				t.Fatalf("empty page=%+v total=%d %v", requests, total, err)
			}
			for _, id := range []string{"1", "2", "absent"} {
				if _, err := f.s.OwnRequest(ctx, f.user.ID, id); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("request %q: %v", id, err)
				}
			}
			if err := f.s.DeleteOwnKey(ctx, f.user.ID, key.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.OwnRequest(ctx, f.user.ID, "0"); err != nil {
				t.Fatalf("key deletion erased historical ownership: %v", err)
			}
		})
	}
}
