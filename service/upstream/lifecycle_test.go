package upstream_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func lifecycleDatabase(t *testing.T, dialect string) (*store.DB, *store.DB, int64) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "lifecycle.db")
	if dialect == store.DialectPostgres {
		dsn = os.Getenv("PG_TEST_DSN")
		if dsn == "" {
			t.Skip("PG_TEST_DSN not set")
		}
	}
	db, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	if err = store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	writer, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	var id int64
	if err = db.Get(&id, db.Rebind(`INSERT INTO upstream_channels(origin_key,source_id,name,dialect,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES ('source',1,'fixture','generic',?,'https://fixture.invalid','/v1/chat/completions','/v1/responses','/v1/messages',?,'','[]','','') RETURNING id`), true, false); err != nil {
		t.Fatal(err)
	}
	return db, writer, id
}

func TestLifecycleLockSerializesConfirmationWithChildCreation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db, writer, id := lifecycleDatabase(t, dialect)
			before, err := upstream.PreviewDeletion(t.Context(), db.DB, upstream.KindChannel, id)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err = upstream.LockLifecycleTx(t.Context(), tx); err != nil {
				t.Fatal(err)
			}
			if _, err = upstream.PlanDeletionTx(t.Context(), tx, []upstream.Reference{{Kind: upstream.KindChannel, ID: id}}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			_, err = writer.ExecContext(ctx, writer.Rebind(`INSERT INTO upstream_models(origin_key,source_id,channel_id,name,enabled) VALUES ('native:local',-1,?,'concurrent-child',?)`), id, true)
			if err == nil {
				t.Fatal("child insertion interleaved after deletion closure confirmation")
			}
			if ctx.Err() == nil {
				t.Fatalf("concurrent insertion failed before the lock timeout: %v", err)
			}
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if _, err = writer.Exec(writer.Rebind(`INSERT INTO upstream_models(origin_key,source_id,channel_id,name,enabled) VALUES ('native:local',-1,?,'concurrent-child',?)`), id, true); err != nil {
				t.Fatal(err)
			}
			if _, err = upstream.Delete(t.Context(), db.DB, upstream.KindChannel, id, upstream.DeleteOptions{Cascade: true, ExpectedRevision: before.Revision}); !errors.Is(err, upstream.ErrRevisionMismatch) {
				t.Fatalf("stale confirmation after concurrent child creation: %v", err)
			}
		})
	}
}

func TestLifecycleDeleteFailureRollsBackClosure(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db, _, id := lifecycleDatabase(t, dialect)
			if _, err := db.Exec(db.Rebind(`INSERT INTO upstream_models(origin_key,source_id,channel_id,name,enabled) VALUES ('source',10,?,'model',?)`), id, true); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO external_source_ids(origin_key,entity_type,source_id,target_id) SELECT 'source','channel_models',10,id FROM upstream_models`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS lifecycle_delete_guard(channel_id INTEGER REFERENCES upstream_channels(id))`); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := db.Exec(`DROP TABLE lifecycle_delete_guard`); err != nil {
					t.Error(err)
				}
			})
			if _, err := db.Exec(db.Rebind(`INSERT INTO lifecycle_delete_guard(channel_id) VALUES (?)`), id); err != nil {
				t.Fatal(err)
			}
			before, err := upstream.PreviewDeletion(t.Context(), db.DB, upstream.KindChannel, id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = upstream.Delete(t.Context(), db.DB, upstream.KindChannel, id, upstream.DeleteOptions{Cascade: true, ExpectedRevision: before.Revision}); err == nil {
				t.Fatal("expected injected FK rejection")
			}
			after, err := upstream.PreviewDeletion(t.Context(), db.DB, upstream.KindChannel, id)
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != before.Revision {
				t.Fatal("failed root deletion left earlier child or mapping removals committed")
			}
		})
	}
}
