package channelplugins_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderAuthenticationPersistenceSnapshotAndDisable(t *testing.T) {
	const source = `export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response"],authentication:{type:"api_key",header:"X-First-Key"}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}`
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			if _, err := channelplugins.Save(t.Context(), db.DB, "auth", source, true, 0); err != nil {
				t.Fatal(err)
			}
			binding := store.DirectProviderBinding{Key: "auth", Protocols: []string{"chat"}}
			manager := channelplugins.New(db.DB)
			old, err := manager.ResolveDriver(t.Context(), binding, "chat", store.DirectCredentialAPIKey)
			if err != nil {
				t.Fatal(err)
			}
			upgraded := strings.Replace(source, "X-First-Key", "X-Second-Key", 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "auth", upgraded, true, 1); err != nil {
				t.Fatal(err)
			}
			current, err := channelplugins.New(db.DB).ResolveDriver(t.Context(), binding, "chat", store.DirectCredentialAPIKey)
			if err != nil || current.Fingerprint() == old.Fingerprint() {
				t.Fatal("persisted authentication upgrade was not captured")
			}
			for _, item := range []struct {
				call   channelplugins.DriverInvocation
				header string
			}{{old, "X-First-Key"}, {current, "X-Second-Key"}} {
				headers := http.Header{}
				if err := item.call.ApplyAuthentication(headers, "fixture-current-key"); err != nil || headers.Get(item.header) != "fixture-current-key" || len(headers) != 1 {
					t.Fatal("authentication snapshot changed with installed program")
				}
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "auth", upgraded, false, 2); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.ResolveDriver(t.Context(), binding, "chat", store.DirectCredentialAPIKey); !errors.Is(err, channelplugins.ErrUnavailable) {
				t.Fatal("disabled authentication remained resolvable")
			}
		})
	}
}
