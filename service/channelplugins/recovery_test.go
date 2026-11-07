package channelplugins_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestArchivedVersionRecoverySerializesDuplicateRestores(t *testing.T) {
	const module = `export const meta={apiVersion:2,kind:"provider",key:"restore-race",name:"Restore",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"]};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			program, err := channelplugin.Compile(t.Context(), module)
			if err != nil {
				t.Fatal(err)
			}
			identity := fmt.Sprintf(`{"version":3,"driverKey":"restore-race","driverFingerprint":%q}`, program.Fingerprint())
			if _, err = db.Exec(db.Rebind(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES(?,?,?,?,?)`), "restore-race-task", "private-id", "", "", identity); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			failures := make(chan error, 8)
			for range 8 {
				wg.Go(func() {
					failures <- channelplugins.RestoreArchivedVersion(t.Context(), db.DB, "restore-race", program.Fingerprint(), module)
				})
			}
			wg.Wait()
			close(failures)
			for err = range failures {
				if err != nil {
					t.Fatal(err)
				}
			}
			var count int
			if err = db.Get(&count, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil || count != 1 {
				t.Fatalf("duplicate captures: %d %v", count, err)
			}
			if err = db.Get(&count, `SELECT COUNT(*) FROM channel_plugins`); err != nil || count != 0 {
				t.Fatal("archive recovery installed an untrusted current plugin")
			}
			row, err := store.LoadChannelPluginVersion(t.Context(), db.DB, "restore-race", program.Fingerprint())
			metadata, _ := json.Marshal(program.Manifest())
			if err != nil || row.Source != module || row.Manifest != string(metadata) {
				t.Fatal("immutable program replaced")
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if err = channelplugins.RestoreArchivedVersion(ctx, db.DB, "restore-race", program.Fingerprint(), module); err == nil {
				t.Fatal("canceled recovery claimed success")
			}
		})
	}
}
