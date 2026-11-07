package channelplugins_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestTaskDriverArchivesOnlyCapturedVersionsAndRestoresOldDefaults(t *testing.T) {
	const module = `export const meta={apiVersion:2,kind:"provider",key:"task",name:"Task",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"],options:[{key:"mode",label:"Mode",type:"string",default:"old"}]};export function submit(i){return {method:"POST",body:i.body};}export function query(i){return {method:"GET",headers:{"X-Mode":i.options.mode}};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			if _, err := channelplugins.Save(t.Context(), db.DB, "task", module, true, 0); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.Get(&count, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil || count != 0 {
				t.Fatal("installation archived an unused program")
			}
			binding := store.DirectProviderBinding{Key: "task", Protocols: []string{"video"}}
			old, err := channelplugins.New(db.DB).ResolveTaskDriver(t.Context(), binding, "video", "api_key", "")
			if err != nil {
				t.Fatal(err)
			}
			upgraded := strings.Replace(module, `default:"old"`, `default:"new"`, 1)
			if _, err = channelplugins.Save(t.Context(), db.DB, "task", upgraded, true, 1); err != nil {
				t.Fatal(err)
			}
			manager := channelplugins.New(db.DB)
			restored, err := manager.ResolveTaskDriver(t.Context(), binding, "video", "api_key", old.Fingerprint())
			if err != nil || restored.Fingerprint() != old.Fingerprint() {
				t.Fatalf("captured version not restored: %v", err)
			}
			current, err := manager.ResolveTaskDriver(t.Context(), binding, "video", "api_key", "")
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range []struct {
				call channelplugins.DriverInvocation
				want string
			}{{restored, "old"}, {current, "new"}} {
				raw, err := item.call.Call(t.Context(), "query", channelplugin.DriverInput{CredentialKind: "none", Options: map[string]json.RawMessage{"mode": json.RawMessage(`"forged"`)}})
				if err != nil || !strings.Contains(string(raw), item.want) || strings.Contains(string(raw), "forged") {
					t.Fatalf("restored defaults/identity overwritten: %s %v", raw, err)
				}
			}
			if err = db.Get(&count, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil || count != 2 {
				t.Fatalf("unexpected archive count: %d %v", count, err)
			}
			if _, err = manager.ResolveTaskDriver(t.Context(), binding, "video", "none", old.Fingerprint()); !errors.Is(err, channelplugins.ErrBinding) {
				t.Fatalf("archive bypassed current credential intersection: %v", err)
			}
			if _, err = channelplugins.Save(t.Context(), db.DB, "task", upgraded, false, 2); err != nil {
				t.Fatal(err)
			}
			if _, err = manager.ResolveTaskDriver(t.Context(), binding, "video", "api_key", old.Fingerprint()); !errors.Is(err, channelplugins.ErrUnavailable) {
				t.Fatalf("archive bypassed disable: %v", err)
			}
			if err = store.DeleteChannelPlugin(t.Context(), db.DB, "task", 3); err != nil {
				t.Fatal(err)
			}
			if _, err = manager.ResolveTaskDriver(t.Context(), binding, "video", "api_key", old.Fingerprint()); !errors.Is(err, channelplugins.ErrUnavailable) {
				t.Fatalf("archive bypassed uninstall: %v", err)
			}
		})
	}
}
