package upstream_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestConnectPaginatedCatalogs(t *testing.T) {
	for _, format := range []string{"gemini", "anthropic"} {
		t.Run(format, func(t *testing.T) {
			db := connectDatabase(t, store.DialectSQLite)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := requests.Add(1)
				if page > 2 {
					t.Error("too many pages")
					w.WriteHeader(500)
					return
				}
				if format == "gemini" {
					if r.URL.Path != "/v1beta/models" || r.URL.Query().Get("pageSize") != "1000" || r.Header.Get("X-Goog-Api-Key") != "fixture-secret" {
						t.Error("Gemini pagination request")
					}
					if page == 1 {
						fmt.Fprint(w, `{"models":[{"name":"models/first"}],"nextPageToken":"next/+?&"}`)
					} else {
						if r.URL.Query().Get("pageToken") != "next/+?&" {
							t.Error("cursor was not encoded safely")
						}
						fmt.Fprint(w, `{"models":[{"name":"models/second"}]}`)
					}
				} else {
					if r.URL.Path != "/v1/models" || r.URL.Query().Get("limit") != "1000" || r.Header.Get("X-Api-Key") != "fixture-secret" {
						t.Error("Anthropic pagination request")
					}
					if page == 1 {
						fmt.Fprint(w, `{"data":[{"id":"first"}],"has_more":true,"last_id":"first"}`)
					} else {
						if r.URL.Query().Get("after_id") != "first" {
							t.Error("missing last ID cursor")
						}
						fmt.Fprint(w, `{"data":[{"id":"second"}],"has_more":false}`)
					}
				}
			}))
			defer server.Close()
			input := connectInput(server.URL)
			if format == "gemini" {
				input.Channel.Endpoints = store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: server.URL + "/v1beta/models", Auth: store.DirectAuthGoogle, ModelPath: true}}
			} else {
				input.Channel.Endpoints = store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: server.URL + "/v1/messages", Auth: store.DirectAuthAPIKey}}
			}
			out, err := upstream.Connect(t.Context(), db.DB, input)
			if err != nil || out.ModelCount != 2 || requests.Load() != 2 {
				t.Fatalf("paginated result=%+v requests=%d err=%v", out, requests.Load(), err)
			}
		})
	}
}

func TestConnectRejectsIncompletePagination(t *testing.T) {
	for _, mode := range []string{"repeated-cursor", "authentication", "second-page-missing", "aggregate-size"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := requests.Add(1)
				if page == 2 {
					if mode == "authentication" {
						w.WriteHeader(401)
						fmt.Fprint(w, "fixture-secret")
						return
					}
					if mode == "second-page-missing" {
						w.WriteHeader(404)
						return
					}
				}
				padding := ""
				if mode == "aggregate-size" {
					padding = strings.Repeat("x", 8<<20)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"name": "models/first"}}, "nextPageToken": "same", "padding": padding})
			}))
			defer server.Close()
			input := connectInput(server.URL)
			input.RecommendedModels = []string{"must-not-hide-pagination-failure"}
			input.Channel.Endpoints = store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: server.URL + "/v1beta/models", Auth: store.DirectAuthGoogle, ModelPath: true}}
			if _, err := upstream.Connect(t.Context(), nil, input); err == nil || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatalf("incomplete pagination accepted or unsafe error: %v", err)
			}
			if requests.Load() != 2 {
				t.Fatalf("expected exactly two pages, got %d", requests.Load())
			}
		})
	}
}

func TestConnectLargeAggregationCatalog(t *testing.T) {
	models := make([]map[string]string, 1000)
	for i := range models {
		models[i] = map[string]string{"id": fmt.Sprintf("model-%04d", i), "description": strings.Repeat("catalog metadata ", 200)}
	}
	body, err := json.Marshal(map[string]any{"data": models})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 2<<20 {
		t.Fatal("large fixture no longer exercises metadata size")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := connectDatabase(t, dialect)
			start := time.Now()
			out, err := upstream.Connect(t.Context(), db.DB, connectInput(server.URL))
			if err != nil || out.ModelCount != 1000 || out.RouteCount != 1000 {
				t.Fatalf("large catalog result=%+v err=%v", out, err)
			}
			if counts := connectCounts(t, db); !reflect.DeepEqual(counts, []int{1, 1, 1000, 1000, 1000, 1000, 1000, 1000}) {
				t.Fatalf("large catalog persisted incomplete graph: %v", counts)
			}
			t.Logf("1000 models, %d response bytes, connect elapsed %s", len(body), time.Since(start))
		})
	}
}
