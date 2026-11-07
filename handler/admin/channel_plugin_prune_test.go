package admin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestChannelPluginArchivePruneChecksProgramContractAndReferences(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			mux := catalogMux(db)
			source := strings.Replace(recoveryModule, `apiVersion:2,`, `apiVersion:2,taskCapture:"durable",`, 1)
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
				err = store.ArchiveChannelPluginVersion(t.Context(), tx, store.ChannelPluginVersion{Key: "recovery", Fingerprint: program.Fingerprint(), Source: source, Manifest: string(metadata)})
			}
			if err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			input := map[string]any{"key": "recovery", "fingerprint": program.Fingerprint()}
			out := catalogRequest(mux, "GET", "/api/channel-plugin-versions", nil)
			if out.Code != 200 || !strings.Contains(out.Body.String(), `"prunable":true`) || strings.Contains(out.Body.String(), "export const") || strings.Contains(out.Body.String(), `"manifest"`) {
				t.Fatalf("invalid safe metadata: %d %s", out.Code, out.Body.String())
			}
			identity, _ := json.Marshal(map[string]any{"version": 3, "driverKey": "recovery", "driverFingerprint": program.Fingerprint()})
			if _, err = db.Exec(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES('pending-prune','','','',?)`, string(identity)); err != nil {
				t.Fatal(err)
			}
			catalogCall(t, mux, "POST", "/api/channel-plugin-versions/prune", input, 409)
			if _, err = db.Exec(`DELETE FROM proxy_video_tasks WHERE public_id='pending-prune'`); err != nil {
				t.Fatal(err)
			}
			catalogCall(t, mux, "POST", "/api/channel-plugin-versions/prune", input, 200)
			catalogCall(t, mux, "POST", "/api/channel-plugin-versions/prune", input, 200)
			input["source"] = "private-source"
			catalogCall(t, mux, "POST", "/api/channel-plugin-versions/prune", input, 400)
		})
	}
}
