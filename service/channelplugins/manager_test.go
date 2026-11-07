package channelplugins_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func database(t *testing.T, dialect string) *store.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "plugins.db")
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
	return db
}

const source = `export const meta={apiVersion:1,key:"fixture",name:"Fixture",version:"1.0.0",protocols:["chat"],hooks:["request"],options:[{key:"mode",label:"Mode",type:"select",default:"safe",choices:["safe","fast"]}]};export function request(input){return {body:[{op:"set",path:"vendor.mode",value:input.options.mode}]};}`

func TestBoundDriverSnapshotsAndCredentialIntersection(t *testing.T) {
	const driver = `export const meta={apiVersion:2,kind:"provider",key:"driver",name:"Driver",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response"],options:[{key:"mode",label:"Mode",type:"select",default:"safe",choices:["safe","fast"]}]};export function request(input){return {method:"POST",body:JSON.stringify({mode:input.options.mode,body:input.body})};}export function response(input){return {body:input.body};}`
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			if _, err := channelplugins.Save(t.Context(), db.DB, "driver", driver, true, 0); err != nil {
				t.Fatal(err)
			}
			binding := store.DirectProviderBinding{Key: "driver", Protocols: []string{"chat"}}
			config := store.DirectRequestConfig{Provider: &binding}
			endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://fixture.invalid/chat", Auth: store.DirectAuthBearer}}
			if _, err := upstream.CreateChannel(t.Context(), db.DB, upstream.ChannelCreate{Name: "Driver", Provider: "custom", Dialect: "generic", Enabled: true, BaseURL: "https://fixture.invalid", Endpoints: endpoints, RequestConfig: config}); err != nil {
				t.Fatalf("provider create failed: %v", err)
			}
			manager := channelplugins.New(db.DB)
			old, err := manager.ResolveDriver(t.Context(), binding, "chat", store.DirectCredentialAPIKey)
			if err != nil || old.Revision != 1 {
				t.Fatalf("driver snapshot: %+v %v", old, err)
			}
			for _, pair := range [][2]string{{"responses", store.DirectCredentialAPIKey}, {"chat", store.DirectCredentialOAuth}} {
				if _, err := manager.ResolveDriver(t.Context(), binding, pair[0], pair[1]); !errors.Is(err, channelplugins.ErrBinding) {
					t.Fatalf("driver borrowed capability %v: %v", pair, err)
				}
			}
			binding.Options = map[string]json.RawMessage{"mode": json.RawMessage(`"fast"`)}
			binding.Protocols[0] = "responses"
			upgraded := strings.ReplaceAll(driver, "safe", "safer")
			if _, err := channelplugins.Save(t.Context(), db.DB, "driver", upgraded, true, 1); err != nil {
				t.Fatalf("compatible driver upgrade failed: %v", err)
			}
			binding = store.DirectProviderBinding{Key: "driver", Protocols: []string{"chat"}}
			current, err := manager.ResolveDriver(t.Context(), binding, "chat", store.DirectCredentialAPIKey)
			if err != nil || current.Revision != 2 {
				t.Fatalf("current driver: %+v %v", current, err)
			}
			for _, item := range []struct {
				call channelplugins.DriverInvocation
				want string
			}{{old, "safe"}, {current, "safer"}} {
				raw, err := item.call.Call(t.Context(), "request", channelplugin.DriverInput{Protocol: "responses", CredentialKind: "oauth", Body: `{"seed":9007199254740993}`, Options: map[string]json.RawMessage{"mode": json.RawMessage(`"fast"`)}})
				if err != nil || !strings.Contains(string(raw), item.want) || !strings.Contains(string(raw), "9007199254740993") || strings.Contains(string(raw), "fast") {
					t.Fatalf("snapshot input/options changed: %s %v", raw, err)
				}
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "driver", strings.ReplaceAll(upgraded, `protocols:["chat"]`, `protocols:["responses"]`), true, 2); !errors.Is(err, channelplugins.ErrBinding) {
				t.Fatalf("driver dropped a bound protocol: %v", err)
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "driver", upgraded, false, 2); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.ResolveDriver(t.Context(), binding, "chat", store.DirectCredentialAPIKey); !errors.Is(err, channelplugins.ErrUnavailable) {
				t.Fatalf("disabled driver resolved: %v", err)
			}
			if _, err := old.Call(t.Context(), "request", channelplugin.DriverInput{Body: `{}`}); err != nil {
				t.Fatalf("captured operation hot-swapped on disable: %v", err)
			}
		})
	}
}

func TestLifecyclePersistenceCASAndAttemptSnapshots(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			row, err := channelplugins.Save(t.Context(), db.DB, "fixture", source, true, 0)
			if err != nil || row.Revision != 1 {
				t.Fatalf("install: %+v %v", row, err)
			}
			binding := []store.DirectPluginBinding{{Key: "fixture", Protocols: []string{"chat"}}}
			manager := channelplugins.New(db.DB)
			old, err := manager.Resolve(t.Context(), binding, "chat")
			if err != nil || len(old) != 1 {
				t.Fatalf("resolve: %v", err)
			}
			upgrade := strings.ReplaceAll(source, "safe", "safer")
			row, err = channelplugins.Save(t.Context(), db.DB, "fixture", upgrade, true, 1)
			if err != nil || row.Revision != 2 {
				t.Fatalf("upgrade: %+v %v", row, err)
			}
			if _, err = channelplugins.Save(t.Context(), db.DB, "fixture", source, true, 1); !errors.Is(err, store.ErrChannelPluginConflict) {
				t.Fatalf("stale edit: %v", err)
			}
			current, err := manager.Resolve(t.Context(), binding, "chat")
			if err != nil || current[0].Revision != 2 {
				t.Fatalf("new generation: %v", err)
			}
			for _, test := range []struct {
				inv  channelplugins.Invocation
				want string
			}{{old[0], "safe"}, {current[0], "safer"}} {
				r, err := test.inv.Program.Call(t.Context(), "request", channelplugin.Input{Protocol: "chat", Body: `{}`, Options: test.inv.Binding.Options})
				if err != nil || r.Body[0].Value != test.want {
					t.Fatalf("snapshot changed: %+v %v", r, err)
				}
			}
			row, err = channelplugins.Save(t.Context(), db.DB, "fixture", upgrade, false, 2)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = manager.Resolve(t.Context(), binding, "chat"); !errors.Is(err, channelplugins.ErrUnavailable) {
				t.Fatalf("disabled executes: %v", err)
			}
			if err = store.DeleteChannelPlugin(t.Context(), db.DB, "fixture", row.Revision); err != nil {
				t.Fatal(err)
			}
			if _, err = channelplugins.Save(t.Context(), db.DB, "fixture", source, true, 0); err != nil {
				t.Fatal(err)
			}
			// Delete/reinstall can reuse revision 1; the source digest prevents ABA.
			reinstalled, err := manager.Resolve(t.Context(), binding, "chat")
			if err != nil || reinstalled[0].Program.Manifest().Options[0].Choices[0] != "safe" {
				t.Fatalf("stale ABA cache: %v", err)
			}
			fresh, err := channelplugins.New(db.DB).Resolve(t.Context(), binding, "chat")
			if err != nil || fresh[0].Revision != 1 {
				t.Fatalf("restart: %v", err)
			}
		})
	}
}

func TestBindingValidationAndIncompatibleUpgrade(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			if _, err := channelplugins.Save(t.Context(), db.DB, "fixture", source, true, 0); err != nil {
				t.Fatal(err)
			}
			config := store.DirectRequestConfig{Plugins: []store.DirectPluginBinding{{Key: "fixture", Protocols: []string{"chat"}, Options: map[string]json.RawMessage{"mode": json.RawMessage(`"fast"`)}}}}
			endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://example.invalid/v1/chat/completions", Auth: store.DirectAuthBearer}}
			id, err := upstream.CreateChannel(t.Context(), db.DB, upstream.ChannelCreate{Name: "Fixture", Provider: "custom", Dialect: "generic", Enabled: true, BaseURL: "https://example.invalid", Endpoints: endpoints, RequestConfig: config})
			if err != nil || id <= 0 {
				t.Fatalf("create binding: %d %v", id, err)
			}
			if _, err = channelplugins.Save(t.Context(), db.DB, "fixture", strings.ReplaceAll(source, "fast", "faster"), true, 1); !errors.Is(err, channelplugins.ErrBinding) {
				t.Fatalf("incompatible upgrade accepted: %v", err)
			}
			provider := strings.Replace(source, "apiVersion:1", `apiVersion:2,kind:"provider",credentialKinds:["api_key"]`, 1)
			provider = strings.Replace(provider, `hooks:["request"]`, `hooks:["request","response"]`, 1) + `export function response(input){return {body:input.body};}`
			if _, err = channelplugins.Save(t.Context(), db.DB, "fixture", provider, true, 1); !errors.Is(err, channelplugins.ErrBinding) {
				t.Fatalf("provider upgrade retained old wire bindings: %v", err)
			}
			var restored store.DirectRequestConfig
			if err = db.Get(&restored, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil || len(restored.Plugins) != 1 {
				t.Fatalf("binding lost on persistence: %+v %v", restored, err)
			}
			config.Plugins[0].Options["unknown"] = json.RawMessage(`true`)
			if err = channelplugins.ValidateBindings(context.Background(), db.DB, config, endpoints); err == nil {
				t.Fatal("unknown option accepted")
			}
			items, err := channelplugins.List(t.Context(), db.DB)
			if err != nil || len(items) != 1 || items[0].Manifest == nil || items[0].LoadError != "" {
				t.Fatalf("management list: %+v %v", items, err)
			}
		})
	}
}

func TestInstalledProviderCannotBorrowWireBinding(t *testing.T) {
	const provider = `export const meta={apiVersion:2,kind:"provider",key:"fixture",name:"Fixture",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response"]};export function request(input){return {method:"POST",body:input.body};}export function response(input){return {body:input.body};}`
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			if _, err := channelplugins.Save(t.Context(), db.DB, "fixture", provider, true, 0); err != nil {
				t.Fatalf("provider installation failed: %v", err)
			}
			items, err := channelplugins.List(t.Context(), db.DB)
			if err != nil || len(items) != 1 || items[0].Manifest == nil || !items[0].Manifest.IsProvider() {
				t.Fatalf("provider identity lost in persistence: %+v %v", items, err)
			}
			bindings := []store.DirectPluginBinding{{Key: "fixture", Protocols: []string{"chat"}}}
			if _, err := channelplugins.New(db.DB).Resolve(t.Context(), bindings, "chat"); !errors.Is(err, channelplugins.ErrBinding) {
				t.Fatalf("installed provider resolved as wire hook: %v", err)
			}
		})
	}
}
