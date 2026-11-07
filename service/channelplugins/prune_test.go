package channelplugins_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const pruneModule = `export const meta={apiVersion:2,kind:"provider",key:"prune",name:"Prune",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"]};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`

func TestPruneRequiresDurableContractAndNoPersistentReference(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			for _, durable := range []bool{false, true} {
				source := pruneModule
				if durable {
					source = strings.Replace(source, `apiVersion:2,`, `apiVersion:2,taskCapture:"durable",`, 1)
				}
				program, err := channelplugin.Compile(t.Context(), source)
				if err != nil {
					t.Fatal(err)
				}
				metadata, _ := json.Marshal(program.Manifest())
				tx, err := db.BeginTxx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				if err = store.LockChannelPluginBindings(t.Context(), tx); err == nil {
					err = store.ArchiveChannelPluginVersion(t.Context(), tx, store.ChannelPluginVersion{Key: "prune", Fingerprint: program.Fingerprint(), Source: source, Manifest: string(metadata)})
				}
				if err != nil {
					_ = tx.Rollback()
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				identity := fmt.Sprintf(`{"version":3,"driverKey":"prune","driverFingerprint":%q}`, program.Fingerprint())
				for _, vendorID := range []string{"", "completed-task"} {
					if _, err = db.Exec(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES('prune-ref',?,'','',?)`, vendorID, identity); err != nil {
						t.Fatal(err)
					}
					if err = channelplugins.PruneArchivedVersion(t.Context(), db.DB, "prune", program.Fingerprint()); !errors.Is(err, channelplugins.ErrVersionNotPrunable) {
						t.Fatalf("persistent reference did not protect archive: %v", err)
					}
					if _, err = db.Exec(`DELETE FROM proxy_video_tasks WHERE public_id='prune-ref'`); err != nil {
						t.Fatal(err)
					}
				}
				err = channelplugins.PruneArchivedVersion(t.Context(), db.DB, "prune", program.Fingerprint())
				if durable {
					if err != nil {
						t.Fatal(err)
					}
					if err = channelplugins.PruneArchivedVersion(t.Context(), db.DB, "prune", program.Fingerprint()); err != nil {
						t.Fatalf("lost-confirmation retry was not idempotent: %v", err)
					}
				} else if !errors.Is(err, channelplugins.ErrVersionNotPrunable) {
					t.Fatalf("zero legacy references granted unsafe deletion: %v", err)
				}
			}
		})
	}
}
