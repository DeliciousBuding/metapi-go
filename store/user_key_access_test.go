package store

import (
	"context"
	"database/sql"
	"slices"
	"testing"
)

func TestUserKeyAccessOwnershipAndUpgrade(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newGatewayUserFixture(t, dialect)
			ctx := context.Background()
			if err := f.s.GrantUserGroupRoute(ctx, f.groupA.ID, f.routeA); err != nil {
				t.Fatal(err)
			}
			check := func(group int64, routes ...int64) {
				t.Helper()
				gotGroup, gotRoutes, err := f.s.ResolveUserKeyAccess(ctx, f.user.ID)
				if err != nil || gotGroup != group || !slices.Equal(gotRoutes, routes) {
					t.Fatalf("snapshot: group=%d routes=%v err=%v", gotGroup, gotRoutes, err)
				}
			}
			check(f.groupA.ID, f.routeA)
			if err := f.s.UpdateUser(ctx, f.user.ID, "", f.groupB.ID, true); err != nil {
				t.Fatal(err)
			}
			check(f.groupB.ID)
			if err := f.s.GrantUserGroupRoute(ctx, f.groupB.ID, f.routeB); err != nil {
				t.Fatal(err)
			}
			check(f.groupB.ID, f.routeB)
			if err := f.s.UpdateUserGroup(ctx, f.groupB.ID, "Disabled", false); err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.s.ResolveUserKeyAccess(ctx, f.user.ID); err != sql.ErrNoRows {
				t.Fatalf("disabled group: %v", err)
			}
			// Discriminator and binding must agree in both database dialects.
			for _, query := range []string{
				`INSERT INTO downstream_api_keys (name,key,key_owner_type) VALUES ('bad','bad-null-user','user')`,
				`INSERT INTO downstream_api_keys (name,key,key_owner_type,owner_user_id) VALUES ('bad','bad-system-user','system',1)`,
				`INSERT INTO downstream_api_keys (name,key,key_owner_type,owner_user_id) VALUES ('bad','bad-zero-user','user',0)`,
				`INSERT INTO downstream_api_keys (name,key,key_owner_type) VALUES ('bad','bad-type','other')`,
			} {
				if _, err := f.db.Exec(query); err == nil {
					t.Fatalf("invalid ownership accepted: %s", query)
				}
			}
			// Emulate an old binary's table, then re-apply this additive step.
			for _, query := range []string{
				`DROP INDEX downstream_api_keys_user_idx`,
				`ALTER TABLE downstream_api_keys DROP COLUMN owner_user_id`,
				`ALTER TABLE downstream_api_keys DROP COLUMN key_owner_type`,
				`INSERT INTO downstream_api_keys (name,key,created_at,updated_at) VALUES ('legacy','synthetic-legacy-key','now','now')`,
			} {
				if _, err := f.db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := migrateUserKeyAccess(f.db); err != nil {
					t.Fatal(err)
				}
			}
			var key DownstreamAPIKey
			if err := f.db.Get(&key, `SELECT * FROM downstream_api_keys WHERE key='synthetic-legacy-key'`); err != nil {
				t.Fatal(err)
			}
			if key.KeyOwnerType != "system" || key.OwnerUserID != nil {
				t.Fatalf("legacy Key adopted: %+v", key)
			}
		})
	}
}
