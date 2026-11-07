package upstream_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderAnonymousDiscoveryRequiresCurrentResolvedIdentity(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"current", "no-resolver", "resolver-rejected", "identity-replaced", "channel-policy-changed"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				db := connectDatabase(t, dialect)
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
						t.Error("anonymous discovery sent credentials")
					}
					fmt.Fprint(w, `{"items":["local-model"]}`)
				}))
				defer server.Close()
				const source = `export const meta={apiVersion:2,kind:"provider",key:"anonymous",name:"Anonymous",version:"1.0.0",protocols:["messages"],credentialKinds:["none"],hooks:["request","response","modelsRequest","modelsResponse"]};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}export function modelsRequest(){return {method:"GET"};}export function modelsResponse(i){return {models:JSON.parse(i.body).items};}`
				if _, err := channelplugins.Save(t.Context(), db.DB, "anonymous", source, true, 0); err != nil {
					t.Fatal(err)
				}
				input := connectInput(server.URL)
				input.Channel.Provider, input.CredentialKind, input.Secret = "ollama", store.DirectCredentialNone, ""
				input.Channel.Endpoints = store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: server.URL + "/v1/messages", Auth: store.DirectAuthNone, Profile: "ollama-messages"}}
				input.Channel.RequestConfig = store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "anonymous", Protocols: []string{"messages"}, Catalog: &store.DirectCatalogEndpoint{URL: server.URL + "/inventory", Auth: store.DirectAuthNone}}}
				out, err := upstream.Connect(t.Context(), db.DB, input)
				if err != nil || calls.Load() != 1 {
					t.Fatalf("initial anonymous connect failed: %v calls=%d", err, calls.Load())
				}
				var resolutions int
				var resolver upstream.DiscoveryCredentialResolver = func(ctx context.Context, id int64, proxy *string) (string, error) {
					resolutions++
					if mode == "resolver-rejected" {
						return "", errors.New("fixture resolver rejected")
					}
					if mode == "identity-replaced" {
						_, err := db.ExecContext(ctx, db.Rebind(`UPDATE upstream_credentials SET kind=?,secret=? WHERE id=?`), store.DirectCredentialAPIKey, "fixture-replacement", id)
						return "", err
					}
					if mode == "channel-policy-changed" {
						if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE upstream_channels SET provider=? WHERE id=?`), "openai", out.ID); err != nil {
							return "", err
						}
					}
					identity, err := oauth.ResolveDirectCredential(ctx, db.DB, id, proxy, false)
					if err != nil {
						return "", err
					}
					return identity.AccessToken, nil
				}
				if mode == "no-resolver" {
					resolver = nil
				}
				preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, out.ID, resolver)
				wantCalls := int32(1)
				if mode == "current" {
					wantCalls = 2
					if err != nil || preview.UpstreamCount != 1 {
						t.Fatalf("valid current anonymous identity failed: %+v %v", preview, err)
					}
				} else if err == nil {
					t.Fatal("unresolved or replaced anonymous identity accepted")
				}
				if calls.Load() != wantCalls || mode != "no-resolver" && resolutions != 1 {
					t.Fatalf("unexpected anonymous I/O: requests=%d resolutions=%d", calls.Load(), resolutions)
				}
			})
		}
	}
}
