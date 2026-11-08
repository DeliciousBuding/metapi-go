package admin

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const recoveryModule = `export const meta={apiVersion:2,kind:"provider",key:"recovery",name:"Recovery",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"],options:[{key:"mode",label:"Mode",type:"string",default:"old"}]};export function submit(i){return {method:"POST",body:i.body};}export function query(i){return {method:"GET",headers:{"X-Mode":i.options.mode}};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`

func TestChannelPluginArchiveRecoveryPreservesExactProgramAndCurrentEligibility(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			mux := catalogMux(db)
			catalogCall(t, mux, "PUT", "/api/channel-plugins/recovery", map[string]any{"source": recoveryModule, "enabled": true, "expectedRevision": 0}, 200)
			binding := store.DirectProviderBinding{Key: "recovery", Protocols: []string{"video"}}
			old, err := channelplugins.New(db.DB).ResolveTaskDriver(t.Context(), binding, "video", "api_key", "")
			if err != nil {
				t.Fatal(err)
			}
			identity := fmt.Sprintf(`{"version":3,"driverKey":"recovery","driverFingerprint":%q,"owner":"synthetic-private-owner"}`, old.Fingerprint())
			for i := 0; i < 2; i++ {
				if _, err = db.Exec(db.Rebind(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity,status_snapshot) VALUES(?,?,?,?,?,?)`), fmt.Sprintf("recovery-%d", i), "synthetic-private-task", "", "", identity, `{"status":"completed"}`); err != nil {
					t.Fatal(err)
				}
			}
			upgraded := strings.Replace(recoveryModule, `default:"old"`, `default:"new"`, 1)
			catalogCall(t, mux, "PUT", "/api/channel-plugins/recovery", map[string]any{"source": upgraded, "enabled": true, "expectedRevision": 1}, 200)
			if _, err = db.Exec(`DELETE FROM channel_plugin_versions`); err != nil {
				t.Fatal(err)
			}
			out := catalogRequest(mux, "GET", "/api/channel-plugin-versions", nil)
			if out.Code != 200 || out.Header().Get("Cache-Control") != noStoreCacheControl || !strings.Contains(out.Body.String(), `"available":false`) || !strings.Contains(out.Body.String(), `"taskReferences":2`) || strings.Contains(out.Body.String(), "synthetic-private") || strings.Contains(out.Body.String(), "export const") {
				t.Fatalf("unsafe or incomplete archive index: %d %s", out.Code, out.Body.String())
			}
			input := map[string]any{"key": "recovery", "fingerprint": old.Fingerprint(), "source": upgraded}
			catalogCall(t, mux, "POST", "/api/channel-plugin-versions/restore", input, 400)
			input["source"] = recoveryModule
			catalogCall(t, mux, "POST", "/api/channel-plugin-versions/restore", input, 200)
			catalogCall(t, mux, "POST", "/api/channel-plugin-versions/restore", input, 200)
			stored, err := store.LoadChannelPlugin(t.Context(), db.DB, "recovery")
			if err != nil || stored.Revision != 2 || stored.Source != upgraded || !stored.Enabled {
				t.Fatal("archive recovery modified the current installation")
			}
			restarted := channelplugins.New(db.DB)
			previous, err := restarted.ResolveTaskDriver(t.Context(), binding, "video", "api_key", old.Fingerprint())
			if err != nil {
				t.Fatal(err)
			}
			raw, err := previous.Call(t.Context(), "query", channelplugin.DriverInput{})
			if err != nil || !strings.Contains(string(raw), `"old"`) {
				t.Fatalf("recovery substituted current defaults: %s %v", raw, err)
			}
			catalogCall(t, mux, "PATCH", "/api/channel-plugins/recovery", map[string]any{"enabled": false, "expectedRevision": 2}, 200)
			if _, err = restarted.ResolveTaskDriver(t.Context(), binding, "video", "api_key", old.Fingerprint()); err == nil {
				t.Fatal("restored archive bypassed current disable")
			}
			out = catalogRequest(mux, "GET", "/api/channel-plugin-versions", nil)
			if !strings.Contains(out.Body.String(), `"available":true`) {
				t.Fatal("restored archive not visible")
			}
		})
	}
}

func TestChannelPluginArchiveRecoveryRejectsUnreferencedCodeAndUnreadableIdentity(t *testing.T) {
	db := catalogTestDB(t, store.DialectSQLite, ":memory:")
	mux := catalogMux(db)
	program, err := channelplugin.Compile(t.Context(), recoveryModule)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"key": "recovery", "fingerprint": program.Fingerprint(), "source": recoveryModule}
	catalogCall(t, mux, "POST", "/api/channel-plugin-versions/restore", input, 409)
	if _, err = db.Exec(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES('broken','private-task','','','{"version":3,"driverKey":"recovery"}')`); err != nil {
		t.Fatal(err)
	}
	out := catalogRequest(mux, "GET", "/api/channel-plugin-versions", nil)
	if out.Code != 500 || strings.Contains(out.Body.String(), "private-task") {
		t.Fatalf("unreadable reference became an empty successful index: %d %s", out.Code, out.Body.String())
	}
	input["source"] = `throw new Error("synthetic-private-code");` + recoveryModule
	out = catalogRequest(mux, "POST", "/api/channel-plugin-versions/restore", input)
	if out.Code != 400 || strings.Contains(out.Body.String(), "synthetic-private-code") {
		t.Fatal("compile failure leaked source")
	}
	var body map[string]any
	if json.Unmarshal(out.Body.Bytes(), &body) != nil {
		t.Fatal("non-JSON management error")
	}
}
