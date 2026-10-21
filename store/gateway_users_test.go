package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func openGatewayUserTestDB(t *testing.T, dialect string) *DB {
	t.Helper()
	dsn := ":memory:"
	if dialect == DialectPostgres {
		skipIfNoPG(t)
		dsn = pgDSN()
	}
	db, err := Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if dialect == DialectPostgres {
		// Own a schema so tests never reset the caller's public schema.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		schema := "gateway_user_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := db.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := db.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Errorf("drop owned test schema: %v", err)
			}
		})
		if _, err := db.Exec("SET search_path TO " + schema); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

type gatewayUserFixture struct {
	db             *DB
	s              *GatewayUserStore
	groupA, groupB GatewayUserGroup
	user           GatewayUser
	routeA, routeB int64
}

func newGatewayUserFixture(t *testing.T, dialect string) gatewayUserFixture {
	t.Helper()
	db := openGatewayUserTestDB(t, dialect)
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	f := gatewayUserFixture{db: db, s: NewGatewayUserStore(db)}
	ctx := context.Background()
	var err error
	f.groupA, err = f.s.CreateUserGroup(ctx, " Alpha ", " 开发组 ", true)
	if err != nil {
		t.Fatal(err)
	}
	f.groupB, err = f.s.CreateUserGroup(ctx, "beta", "其他组", true)
	if err != nil {
		t.Fatal(err)
	}
	f.user, err = f.s.CreateUser(ctx, " Alice ", " 用户 ", f.groupA.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	for i, dest := range []*int64{&f.routeA, &f.routeB} {
		if err := db.Get(dest, `INSERT INTO token_routes (model_pattern, enabled, created_at, updated_at)
			VALUES (?, TRUE, 'now', 'now') RETURNING id`, fmt.Sprintf("fixture-route-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestGatewayUserCurrentGroupAuthorization(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			assertRoutes := func(expected ...int64) {
				t.Helper()
				got, err := f.s.ListActiveGatewayUserRouteIDs(ctx, f.user.ID)
				if err != nil || got == nil || !slices.Equal(got, expected) {
					t.Fatalf("current routes = %v %v, want %v", got, err, expected)
				}
			}
			if f.groupA.Slug != "alpha" || f.groupA.Name != "开发组" || f.user.Username != "alice" || f.user.DisplayName != "用户" {
				t.Fatalf("normalization: %+v %+v", f.groupA, f.user)
			}
			assertRoutes() // An active user without group grants has no access.
			for _, pair := range [][2]int64{{f.groupA.ID, f.routeA}, {f.groupB.ID, f.routeB}} {
				if err := f.s.GrantUserGroupRoute(ctx, pair[0], pair[1]); err != nil {
					t.Fatal(err)
				}
			}
			assertRoutes(f.routeA)
			if err := f.s.UpdateUser(ctx, f.user.ID, "Moved", f.groupB.ID, true); err != nil {
				t.Fatal(err)
			}
			active, err := f.s.GetActiveGatewayUser(ctx, f.user.ID)
			if err != nil || active.GroupID != f.groupB.ID || active.DisplayName != "Moved" || active.Username != "alice" {
				t.Fatalf("moved user = %+v %v", active, err)
			}
			assertRoutes(f.routeB) // No union with the former group.
			for _, state := range []struct {
				name, off, on string
				id            int64
			}{
				{"user", `UPDATE gateway_users SET enabled = FALSE WHERE id = ?`, `UPDATE gateway_users SET enabled = TRUE WHERE id = ?`, f.user.ID},
				{"group", `UPDATE gateway_user_groups SET enabled = FALSE WHERE id = ?`, `UPDATE gateway_user_groups SET enabled = TRUE WHERE id = ?`, f.groupB.ID},
				{"route", `UPDATE token_routes SET enabled = FALSE WHERE id = ?`, `UPDATE token_routes SET enabled = TRUE WHERE id = ?`, f.routeB},
			} {
				t.Run(state.name, func(t *testing.T) {
					if _, err := f.db.Exec(state.off, state.id); err != nil {
						t.Fatal(err)
					}
					assertRoutes()
					if state.name != "route" {
						if _, err := f.s.GetActiveGatewayUser(ctx, f.user.ID); !errors.Is(err, sql.ErrNoRows) {
							t.Fatalf("inactive identity = %v", err)
						}
					}
					if _, err := f.db.Exec(state.on, state.id); err != nil {
						t.Fatal(err)
					}
					assertRoutes(f.routeB)
				})
			}
			if err := f.s.RevokeUserGroupRoute(ctx, f.groupB.ID, f.routeB); err != nil {
				t.Fatal(err)
			}
			assertRoutes()
			if err := f.s.GrantUserGroupRoute(ctx, f.groupB.ID, f.routeB); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`DELETE FROM token_routes WHERE id = ?`, f.routeB); err != nil {
				t.Fatal(err)
			}
			grants, err := f.s.ListUserGroupRouteGrants(ctx, f.groupB.ID)
			if err != nil || len(grants) != 0 {
				t.Fatalf("deleted route retained grants: %v %v", grants, err)
			}
			assertRoutes()
			if err := f.s.UpdateUserGroup(ctx, f.groupA.ID, "Paused", false); err != nil {
				t.Fatal(err)
			}
			paused, err := f.s.CreateUser(ctx, "paused", "", f.groupA.ID, false)
			if err != nil || paused.Enabled {
				t.Fatalf("disabled preconfiguration = %+v %v", paused, err)
			}
			if _, err := f.s.GetActiveGatewayUser(ctx, paused.ID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("preconfigured disabled user is active: %v", err)
			}
		})
	}
}

func TestGatewayUserInputAndRelationships(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			for _, invalid := range []string{"", "has space", "bad\nname", strings.Repeat("x", 65), "x\x00"} {
				if _, err := f.s.CreateUser(ctx, invalid, "", f.groupA.ID, true); !errors.Is(err, ErrGatewayUserInvalid) {
					t.Fatalf("invalid username %q = %v", invalid, err)
				}
				if _, err := f.s.CreateUserGroup(ctx, invalid, "Name", true); !errors.Is(err, ErrGatewayUserInvalid) {
					t.Fatalf("invalid slug %q = %v", invalid, err)
				}
			}
			for _, invalid := range []string{"bad\x00name", strings.Repeat("中", 101), string([]byte{0xff})} {
				if err := f.s.UpdateUser(ctx, f.user.ID, invalid, f.groupA.ID, true); !errors.Is(err, ErrGatewayUserInvalid) {
					t.Fatalf("invalid display name = %v", err)
				}
			}
			if _, err := f.s.CreateUser(ctx, "ALICE", "", f.groupB.ID, true); err == nil {
				t.Fatal("duplicate normalized username accepted")
			}
			if _, err := f.s.CreateUserGroup(ctx, "ALPHA", "Another", true); err == nil {
				t.Fatal("duplicate normalized group slug accepted")
			}
			if _, err := f.s.CreateUser(ctx, "missing-group", "", 999999, true); err == nil {
				t.Fatal("user with missing group accepted")
			}
			if err := f.s.UpdateUser(ctx, f.user.ID, "Bad move", 999999, true); err == nil {
				t.Fatal("move to missing group accepted")
			}
			unchanged, err := f.s.GetUser(ctx, f.user.ID)
			if err != nil || unchanged != f.user {
				t.Fatalf("failed move changed user: %+v %v", unchanged, err)
			}
			for _, query := range []string{
				`INSERT INTO gateway_users (username, display_name, enabled, created_at, updated_at) VALUES ('no-group', '', TRUE, 'now', 'now')`,
				fmt.Sprintf(`INSERT INTO gateway_user_group_route_grants (group_id, route_id, created_at) VALUES (999999, %d, 'now')`, f.routeA),
				fmt.Sprintf(`INSERT INTO gateway_user_group_route_grants (group_id, route_id, created_at) VALUES (%d, 999999, 'now')`, f.groupA.ID),
			} {
				if _, err := f.db.Exec(query); err == nil {
					t.Fatal("invalid direct SQL relationship accepted")
				}
			}
			if _, err := f.db.Exec(`DELETE FROM gateway_user_groups WHERE id = ?`, f.groupA.ID); err == nil {
				t.Fatal("group deletion orphaned user")
			}
			if err := f.s.GrantUserGroupRoute(ctx, f.groupA.ID, f.routeA); err != nil {
				t.Fatal(err)
			}
			if err := f.s.GrantUserGroupRoute(ctx, f.groupA.ID, f.routeA); err == nil {
				t.Fatal("duplicate group grant accepted")
			}
			if err := f.s.GrantUserGroupRoute(ctx, f.groupA.ID, 999999); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("missing route grant = %v", err)
			}
			if err := f.s.GrantUserGroupRoute(ctx, 999999, f.routeA); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("missing group grant = %v", err)
			}
			if err := f.s.GrantUserGroupRoute(ctx, f.groupB.ID, f.routeB); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`DELETE FROM gateway_user_groups WHERE id = ?`, f.groupB.ID); err != nil {
				t.Fatal(err)
			}
			var remaining int
			if err := f.db.Get(&remaining, `SELECT count(*) FROM gateway_user_group_route_grants WHERE group_id = ?`, f.groupB.ID); err != nil || remaining != 0 {
				t.Fatalf("deleted unused group retained grants: %d %v", remaining, err)
			}
			if _, err := f.s.ListUserGroupRouteGrants(ctx, 999999); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("missing group list = %v", err)
			}
			if _, err := f.s.GetActiveGatewayUser(ctx, 999999); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("missing active user = %v", err)
			}
			if routes, err := f.s.ListActiveGatewayUserRouteIDs(ctx, 999999); err != nil || len(routes) != 0 {
				t.Fatalf("missing user's routes = %v %v", routes, err)
			}
			if _, err := f.s.ListActiveGatewayUserRouteIDs(ctx, 0); !errors.Is(err, ErrGatewayUserInvalid) {
				t.Fatalf("invalid user ID = %v", err)
			}
		})
	}
}

func TestGatewayUserDefaultGroupCreation(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			if _, err := f.s.CreateUser(ctx, "negative-group", "", -1, true); !errors.Is(err, ErrGatewayUserInvalid) {
				t.Fatalf("negative group ID = %v", err)
			}
			// A duplicate user fails after the default-group INSERT and must roll
			// back both operations, leaving the explicitly selected groups intact.
			if _, err := f.s.CreateUser(ctx, f.user.Username, "Duplicate", 0, true); err == nil {
				t.Fatal("duplicate user with default group accepted")
			}
			if n := countRows(t, f.db, "gateway_user_groups"); n != 2 {
				t.Fatalf("failed user creation left a default group: %d groups", n)
			}
			if n := countRows(t, f.db, "gateway_users"); n != 1 {
				t.Fatalf("failed user creation changed users: %d", n)
			}
			first, err := f.s.CreateUser(ctx, "first-default", "First", 0, true)
			if err != nil {
				t.Fatal(err)
			}
			group, err := f.s.GetUserGroup(ctx, first.GroupID)
			if err != nil || group.Slug != "default" || group.Name != "默认分组" || !group.Enabled || group.CreatedAt == "" || group.UpdatedAt == "" {
				t.Fatalf("created default group = %+v %v", group, err)
			}
			if routes, err := f.s.ListActiveGatewayUserRouteIDs(ctx, first.ID); err != nil || routes == nil || len(routes) != 0 {
				t.Fatalf("default group automatically authorized old routes: %v %v", routes, err)
			}
			second, err := f.s.CreateUser(ctx, "second-default", "Second", 0, true)
			if err != nil || second.GroupID != first.GroupID {
				t.Fatalf("default users did not share one group: %+v %v", second, err)
			}
			if err := f.s.UpdateUser(ctx, first.ID, "Invalid move", 0, true); !errors.Is(err, ErrGatewayUserInvalid) {
				t.Fatalf("update silently defaulted missing group: %v", err)
			}
			unchanged, err := f.s.GetUser(ctx, first.ID)
			if err != nil || unchanged != first {
				t.Fatalf("invalid move changed user: %+v %v", unchanged, err)
			}
			if err := f.s.UpdateUserGroup(ctx, group.ID, "Customized", false); err != nil {
				t.Fatal(err)
			}
			if err := f.s.GrantUserGroupRoute(ctx, group.ID, f.routeA); err != nil {
				t.Fatal(err)
			}
			before, err := f.s.GetUserGroup(ctx, group.ID)
			if err != nil {
				t.Fatal(err)
			}
			grantsBefore, err := f.s.ListUserGroupRouteGrants(ctx, group.ID)
			if err != nil {
				t.Fatal(err)
			}
			third, err := f.s.CreateUser(ctx, "third-default", "Third", 0, true)
			if err != nil || third.GroupID != group.ID {
				t.Fatalf("existing default group not reused: %+v %v", third, err)
			}
			after, err := f.s.GetUserGroup(ctx, group.ID)
			if err != nil || before != after {
				t.Fatalf("default group configuration overwritten: %+v %v", after, err)
			}
			grantsAfter, err := f.s.ListUserGroupRouteGrants(ctx, group.ID)
			if err != nil || !slices.Equal(grantsBefore, grantsAfter) {
				t.Fatalf("default group grants changed: %+v %v", grantsAfter, err)
			}
			if _, err := f.s.GetActiveGatewayUser(ctx, third.ID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("disabled default group allowed new user: %v", err)
			}
			if routes, err := f.s.ListActiveGatewayUserRouteIDs(ctx, third.ID); err != nil || len(routes) != 0 {
				t.Fatalf("disabled default group allowed routes: %v %v", routes, err)
			}
			if n := countRows(t, f.db, "gateway_user_groups"); n != 3 {
				t.Fatalf("default group was duplicated: %d groups", n)
			}
		})
	}
}

func TestGatewayUserUnavailableFailsClosed(t *testing.T) {
	ctx := context.Background()
	for _, s := range []*GatewayUserStore{nil, NewGatewayUserStore(nil)} {
		if _, err := s.GetActiveGatewayUser(ctx, 1); !errors.Is(err, ErrGatewayUserUnavailable) {
			t.Fatalf("nil identity store = %v", err)
		}
		if _, err := s.ListActiveGatewayUserRouteIDs(ctx, 1); !errors.Is(err, ErrGatewayUserUnavailable) {
			t.Fatalf("nil grant store = %v", err)
		}
		if _, err := s.ListUsers(ctx); !errors.Is(err, ErrGatewayUserUnavailable) {
			t.Fatalf("nil directory store = %v", err)
		}
	}
	f := newGatewayUserFixture(t, DialectSQLite)
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.GetActiveGatewayUser(ctx, f.user.ID); err == nil {
		t.Fatal("closed identity store allowed lookup")
	}
	if _, err := f.s.ListActiveGatewayUserRouteIDs(ctx, f.user.ID); err == nil {
		t.Fatal("closed grant store hid failure")
	}
}

func TestGatewayUserUpgradeAndRegistry(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := openGatewayUserTestDB(t, dialect)
			for _, table := range schemaTables {
				if strings.HasPrefix(table.name, "gateway_user") {
					continue
				}
				if _, err := db.Exec(table.build(dialect)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`INSERT INTO downstream_api_keys (name, key, enabled, supported_models, created_at, updated_at) VALUES ('existing', 'synthetic-existing-key', TRUE, '["fixture-model"]', 'now', 'now')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO token_routes (model_pattern, enabled, created_at, updated_at) VALUES ('existing-model', TRUE, 'now', 'now')`); err != nil {
				t.Fatal(err)
			}
			var routeBefore TokenRoute
			if err := db.Get(&routeBefore, `SELECT * FROM token_routes WHERE model_pattern = 'existing-model'`); err != nil {
				t.Fatal(err)
			}
			var before DownstreamAPIKey
			if err := db.Get(&before, `SELECT * FROM downstream_api_keys WHERE name = 'existing'`); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
			}
			var after DownstreamAPIKey
			if err := db.Get(&after, `SELECT * FROM downstream_api_keys WHERE name = 'existing'`); err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("upgrade changed legacy key: %+v %+v %v", before, after, err)
			}
			var routeAfter TokenRoute
			if err := db.Get(&routeAfter, `SELECT * FROM token_routes WHERE model_pattern = 'existing-model'`); err != nil || !reflect.DeepEqual(routeBefore, routeAfter) {
				t.Fatalf("upgrade changed legacy route: %+v %+v %v", routeBefore, routeAfter, err)
			}
			for _, table := range []string{"gateway_user_groups", "gateway_users", "gateway_user_group_route_grants"} {
				if n := countRows(t, db, table); n != 0 {
					t.Fatalf("upgrade seeded %s: %d", table, n)
				}
				if !slices.Contains(AllTableNames(), table) || !slices.Contains(FactoryResetTableNames(), table) || slices.Contains(BackupTableNames(), table) {
					t.Fatalf("incorrect registry inclusion for %s", table)
				}
			}
			var count int
			if err := db.Get(&count, `SELECT count(*) FROM schema_migrations WHERE version = 'gateway_users_v1'`); err != nil || count != 1 {
				t.Fatalf("migration journal = %d %v", count, err)
			}
			s := NewGatewayUserStore(db)
			group, err := s.CreateUserGroup(context.Background(), "default", "Customized", false)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.GrantUserGroupRoute(context.Background(), group.ID, routeBefore.ID); err != nil {
				t.Fatal(err)
			}
			grantsBefore, err := s.ListUserGroupRouteGrants(context.Background(), group.ID)
			if err != nil {
				t.Fatal(err)
			}
			// Simulate a missing migration journal entry with existing user data.
			if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 'gateway_users_v1'`); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrateGatewayUsers(db); err != nil {
				t.Fatal(err)
			}
			got, err := s.GetUserGroup(context.Background(), group.ID)
			if err != nil || got != group {
				t.Fatalf("migration overwrote existing default group: %+v %v", got, err)
			}
			grantsAfter, err := s.ListUserGroupRouteGrants(context.Background(), group.ID)
			if err != nil || !slices.Equal(grantsBefore, grantsAfter) {
				t.Fatalf("migration changed existing default group grants: %+v %v", grantsAfter, err)
			}
		})
	}
	for _, tc := range []struct {
		table string
		model any
	}{{"gateway_user_groups", GatewayUserGroup{}}, {"gateway_users", GatewayUser{}}, {"gateway_user_group_route_grants", GatewayUserGroupRouteGrant{}}} {
		var tags []string
		tp := reflect.TypeOf(tc.model)
		for i := range tp.NumField() {
			tags = append(tags, tp.Field(i).Tag.Get("db"))
		}
		cols, err := schemaColumns(tc.table)
		var names []string
		for _, col := range cols {
			names = append(names, col.name)
		}
		if err != nil || !slices.Equal(tags, names) {
			t.Fatalf("registry/model mismatch %s: %v %v", tc.table, tags, names)
		}
	}
}

func TestGatewayUserDirectoryLimit(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			tx, err := f.db.Beginx()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			for i := range GatewayUserDirectoryLimit {
				if _, err := tx.Exec(f.db.Rebind(`INSERT INTO gateway_user_groups (slug, name, enabled, created_at, updated_at) VALUES (?, 'Group', FALSE, 'now', 'now')`), fmt.Sprintf("group-%d", i)); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(f.db.Rebind(`INSERT INTO gateway_users (username, display_name, group_id, enabled, created_at, updated_at) VALUES (?, '', ?, FALSE, 'now', 'now')`), fmt.Sprintf("user-%d", i), f.groupA.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			groups, err := f.s.ListUserGroups(context.Background())
			if err != nil || len(groups) != GatewayUserDirectoryLimit || groups[0].ID != f.groupA.ID {
				t.Fatalf("group directory limit/order: %d %v", len(groups), err)
			}
			users, err := f.s.ListUsers(context.Background())
			if err != nil || len(users) != GatewayUserDirectoryLimit || users[0].ID != f.user.ID {
				t.Fatalf("user directory limit/order: %d %v", len(users), err)
			}
		})
	}
}

func TestGatewayUserDialectCopyAndReset(t *testing.T) {
	for _, sourceDialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(sourceDialect, func(t *testing.T) {
			skipIfNoPG(t)
			f := newGatewayUserFixture(t, sourceDialect)
			ctx := context.Background()
			if err := f.s.GrantUserGroupRoute(ctx, f.groupA.ID, f.routeA); err != nil {
				t.Fatal(err)
			}
			targetDialect := DialectSQLite
			if sourceDialect == DialectSQLite {
				targetDialect = DialectPostgres
			}
			target := openGatewayUserTestDB(t, targetDialect)
			if err := AutoMigrate(target); err != nil {
				t.Fatal(err)
			}
			snapshot, err := readAllTables(f.db.DB.DB)
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range buildStatements(snapshot) {
				if !strings.HasPrefix(statement.table, "gateway_user") && statement.table != "token_routes" {
					continue
				}
				query, values := buildInsertSQLite(statement)
				if targetDialect == DialectPostgres {
					query, values = buildInsertPG(statement)
				}
				if _, err := target.Exec(query, values...); err != nil {
					t.Fatal(err)
				}
			}
			copied := NewGatewayUserStore(target)
			user, err := copied.GetActiveGatewayUser(ctx, f.user.ID)
			if err != nil || user != f.user {
				t.Fatalf("copied identity = %+v %v", user, err)
			}
			routes, err := copied.ListActiveGatewayUserRouteIDs(ctx, f.user.ID)
			if err != nil || !slices.Equal(routes, []int64{f.routeA}) {
				t.Fatalf("copied grants = %v %v", routes, err)
			}
			deleted, err := FactoryReset(target.DB)
			if err != nil || deleted["gateway_users"] != 1 || deleted["gateway_user_groups"] != 2 || deleted["gateway_user_group_route_grants"] != 1 {
				t.Fatalf("reset gateway identity data = %v %v", deleted, err)
			}
			if routes, err := copied.ListActiveGatewayUserRouteIDs(ctx, f.user.ID); err != nil || len(routes) != 0 {
				t.Fatalf("reset retained authorization = %v %v", routes, err)
			}
		})
	}
}
