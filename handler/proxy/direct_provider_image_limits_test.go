package proxyhandler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderImageCumulativeJSONCeilingPreservesEarlierUsage(t *testing.T) {
	const responseBytes = 1 << 20
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := calls.Add(1)
		if count > 17 {
			t.Error("cumulative byte ceiling did not stop polling")
			w.WriteHeader(500)
			return
		}
		prefix := fmt.Sprintf(`{"task_id":"one","meter":{"input":7,"output":%d,"total":%d},"pad":"`, count-1, count+6)
		body := prefix + strings.Repeat("a", responseBytes-len(prefix)-2) + `"}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()
	// The fixed 16 MiB ceiling is exercised, not replaced by a test knob.
	if maxImageTaskResponseBytes != 16<<20 {
		t.Fatal("the documented cumulative ceiling changed")
	}
	index := strings.Index(imageOperationModule, "export function taskResult")
	source := imageOperationModule[:index] + `export function taskResult(){return {status:"pending"};}`
	op := imageOperationFixture(t, server.URL, source)
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/images/generations", strings.NewReader(`{}`))
	_, err := op.run(request)
	// The 17th one-MiB response exceeds the ceiling before observations/JS;
	// only the earlier 16th response's authoritative counters are retained.
	if err == nil || calls.Load() != 17 || directMediaFailureUsage(err).TotalTokens != 22 {
		t.Fatalf("cumulative ceiling or prior usage lost: calls=%d usage=%+v err=%v", calls.Load(), directMediaFailureUsage(err), err)
	}
}

func TestProviderImageRealHTTPCancellationStopsVendorAndSettlesOnce(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			entered, canceled := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method == http.MethodGet {
					close(entered)
					<-r.Context().Done()
					close(canceled)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"task_id":"one","task_status":"PENDING","meter":{"input":7,"output":0,"total":7}}`)
			}))
			defer vendor.Close()
			endpoints := store.DirectEndpoints{ImageGeneration: &store.DirectEndpoint{URL: vendor.URL + "/images/generations", Auth: store.DirectAuthBearer}}
			db := installDirectMediaRuntime(t, dialect, endpoints, store.DirectProtocolImageGeneration, "")
			if _, err := channelplugins.Save(t.Context(), db.DB, "image-operation", imageOperationModule, true, 0); err != nil {
				t.Fatal(err)
			}
			settings := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "image-operation", Protocols: []string{"imageGeneration"}, ImageQueries: map[string]store.DirectImageQueryEndpoint{"imageGeneration": {URL: vendor.URL + "/tasks/{taskId}", Auth: store.DirectAuthBearer}}}}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), settings); err != nil {
				t.Fatal(err)
			}
			cfg := getUpstreamConfig()
			cfg.ChannelPlugins = channelplugins.New(db.DB)
			entries := make(chan proxy.ProxyLogEntry, 2)
			cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { entries <- entry; return nil }
			downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy})
				HandleImagesGenerations(w, r.WithContext(ctx))
			}))
			defer downstream.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request, _ := http.NewRequestWithContext(ctx, http.MethodPost, downstream.URL+"/v1/images/generations", strings.NewReader(`{"model":"client-alias","prompt":"fixture image"}`))
			request.Header.Set("Content-Type", "application/json")
			result := make(chan error, 1)
			go func() {
				response, err := downstream.Client().Do(request)
				if response != nil {
					_ = response.Body.Close()
				}
				result <- err
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("real HTTP image query did not start")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("downstream lost HTTP cancellation: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("downstream HTTP request outlived cancellation")
			}
			select {
			case <-canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("vendor query outlived canceled downstream HTTP")
			}
			select {
			case entry := <-entries:
				if calls.Load() != 2 || entry.TotalTokens == nil || *entry.TotalTokens != 7 || entry.Status != "failed" {
					t.Fatalf("cancel replayed or lost original usage: calls=%d entry=%+v", calls.Load(), entry)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("canceled HTTP attempt did not settle")
			}
			select {
			case <-entries:
				t.Fatal("cancellation settled twice")
			default:
			}
		})
	}
}
