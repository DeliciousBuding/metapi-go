package upstream_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderHostAuthenticationCatalogPagesAndExplicitNone(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, noAuth := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/none=%v", dialect, noAuth), func(t *testing.T) {
				db := connectDatabase(t, dialect)
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					if r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Goog-Api-Key") != "" {
						t.Error("catalog sent raw credentials")
					}
					if noAuth {
						if r.Header.Get("Authorization") != "" {
							t.Error("explicit none sent provider credentials")
						}
					} else {
						parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
						if len(parts) != 3 {
							t.Error("missing catalog JWT")
							w.WriteHeader(401)
							return
						}
						mac := hmac.New(sha256.New, []byte("fixture-signing-key"))
						_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
						sig, err := base64.RawURLEncoding.DecodeString(parts[2])
						if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
							t.Error("catalog JWT signature does not match current key")
						}
						head, _ := base64.RawURLEncoding.DecodeString(parts[0])
						body, _ := base64.RawURLEncoding.DecodeString(parts[1])
						var header map[string]string
						var claims struct {
							Key    string `json:"api_key"`
							Expiry int64  `json:"exp"`
							Issued int64  `json:"timestamp"`
						}
						if json.Unmarshal(head, &header) != nil || json.Unmarshal(body, &claims) != nil || header["sign_type"] != "SIGN" || claims.Key != "fixture-id" || claims.Expiry <= time.Now().UnixMilli() || claims.Issued > time.Now().UnixMilli() {
							t.Error("catalog lost Zhipu clock/header contract")
						}
					}
					if n == 1 {
						fmt.Fprint(w, `{"items":["first-model"],"next":"second"}`)
					} else {
						if r.URL.Query().Get("cursor") != "second" {
							t.Error("catalog did not preserve its paging owner")
						}
						fmt.Fprint(w, `{"items":["second-model"]}`)
					}
				}))
				t.Cleanup(server.Close)
				source := strings.Replace(providerCatalogSource, `hooks:[`, `authentication:{type:"jwt_hs256",header:"Authorization",prefix:"Bearer ",jwt:{keySeparator:".",issuerClaim:"api_key",ttlSeconds:1800,timeUnit:"milliseconds",issuedAtClaim:"timestamp",signType:"SIGN"}},hooks:[`, 1)
				if _, err := channelplugins.Save(t.Context(), db.DB, "catalog", source, true, 0); err != nil {
					t.Fatal(err)
				}
				input := connectInput(server.URL)
				input.Secret = "fixture-id.fixture-signing-key"
				auth := store.DirectAuthBearer
				if noAuth {
					auth = store.DirectAuthNone
				}
				input.Channel.RequestConfig = store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "catalog", Protocols: []string{"chat"}, Catalog: &store.DirectCatalogEndpoint{URL: server.URL + "/inventory", Auth: auth, CursorParameter: "cursor"}}}
				out, err := upstream.Connect(t.Context(), db.DB, input)
				if err != nil || out.ModelCount != 2 || calls.Load() != 2 {
					t.Fatalf("authenticated catalog failed: models=%d calls=%d err=%v", out.ModelCount, calls.Load(), err)
				}
			})
		}
	}
}

func TestProviderHostAuthenticationCatalogRedirectDoesNotLeakOrPartiallyConnect(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := connectDatabase(t, dialect)
			var received, redirected atomic.Int32
			foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				fmt.Fprint(w, `{"items":["must-not-connect"]}`)
			}))
			t.Cleanup(foreign.Close)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirected.Add(1)
				if r.Header.Get("X-Vendor-Key") != "Key fixture-current-key" || r.Header.Get("Authorization") != "" {
					t.Error("catalog did not send only its host authentication")
				}
				http.Redirect(w, r, foreign.URL+"/foreign", http.StatusFound)
			}))
			t.Cleanup(server.Close)
			source := strings.Replace(providerCatalogSource, `hooks:[`, `authentication:{type:"api_key",header:"X-Vendor-Key",prefix:"Key "},hooks:[`, 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "catalog", source, true, 0); err != nil {
				t.Fatal(err)
			}
			input := connectInput(server.URL)
			input.Secret = "fixture-current-key"
			input.Channel.RequestConfig = store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "catalog", Protocols: []string{"chat"}, Catalog: &store.DirectCatalogEndpoint{URL: server.URL + "/inventory", Auth: store.DirectAuthBearer}}}
			before := connectCounts(t, db)
			_, err := upstream.Connect(t.Context(), db.DB, input)
			if err == nil || strings.Contains(err.Error(), "fixture-current-key") || received.Load() != 0 || redirected.Load() != 1 {
				t.Fatal("catalog followed a foreign origin or exposed authentication")
			}
			after := connectCounts(t, db)
			for index := range before {
				if before[index] != after[index] {
					t.Fatal("rejected authenticated catalog partially created a connection")
				}
			}
		})
	}
}
