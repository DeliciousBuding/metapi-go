package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newAPIPaginationToken(id int) map[string]interface{} {
	return map[string]interface{}{
		"id":     id,
		"name":   fmt.Sprintf("token-%d", id),
		"key":    fmt.Sprintf("sk-token-%d", id),
		"status": 1,
	}
}

func newAPIPaginationTokens(start, count int) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, count)
	for id := start; id < start+count; id++ {
		items = append(items, newAPIPaginationToken(id))
	}
	return items
}

func writeNewAPITokenPage(w http.ResponseWriter, total int, items []map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"items":     items,
			"total":     total,
			"page_size": UpstreamTokenListPageLimit,
		},
	})
}

func TestNewApiAdapter_GetAPITokens_PagesThrough101Boundary(t *testing.T) {
	var pages []int
	var pageSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/token/" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("p"))
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		pages = append(pages, page)
		pageSizes = append(pageSizes, size)
		switch page {
		case 1:
			writeNewAPITokenPage(w, UpstreamTokenListPageLimit+1, newAPIPaginationTokens(1, UpstreamTokenListPageLimit))
		case 2:
			writeNewAPITokenPage(w, UpstreamTokenListPageLimit+1, newAPIPaginationTokens(UpstreamTokenListPageLimit+1, 1))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
	if err != nil {
		t.Fatalf("GetAPITokens: %v", err)
	}
	if len(tokens) != UpstreamTokenListPageLimit+1 {
		t.Fatalf("tokens = %d, want %d", len(tokens), UpstreamTokenListPageLimit+1)
	}
	if tokens[0].Key != "sk-token-1" || tokens[len(tokens)-1].Key != "sk-token-101" {
		t.Fatalf("unexpected token boundaries: first=%q last=%q", tokens[0].Key, tokens[len(tokens)-1].Key)
	}
	if !reflect.DeepEqual(pages, []int{1, 2}) {
		t.Fatalf("pages = %v, want [1 2]", pages)
	}
	for _, size := range pageSizes {
		if size != UpstreamTokenListPageLimit {
			t.Fatalf("page_size = %d, want %d", size, UpstreamTokenListPageLimit)
		}
	}
}

func TestNewApiAdapter_GetAPITokens_HydratesMaskedKeysInBatchesOf100(t *testing.T) {
	var batchSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
			page, _ := strconv.Atoi(r.URL.Query().Get("p"))
			items := newAPIPaginationTokens(1, UpstreamTokenListPageLimit)
			if page == 2 {
				items = newAPIPaginationTokens(UpstreamTokenListPageLimit+1, 1)
			}
			for _, item := range items {
				item["key"] = fmt.Sprintf("masked-%d****", item["id"])
			}
			writeNewAPITokenPage(w, UpstreamTokenListPageLimit+1, items)
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/batch/keys":
			var body struct {
				IDs []int `json:"ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode batch body: %v", err)
			}
			batchSizes = append(batchSizes, len(body.IDs))
			keys := make(map[string]string, len(body.IDs))
			for _, id := range body.IDs {
				keys[strconv.Itoa(id)] = fmt.Sprintf("full-%d", id)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{"keys": keys}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
	if err != nil {
		t.Fatalf("GetAPITokens: %v", err)
	}
	if len(tokens) != UpstreamTokenListPageLimit+1 {
		t.Fatalf("tokens = %d, want %d", len(tokens), UpstreamTokenListPageLimit+1)
	}
	if !reflect.DeepEqual(batchSizes, []int{UpstreamTokenListPageLimit, 1}) {
		t.Fatalf("batch sizes = %v, want [100 1]", batchSizes)
	}
	if tokens[0].Key != "full-1" || tokens[len(tokens)-1].Key != "full-101" {
		t.Fatalf("unexpected hydrated boundaries: first=%q last=%q", tokens[0].Key, tokens[len(tokens)-1].Key)
	}
}

func TestNewApiAdapter_GetAPITokens_HydratesDistinctIDsWithSameMaskedDisplay(t *testing.T) {
	for _, tc := range []struct {
		name      string
		total     int
		pageItems map[int][]map[string]interface{}
		wantIDs   []int
		wantKeys  []string
	}{
		{
			name:  "same_page",
			total: 2,
			pageItems: map[int][]map[string]interface{}{
				1: {
					{"id": 1, "key": "same****mask", "status": 1},
					{"id": 2, "key": "same****mask", "status": 1},
				},
			},
			wantIDs:  []int{1, 2},
			wantKeys: []string{"full-1", "full-2"},
		},
		{
			name:  "across_pages",
			total: 101,
			pageItems: map[int][]map[string]interface{}{
				1: append([]map[string]interface{}{{"id": 1, "key": "same****mask", "status": 1}}, newAPIPaginationTokens(2, 99)...),
				2: {{"id": 101, "key": "same****mask", "status": 1}},
			},
			wantIDs:  []int{1, 101},
			wantKeys: []string{"full-1", "full-101"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var batchIDs []int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
					page, _ := strconv.Atoi(r.URL.Query().Get("p"))
					items, ok := tc.pageItems[page]
					if !ok {
						http.NotFound(w, r)
						return
					}
					writeNewAPITokenPage(w, tc.total, items)
				case r.Method == http.MethodPost && r.URL.Path == "/api/token/batch/keys":
					var body struct {
						IDs []int `json:"ids"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode batch body: %v", err)
					}
					batchIDs = append(batchIDs, body.IDs...)
					keys := make(map[string]string, len(body.IDs))
					for _, id := range body.IDs {
						keys[strconv.Itoa(id)] = fmt.Sprintf("full-%d", id)
					}
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"success": true,
						"data":    map[string]interface{}{"keys": keys},
					})
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
			if err != nil {
				t.Fatalf("GetAPITokens: %v", err)
			}
			if len(tokens) != tc.total {
				t.Fatalf("tokens = %d, want %d", len(tokens), tc.total)
			}
			if tokens[0].Key != tc.wantKeys[0] || tokens[len(tokens)-1].Key != tc.wantKeys[1] {
				t.Fatalf("tokens = %+v, want hydrated keys %v", tokens, tc.wantKeys)
			}
			if !reflect.DeepEqual(batchIDs, tc.wantIDs) {
				t.Fatalf("batch IDs = %v, want %v", batchIDs, tc.wantIDs)
			}
		})
	}
}

func TestNewApiAdapter_GetAPITokens_RetriesLegacyContractAfterFlatCurrentResponse(t *testing.T) {
	var currentCalls atomic.Int32
	var legacyCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/token/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Query().Get("p") == "1" && r.URL.Query().Get("page_size") == strconv.Itoa(UpstreamTokenListPageLimit):
			currentCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": []interface{}{
					map[string]interface{}{"id": 2, "key": "sk-test-wrong", "status": 1},
				},
			})
		case r.URL.Query().Get("p") == "0" && r.URL.Query().Get("size") == strconv.Itoa(UpstreamTokenListPageLimit):
			legacyCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": []interface{}{
					map[string]interface{}{"id": 1, "key": "sk-test-home", "status": 1},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, complete, err := newApiAdapterUnderTest().GetAPITokensComplete(ctx, srv.URL, "dashboard-pat", nil, nil)
	if !errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		t.Fatalf("GetAPITokensComplete error = %v, want completeness unproven", err)
	}
	if complete {
		t.Fatal("Complete = true for a legacy flat response without total")
	}
	if len(tokens) != 1 || tokens[0].Key != "sk-test-home" {
		t.Fatalf("tokens = %+v, want the legacy-contract homepage, not the current-query page", tokens)
	}
	if currentCalls.Load() != 1 || legacyCalls.Load() != 1 {
		t.Fatalf("current/legacy calls = %d/%d, want 1/1", currentCalls.Load(), legacyCalls.Load())
	}
}

func TestNewApiAdapter_GetAPITokens_RejectsRemoteTotalAboveResourceLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/token/" {
			http.NotFound(w, r)
			return
		}
		writeNewAPITokenPage(w, newAPITokenListMaxItems+1, newAPIPaginationTokens(1, UpstreamTokenListPageLimit))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
	if err == nil {
		t.Fatalf("GetAPITokens error = nil, tokens=%d; want remote total over limit rejected", len(tokens))
	}
	if len(tokens) != 0 {
		t.Fatalf("partial tokens returned on resource limit: %+v", tokens)
	}
	if calls.Load() != 1 {
		t.Fatalf("listing calls = %d, want 1 (reject from first total)", calls.Load())
	}
}

func TestNewApiAdapter_GetAPITokens_AllowsExactlyResourceLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/token/" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("p"))
		writeNewAPITokenPage(
			w,
			newAPITokenListMaxItems,
			newAPIPaginationTokens((page-1)*UpstreamTokenListPageLimit+1, UpstreamTokenListPageLimit),
		)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
	if err != nil {
		t.Fatalf("GetAPITokens: %v", err)
	}
	if len(tokens) != newAPITokenListMaxItems {
		t.Fatalf("tokens = %d, want %d", len(tokens), newAPITokenListMaxItems)
	}
	if calls.Load() != newAPITokenListMaxPages {
		t.Fatalf("listing calls = %d, want %d", calls.Load(), newAPITokenListMaxPages)
	}
}

func TestNewApiAdapter_GetAPITokens_RejectsAggregateDecodedBytesAboveResourceLimit(t *testing.T) {
	const pageCount = 3
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/token/" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("p"))
		items := newAPIPaginationTokens((page-1)*UpstreamTokenListPageLimit+1, UpstreamTokenListPageLimit)
		for _, item := range items {
			item["padding"] = strings.Repeat("x", 28*1024)
		}
		writeNewAPITokenPage(w, pageCount*UpstreamTokenListPageLimit, items)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
	if err == nil {
		t.Fatalf("GetAPITokens error = nil, tokens=%d; want aggregate decoded bytes over limit rejected", len(tokens))
	}
	if len(tokens) != 0 {
		t.Fatalf("partial tokens returned on decoded-byte limit: %+v", tokens)
	}
	if calls.Load() != pageCount {
		t.Fatalf("listing calls = %d, want %d before rejecting aggregate bytes", calls.Load(), pageCount)
	}
}

func TestNewApiAdapter_GetAPITokens_RejectsIncompletePagination(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "duplicate_page",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeNewAPITokenPage(w, UpstreamTokenListPageLimit*2, newAPIPaginationTokens(1, UpstreamTokenListPageLimit))
			},
		},
		{
			name: "total_changed",
			handler: func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("p"))
				if page == 1 {
					writeNewAPITokenPage(w, UpstreamTokenListPageLimit*2, newAPIPaginationTokens(1, UpstreamTokenListPageLimit))
					return
				}
				writeNewAPITokenPage(w, UpstreamTokenListPageLimit*2+1, newAPIPaginationTokens(UpstreamTokenListPageLimit+1, UpstreamTokenListPageLimit))
			},
		},
		{
			name: "short_page_before_total",
			handler: func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("p"))
				if page == 1 {
					writeNewAPITokenPage(w, UpstreamTokenListPageLimit*2, newAPIPaginationTokens(1, UpstreamTokenListPageLimit))
					return
				}
				writeNewAPITokenPage(w, UpstreamTokenListPageLimit*2, nil)
			},
		},
		{
			name: "missing_total",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{"items": newAPIPaginationTokens(1, 1)}})
			},
		},
		{
			name: "missing_key",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": 1}}, "total": 1}})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tokens, err := newApiAdapterUnderTest().GetAPITokens(ctx, srv.URL, "dashboard-pat", nil, nil)
			if err == nil {
				t.Fatalf("GetAPITokens error = nil, tokens=%d; want incomplete listing rejected", len(tokens))
			}
			if len(tokens) != 0 {
				t.Fatalf("partial tokens returned on failure: %+v", tokens)
			}
		})
	}
}

func TestNewApiAdapter_DeleteAPIToken_FindsTargetOnSecondPage(t *testing.T) {
	var deleted atomic.Int32
	var pages []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
			page, _ := strconv.Atoi(r.URL.Query().Get("p"))
			pages = append(pages, page)
			if page == 1 {
				writeNewAPITokenPage(w, UpstreamTokenListPageLimit+1, newAPIPaginationTokens(1, UpstreamTokenListPageLimit))
				return
			}
			target := newAPIPaginationToken(UpstreamTokenListPageLimit + 1)
			target["key"] = "target-key"
			writeNewAPITokenPage(w, UpstreamTokenListPageLimit+1, []map[string]interface{}{target})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/token/101":
			deleted.Add(1)
			fmt.Fprint(w, `{"success":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	uid := 1
	if err := newApiAdapterUnderTest().DeleteAPIToken(ctx, srv.URL, "dashboard-pat", "target-key", &uid, nil); err != nil {
		t.Fatalf("DeleteAPIToken: %v", err)
	}
	if !reflect.DeepEqual(pages, []int{1, 2}) {
		t.Fatalf("pages = %v, want [1 2]", pages)
	}
	if deleted.Load() != 1 {
		t.Fatalf("DELETE count = %d, want 1", deleted.Load())
	}
}

func TestNewApiAdapter_DeleteAPIToken_UnprovenLegacyMissFailsClosed(t *testing.T) {
	var currentCalls atomic.Int32
	var legacyCalls atomic.Int32
	var deleteCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("New-Api-User") != "1" {
			t.Error("legacy delete listing must keep the owner identity")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/" && r.URL.Query().Get("p") == "1":
			currentCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": []interface{}{
					map[string]interface{}{"id": 1, "key": "other-current-page", "status": 1},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/" && r.URL.Query().Get("p") == "0":
			legacyCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": []interface{}{
					map[string]interface{}{"id": 2, "key": "other-legacy-page", "status": 1},
				},
			})
		case r.Method == http.MethodDelete:
			deleteCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	uid := 1
	err := newApiAdapterUnderTest().DeleteAPIToken(ctx, srv.URL, "dashboard-pat", "sk-missing-target", &uid, nil)
	if !errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		t.Fatalf("DeleteAPIToken error = %v, want completeness unproven", err)
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("DELETE count = %d, want 0 for an unproven legacy miss", deleteCalls.Load())
	}
	if currentCalls.Load() != 1 || legacyCalls.Load() != 1 {
		t.Fatalf("current/legacy listing calls = %d/%d, want 1/1", currentCalls.Load(), legacyCalls.Load())
	}
}

func TestNewApiAdapter_DeleteAPIToken_DoesNotReplayFailedDelete(t *testing.T) {
	var deleted atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
			writeNewAPITokenPage(w, 1, []map[string]interface{}{newAPIPaginationToken(42)})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/token/42":
			deleted.Add(1)
			http.Error(w, `{"success":false,"message":"delete failed"}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	uid := 1
	err := newApiAdapterUnderTest().DeleteAPIToken(ctx, srv.URL, "dashboard-pat", "sk-token-42", &uid, nil)
	if err == nil {
		t.Fatal("DeleteAPIToken reported success after DELETE failed")
	}
	if deleted.Load() != 1 {
		t.Fatalf("DELETE count = %d, want exactly 1 (no cookie/identity replay)", deleted.Load())
	}
}
