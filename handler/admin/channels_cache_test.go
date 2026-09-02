package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// This file pins the snapshot-cache contract of GET /api/channels: the cache
// must hold several three-dimensional keys (unbounded/paged × pageSize ×
// status filter) at once, stay bounded, evict deterministically, keep the
// ?refresh=true bypass and the mutation invalidation hook, and still run the
// fleet-wide JOIN once per key under concurrency. Every assertion below is
// driven through the real handler (router + response headers), never through
// the cache struct's internals, so the expectations come from the documented
// semantics rather than from the implementation.

// seedChannelsCacheFleet inserts n route_channels rows on one shared
// route/account/token triple and returns those ids (the mutation test needs
// them to POST a new channel).
func seedChannelsCacheFleet(t *testing.T, db *store.DB, n int) (routeID, accountID, tokenID int64) {
	t.Helper()
	routeID, accountID, tokenID = seedRouteChannelRefs(t, db)
	for i := 0; i < n; i++ {
		if _, err := db.Exec(
			`INSERT INTO route_channels
				(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
			 VALUES (?, ?, ?, ?, 0, 10, TRUE, FALSE)`,
			routeID, accountID, tokenID, "gpt-cache-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("insert channel %d: %v", i, err)
		}
	}
	return routeID, accountID, tokenID
}

// requireCacheState drives GET path and asserts the x-channels-snapshot-cache
// response header is want ("hit"/"miss"), returning the recorder so callers can
// also inspect the payload.
func requireCacheState(t *testing.T, r chi.Router, path, want string) *httptest.ResponseRecorder {
	t.Helper()
	rec := doGet(t, r, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("x-channels-snapshot-cache"); got != want {
		t.Fatalf("GET %s: x-channels-snapshot-cache=%q, want %q", path, got, want)
	}
	return rec
}

func channelsCacheEnvelope(t *testing.T, rec *httptest.ResponseRecorder) (items []any, total int, page int, pageSize int) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, rec.Body.String())
	}
	items, _ = resp["items"].([]any)
	toInt := func(key string) int {
		v, _ := resp[key].(float64)
		return int(v)
	}
	return items, toInt("total"), toInt("page"), toInt("pageSize")
}

// TestChannels_SnapshotCache_PagedKeysDoNotEvictEachOther is the endpoint-level
// regression for the single-slot cache: alternating between two pages must hit
// from the second round on. With one slot the second page evicts the first, so
// every request misses and the fleet-wide 5-way JOIN runs every time — the 10s
// TTL absorbs none of the polling it exists for.
func TestChannels_SnapshotCache_PagedKeysDoNotEvictEachOther(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedChannelsCacheFleet(t, db, 120)

	const (
		page1 = "/api/channels?page=1&pageSize=50"
		page2 = "/api/channels?page=2&pageSize=50"
	)

	// Round 1: both keys are cold → miss (and populate).
	firstP1 := requireCacheState(t, r, page1, "miss")
	firstP2 := requireCacheState(t, r, page2, "miss")

	// Round 2: both keys are warm and inside the 10s TTL → hit.
	secondP1 := requireCacheState(t, r, page1, "hit")
	secondP2 := requireCacheState(t, r, page2, "hit")

	// A hit must return the very same payload for that key (no key confusion,
	// no cross-page shadowing).
	if secondP1.Body.String() != firstP1.Body.String() {
		t.Fatalf("page 1 hit body differs from its miss body")
	}
	if secondP2.Body.String() != firstP2.Body.String() {
		t.Fatalf("page 2 hit body differs from its miss body")
	}
	if firstP1.Body.String() == firstP2.Body.String() {
		t.Fatalf("page 1 and page 2 returned identical payloads; cache keys must differ")
	}
	items1, total1, pageEcho1, size1 := channelsCacheEnvelope(t, firstP1)
	items2, total2, pageEcho2, size2 := channelsCacheEnvelope(t, secondP2)
	if total1 != 120 || total2 != 120 {
		t.Fatalf("total = %d/%d, want 120/120", total1, total2)
	}
	if pageEcho1 != 1 || pageEcho2 != 2 {
		t.Fatalf("page echo = %d/%d, want 1/2", pageEcho1, pageEcho2)
	}
	if size1 != 50 || size2 != 50 {
		t.Fatalf("pageSize = %d/%d, want 50/50", size1, size2)
	}
	if len(items1) != 50 || len(items2) != 50 {
		t.Fatalf("items = %d/%d, want 50/50", len(items1), len(items2))
	}
}

// TestChannels_SnapshotCache_UnboundedAndPagedViewsCoexist asserts the two
// realistic traffic shapes — the dashboard's unbounded poll and an operator's
// paged/filtered browsing — do not evict each other, including the status
// dimension of the key.
func TestChannels_SnapshotCache_UnboundedAndPagedViewsCoexist(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedChannelsCacheFleet(t, db, 120)

	const (
		unbounded  = "/api/channels"
		filtered   = "/api/channels?status=enabled"
		paged      = "/api/channels?page=2&pageSize=50"
		pagedSmall = "/api/channels?page=1&pageSize=20"
	)

	unboundedMiss := requireCacheState(t, r, unbounded, "miss")
	requireCacheState(t, r, paged, "miss")
	requireCacheState(t, r, filtered, "miss")
	requireCacheState(t, r, pagedSmall, "miss")

	// Every one of the four keys must still be resident: the dashboard poll
	// survives operator browsing, and a status filter is its own key.
	unboundedHit := requireCacheState(t, r, unbounded, "hit")
	requireCacheState(t, r, paged, "hit")
	requireCacheState(t, r, filtered, "hit")
	requireCacheState(t, r, pagedSmall, "hit")

	if unboundedHit.Body.String() != unboundedMiss.Body.String() {
		t.Fatalf("unbounded hit body differs from its miss body")
	}
	items, total, pageEcho, pageSize := channelsCacheEnvelope(t, unboundedHit)
	if total != 120 || len(items) != 120 {
		t.Fatalf("unbounded total=%d items=%d, want 120/120", total, len(items))
	}
	if pageEcho != 1 || pageSize != 120 {
		t.Fatalf("unbounded envelope page=%d pageSize=%d, want 1/120 (single full page)", pageEcho, pageSize)
	}
	// The status-filtered unbounded view is a different key with a different
	// (filtered) payload, so it must not have served the plain unbounded bytes.
	filteredRec := requireCacheState(t, r, filtered, "hit")
	if filteredRec.Body.String() == unboundedHit.Body.String() {
		t.Fatalf("status-filtered view returned the unfiltered payload; keys must stay distinct")
	}
}

// TestChannels_SnapshotCache_IsBoundedAndEvictsOldestFirst covers the capacity
// contract: inserting more distinct keys than the cache may hold must not grow
// it without bound, the entries that go must be the oldest inserted (FIFO, not
// map-iteration luck), and the survivors must still hit.
func TestChannels_SnapshotCache_IsBoundedAndEvictsOldestFirst(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedChannelsCacheFleet(t, db, 60)

	// 16 is the documented bound: raising it is a memory/review decision, not a
	// free parameter, so the test pins it instead of silently following it.
	if channelsSnapshotMaxEntries != 16 {
		t.Fatalf("channelsSnapshotMaxEntries = %d, want 16 (documented capacity bound)", channelsSnapshotMaxEntries)
	}

	const distinctKeys = 20 // > channelsSnapshotMaxEntries
	pathFor := func(page int) string {
		return "/api/channels?page=" + strconv.Itoa(page) + "&pageSize=1"
	}

	// Insert 20 distinct keys in a known order; each is cold, so each is a miss
	// and each stamps the next insertion slot.
	for page := 1; page <= distinctKeys; page++ {
		requireCacheState(t, r, pathFor(page), "miss")
	}

	// Survivors first. Hits do not insert, so this loop cannot perturb the
	// eviction order the next loop depends on. Inserts 17..20 evicted the four
	// oldest entries (pages 1..4), leaving pages 5..20 resident.
	for page := distinctKeys - channelsSnapshotMaxEntries + 1; page <= distinctKeys; page++ {
		requireCacheState(t, r, pathFor(page), "hit")
	}

	// The evicted keys must miss again: that is the observable proof the map
	// has an upper bound of channelsSnapshotMaxEntries live entries, and that
	// the entries which left were the earliest inserted (deterministic FIFO).
	for page := 1; page <= distinctKeys-channelsSnapshotMaxEntries; page++ {
		requireCacheState(t, r, pathFor(page), "miss")
	}
}

// TestChannels_SnapshotCache_RefreshBypassesThenRepopulates pins the
// ?refresh=true bypass: the forced request recomputes (miss) and leaves the key
// warm again (hit) for everyone else.
func TestChannels_SnapshotCache_RefreshBypassesThenRepopulates(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedChannelsCacheFleet(t, db, 60)

	const paged = "/api/channels?page=1&pageSize=10"

	cold := requireCacheState(t, r, paged, "miss")
	requireCacheState(t, r, paged, "hit")

	forced := requireCacheState(t, r, paged+"&refresh=true", "miss")
	if forced.Body.String() != cold.Body.String() {
		t.Fatalf("forced refresh recomputed a different payload than the cold miss")
	}

	// The bypass must repopulate, not merely invalidate: the next plain request
	// is served from cache again.
	requireCacheState(t, r, paged, "hit")
}

// TestChannels_SnapshotCache_MutationInvalidatesEveryKey pins the invalidation
// hook: a route_channels mutation clears the whole cache (it is the "the data
// changed" entry point, not an eviction path), so every key — unbounded,
// filtered and paged — misses afterwards and recomputes with the new row.
func TestChannels_SnapshotCache_MutationInvalidatesEveryKey(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedChannelsCacheFleet(t, db, 60)

	const (
		unbounded = "/api/channels"
		filtered  = "/api/channels?status=enabled"
		paged     = "/api/channels?page=1&pageSize=10"
	)
	requireCacheState(t, r, unbounded, "miss")
	requireCacheState(t, r, filtered, "miss")
	requireCacheState(t, r, paged, "miss")
	// All three are warm now.
	requireCacheState(t, r, unbounded, "hit")
	requireCacheState(t, r, filtered, "hit")
	requireCacheState(t, r, paged, "hit")

	add := doPostJSON(t, r, "/api/routes/"+itoa(routeID)+"/channels", map[string]any{
		"accountId":   accountID,
		"tokenId":     tokenID,
		"sourceModel": "gpt-cache-added",
		"priority":    1,
		"weight":      10,
	})
	if add.Code != http.StatusOK {
		t.Fatalf("add channel: %d %s", add.Code, add.Body.String())
	}

	// Every key misses again — invalidation is whole-cache, not per-page.
	afterUnbounded := requireCacheState(t, r, unbounded, "miss")
	requireCacheState(t, r, filtered, "miss")
	requireCacheState(t, r, paged, "miss")

	// ... and the recomputed snapshot actually contains the new row.
	_, total, _, _ := channelsCacheEnvelope(t, afterUnbounded)
	if total != 61 {
		t.Fatalf("post-mutation total = %d, want 61 (cache must not serve the stale snapshot)", total)
	}
}

// TestChannels_SnapshotCache_ConcurrentMissesRunJoinOnce asserts the
// singleflight half of the contract survives the multi-key map: N simultaneous
// requests for one cold key must produce exactly one fleet-wide JOIN, with
// identical bytes for every caller.
func TestChannels_SnapshotCache_ConcurrentMissesRunJoinOnce(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedChannelsCacheFleet(t, db, 60)
	globalChannelsCache.clear()

	// SQLite is opened with MaxOpenConns(1), so holding the only pooled
	// connection guarantees the leader's compute is still in flight when every
	// follower arrives: without that barrier the leader could finish before the
	// followers start, they would all read a warm cache and the test would pass
	// even if singleflight were removed. The saturated pool is also the
	// counter — database/sql bumps Stats().WaitCount for every query that has
	// to wait for a connection, so with the only connection held here, each
	// compute that reaches the DB adds exactly one wait. WaitCount delta is
	// therefore the number of JOIN attempts.
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("acquire pooled connection: %v", err)
	}
	connClosed := false
	defer func() {
		if !connClosed {
			_ = conn.Close()
		}
	}()

	waitsBefore := db.Stats().WaitCount

	const callers = 8
	var wg sync.WaitGroup
	recorders := make([]*httptest.ResponseRecorder, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/channels?page=1&pageSize=50", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			recorders[i] = rec
		}(i)
	}

	// All callers are blocked behind the held connection now; give the
	// goroutines time to actually pile up on the single-flight group, then
	// release the connection so the (single) compute can run.
	time.Sleep(250 * time.Millisecond)
	connClosed = true
	if err := conn.Close(); err != nil {
		t.Fatalf("release pooled connection: %v", err)
	}
	wg.Wait()

	if delta := db.Stats().WaitCount - waitsBefore; delta != 1 {
		t.Fatalf("%d concurrent misses on one cold key attempted %d DB queries, want exactly 1 (singleflight must dedupe the fleet-wide JOIN)", callers, delta)
	}

	var wantBody string
	for i, rec := range recorders {
		if rec == nil {
			t.Fatalf("caller %d produced no response", i)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("caller %d: status=%d body=%s", i, rec.Code, rec.Body.String())
		}
		if i == 0 {
			wantBody = rec.Body.String()
			continue
		}
		if rec.Body.String() != wantBody {
			t.Fatalf("caller %d got a different payload than caller 0", i)
		}
	}

	// The key is warm afterwards, and still bounded to a single entry's worth
	// of work: a plain follow-up request is a hit.
	requireCacheState(t, r, "/api/channels?page=1&pageSize=50", "hit")
}
