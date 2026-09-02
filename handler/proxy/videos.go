package proxyhandler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// ProxyVideoTask holds a video task mapping (publicId -> upstreamVideoId).

// Create saves a process-local cache entry and dual-writes to
// `proxy_video_tasks` when store.GetDB() is available so multi-instance /
// restart can resolve publicId. GET/DELETE treat the store as an optional
// rewrite aid, not a hard gate — missing entries still pass the client id
// through to upstream. Sticky site/token pin remains a known limitation.
// See limitation.md.
type ProxyVideoTask struct {
	PublicID        string `json:"publicId"`
	UpstreamVideoID string `json:"upstreamVideoId"`
	SiteURL         string `json:"siteUrl"`
	TokenValue      string `json:"tokenValue"`
	RequestedModel  string `json:"requestedModel"`
	ActualModel     string `json:"actualModel"`
	ChannelID       int64  `json:"channelId"`
	AccountID       int64  `json:"accountId"`
}

// The process-local video task cache has two owners with two lifetimes:
//
//   - videoTaskStore (below) is a rewrite cache: publicId -> upstream video id
//     plus the channel/account pin used for sticky routing. It is not durable —
//     a restart starts empty and re-warms from the table on the first cold GET.
//   - proxy_video_tasks (DB) is the durable mapping shared across instances and
//     restarts. Its rows are pruned by
//     scheduler.NewProxyVideoTaskRetentionScheduler.
//
// Both age out on the same operator-visible dial
// (PROXY_VIDEO_TASK_RETENTION_DAYS). The cache never deletes DB rows — pruning
// durable state stays the scheduler's job, under its lease — and eviction here
// is lazy: it rides along on inserts, so there is no background goroutine and
// no per-request full scan.
//
// videoTaskEntry is one cache line. storedAt is when the mapping entered (or was
// refreshed in) *this process's* cache, not the durable row's created_at. Reads
// do not refresh it, so a mapping ages out on a fixed schedule whether or not
// clients keep polling it.
type videoTaskEntry struct {
	task     *ProxyVideoTask
	storedAt time.Time
}

const (
	// videoTaskSweepEveryInserts bounds how many inserts may pass without a
	// sweep (burst guard): the map can overshoot the capacity guardrail by at
	// most this many lines between sweeps.
	videoTaskSweepEveryInserts = 256
	// videoTaskSweepMinInterval bounds how long an entry may sit past its TTL
	// in a low-traffic process (idle guard): the first insert after the window
	// sweeps even when the insert counter is nowhere near its threshold.
	videoTaskSweepMinInterval = 5 * time.Minute
	// videoTaskCacheHeadroomDivisor sets the batch-trim target: over the cap,
	// the sweep evicts oldest-first down to cap-cap/16 rather than exactly cap,
	// so the O(n log n) trim runs once per ~cap/16 inserts instead of on every
	// insert at the cap.
	videoTaskCacheHeadroomDivisor = 16
)

var (
	videoTaskStore   = make(map[string]*videoTaskEntry)
	videoTaskStoreMu sync.RWMutex

	// videoTaskCacheMaxEntries is the hard capacity guardrail on cache lines
	// (~200 B each, so ~4 MB at 20k). It holds even when TTL eviction is
	// explicitly disabled, because bounding process memory is not an
	// operator-optional behaviour. Fixed in production; tests lower it.
	videoTaskCacheMaxEntries = 20000
	// videoTaskNow is the cache clock, injectable so eviction tests do not
	// sleep. Production never reassigns it.
	videoTaskNow = time.Now

	// Sweep bookkeeping; both guarded by videoTaskStoreMu.
	videoTaskInsertsSinceSweep int
	videoTaskLastSweepAt       time.Time
)

// HandleVideosCreate handles POST /v1/videos.
// Supports multipart/form-data or JSON body. Model is required.

// On successful non-stream upstream 2xx, dispatch rewrites response `id` to a
// generated publicId and SaveProxyVideoTask (process-local).
func HandleVideosCreate(w http.ResponseWriter, r *http.Request) {
	ctx, errResp := PrepareCtx(r, SurfConfig{
		Endpoint:       "videos",
		DownstreamPath: "/v1/videos",
		RequireModel:   true,
	})
	if errResp != nil {
		writeJSONError(w, errResp.Status, errResp.Error, errResp.ErrorType)
		return
	}

	if ctx.RequestedModel == "" {
		writeJSONError(w, 400, "model is required", "invalid_request_error")
		return
	}

	dispatchUpstream(w, r, ctx)
}

// HandleVideosGet handles GET /v1/videos/{id}.

// If a local mapping exists and UpstreamVideoID differs from the public path id,
// the upstream path uses UpstreamVideoID. When the mapping is missing, the
// client-provided id is passed through — no store-gated 404 theater.

// Sticky pin: when the mapping has ChannelID > 0, force preferred channel
// selection so the request hits the same upstream account/site as create.
func HandleVideosGet(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if publicID == "" {
		writeJSONError(w, 400, "missing video id", "invalid_request_error")
		return
	}

	task := GetProxyVideoTaskByPublicID(publicID)
	upstreamID := resolveVideoUpstreamIDFromTask(publicID, task)

	ctx, errResp := PrepareCtx(r, SurfConfig{
		Endpoint:       "videos",
		DownstreamPath: "/v1/videos/" + upstreamID,
		RequireModel:   false,
	})
	if errResp != nil {
		writeJSONError(w, errResp.Status, errResp.Error, errResp.ErrorType)
		return
	}
	applyVideoTaskStickyPin(ctx, task)

	dispatchUpstream(w, r, ctx)
}

// HandleVideosDelete handles DELETE /v1/videos/{id}.

// Clears any local mapping for the public id, then always dispatches DELETE to
// upstream (mapping is optional rewrite aid, not a hard gate). Prefer honest
// upstream status over a local-only 204.

// Sticky pin: resolve mapping before delete so channel preference and
// path rewrite still apply for this request.
func HandleVideosDelete(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if publicID == "" {
		writeJSONError(w, 400, "missing video id", "invalid_request_error")
		return
	}

	task := GetProxyVideoTaskByPublicID(publicID)
	upstreamID := resolveVideoUpstreamIDFromTask(publicID, task)
	// Best-effort local cleanup whether or not a mapping existed.
	DeleteProxyVideoTaskByPublicID(publicID)

	ctx, errResp := PrepareCtx(r, SurfConfig{
		Endpoint:       "videos",
		DownstreamPath: "/v1/videos/" + upstreamID,
		RequireModel:   false,
	})
	if errResp != nil {
		writeJSONError(w, errResp.Status, errResp.Error, errResp.ErrorType)
		return
	}
	applyVideoTaskStickyPin(ctx, task)

	dispatchUpstream(w, r, ctx)
}

// resolveVideoUpstreamIDFromTask returns the upstream path id for a client-facing id.
// When a mapping exists with a non-empty UpstreamVideoID different from publicID,
// that upstream id is used; otherwise publicID is passed through unchanged.
func resolveVideoUpstreamIDFromTask(publicID string, task *ProxyVideoTask) string {
	if task == nil {
		return publicID
	}
	if task.UpstreamVideoID != "" && task.UpstreamVideoID != publicID {
		return task.UpstreamVideoID
	}
	return publicID
}

// applyVideoTaskStickyPin forces preferred channel selection when the mapping
// recorded a channel at create time. Also seeds RequestedModel from the
// mapping when the client omitted model (typical for GET/DELETE).
func applyVideoTaskStickyPin(ctx *Ctx, task *ProxyVideoTask) {
	if ctx == nil || task == nil {
		return
	}
	if ctx.RequestedModel == "" {
		if task.RequestedModel != "" {
			ctx.RequestedModel = task.RequestedModel
		} else if task.ActualModel != "" {
			ctx.RequestedModel = task.ActualModel
		}
	}
	if task.ChannelID > 0 {
		ch := task.ChannelID
		ctx.ForcedChannelID = &ch
	}
}

// SaveProxyVideoTask saves a video task mapping to the process-local cache and
// dual-writes to proxy_video_tasks when a runtime DB is available.
// Both copies carry the same timestamp and both are bounded by
// PROXY_VIDEO_TASK_RETENTION_DAYS (cache: lazy sweep; DB: retention scheduler).
func SaveProxyVideoTask(task *ProxyVideoTask) {
	if task == nil || strings.TrimSpace(task.PublicID) == "" {
		return
	}
	// Store a copy so callers cannot mutate the map entry after return.
	cp := *task
	cp.PublicID = strings.TrimSpace(cp.PublicID)
	cp.UpstreamVideoID = strings.TrimSpace(cp.UpstreamVideoID)

	now := videoTaskNow().UTC()
	videoTaskStoreMu.Lock()
	putVideoTaskEntryLocked(cp.PublicID, &cp, now)
	videoTaskStoreMu.Unlock()

	if err := upsertProxyVideoTaskDB(&cp, now); err != nil {
		slog.Warn("proxy video task: durable upsert failed (memory cache still set)",
			"public_id", cp.PublicID, "error", err)
	}
}

// GetProxyVideoTaskByPublicID retrieves a video task by publicId.
// Memory cache first; cold miss falls back to proxy_video_tasks.
// A cache line past the retention TTL counts as a miss even before the next
// (throttled) sweep deletes it; the durable table then decides exactly as it
// would for a cold process, so a row the scheduler has not pruned yet re-warms
// the line with a fresh storedAt.
func GetProxyVideoTaskByPublicID(publicID string) *ProxyVideoTask {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return nil
	}
	videoTaskStoreMu.RLock()
	entry := videoTaskStore[publicID]
	if entry != nil && entry.task != nil && !videoTaskCacheEntryExpired(entry) {
		cp := *entry.task
		videoTaskStoreMu.RUnlock()
		return &cp
	}
	videoTaskStoreMu.RUnlock()

	loaded, err := loadProxyVideoTaskDB(publicID)
	if err != nil {
		slog.Debug("proxy video task: durable load failed", "public_id", publicID, "error", err)
		return nil
	}
	if loaded == nil {
		return nil
	}
	// Warm process-local cache (a warm load is an insert, so it stamps its own
	// storedAt and takes part in sweep bookkeeping like a create does).
	now := videoTaskNow().UTC()
	videoTaskStoreMu.Lock()
	putVideoTaskEntryLocked(loaded.PublicID, loaded, now)
	videoTaskStoreMu.Unlock()
	cp := *loaded
	return &cp
}

// DeleteProxyVideoTaskByPublicID deletes a video task by publicId (memory + DB).
func DeleteProxyVideoTaskByPublicID(publicID string) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return
	}
	videoTaskStoreMu.Lock()
	delete(videoTaskStore, publicID)
	videoTaskStoreMu.Unlock()

	if err := deleteProxyVideoTaskDB(publicID); err != nil {
		slog.Warn("proxy video task: durable delete failed", "public_id", publicID, "error", err)
	}
}

// putVideoTaskEntryLocked inserts or refreshes a cache line and runs the
// amortized eviction check. Callers must hold videoTaskStoreMu for writing.
func putVideoTaskEntryLocked(publicID string, task *ProxyVideoTask, now time.Time) {
	videoTaskStore[publicID] = &videoTaskEntry{task: task, storedAt: now}
	videoTaskInsertsSinceSweep++
	maybeSweepVideoTaskCacheLocked(now)
}

// maybeSweepVideoTaskCacheLocked runs at most one sweep per
// videoTaskSweepEveryInserts inserts or per videoTaskSweepMinInterval, which is
// what keeps the insert path amortized O(1): the two thresholds bound (a) how
// far the map can overshoot the cap during a burst and (b) how long an entry
// can sit past its TTL in an idle-ish process. Callers must hold the write lock.
func maybeSweepVideoTaskCacheLocked(now time.Time) {
	due := videoTaskInsertsSinceSweep >= videoTaskSweepEveryInserts ||
		now.Sub(videoTaskLastSweepAt) >= videoTaskSweepMinInterval
	if !due {
		return
	}
	videoTaskInsertsSinceSweep = 0
	videoTaskLastSweepAt = now
	sweepVideoTaskCacheLocked(now)
}

// sweepVideoTaskCacheLocked is the single eviction pass: one O(n) walk for TTL
// expiry plus a capacity trim when the guardrail is exceeded. It never touches
// the DB — proxy_video_tasks rows are the retention scheduler's to prune.
// Callers must hold videoTaskStoreMu for writing.
func sweepVideoTaskCacheLocked(now time.Time) {
	var expired int
	if ttl := videoTaskCacheTTL(); ttl > 0 {
		cutoff := now.Add(-ttl)
		for id, entry := range videoTaskStore {
			if entry == nil || entry.storedAt.Before(cutoff) {
				delete(videoTaskStore, id)
				expired++
			}
		}
	}
	trimmed := trimVideoTaskCacheLocked()
	if expired+trimmed > 0 {
		slog.Debug("proxy video task cache: sweep",
			"expired", expired,
			"over_cap_trimmed", trimmed,
			"remaining", len(videoTaskStore),
		)
	}
}

// trimVideoTaskCacheLocked enforces the hard capacity guardrail, evicting
// oldest-first down to the headroom target so a cache pinned at the cap does
// not pay for a trim on every insert. Returns the number of lines dropped.
// Callers must hold videoTaskStoreMu for writing.
func trimVideoTaskCacheLocked() int {
	if len(videoTaskStore) <= videoTaskCacheMaxEntries {
		return 0
	}
	target := videoTaskCacheMaxEntries - videoTaskCacheMaxEntries/videoTaskCacheHeadroomDivisor
	drop := len(videoTaskStore) - target
	if drop < 1 || drop > len(videoTaskStore) {
		// Only reachable for a nonsensical (<= 0) cap, which means "hold nothing".
		drop = len(videoTaskStore)
	}
	oldest := make([]videoTaskEntryRef, 0, len(videoTaskStore))
	for id, entry := range videoTaskStore {
		ref := videoTaskEntryRef{id: id}
		if entry != nil {
			ref.storedAt = entry.storedAt
		}
		oldest = append(oldest, ref)
	}
	slices.SortFunc(oldest, func(a, b videoTaskEntryRef) int { return a.storedAt.Compare(b.storedAt) })
	for _, ref := range oldest[:drop] {
		delete(videoTaskStore, ref.id)
	}
	return drop
}

// videoTaskEntryRef is a (id, storedAt) pair used to order lines for the
// capacity trim without holding pointers into the map while deleting from it.
type videoTaskEntryRef struct {
	id       string
	storedAt time.Time
}

// videoTaskCacheTTL reports how long a cache line may live, derived from the
// same PROXY_VIDEO_TASK_RETENTION_DAYS knob that prunes proxy_video_tasks.
// 0 means TTL eviction is off — either config is not loaded yet or an operator
// explicitly disabled retention — and only the capacity guardrail applies.
func videoTaskCacheTTL() time.Duration {
	cfg := config.GetSafe()
	if cfg == nil || cfg.ProxyVideoTaskRetentionDays <= 0 {
		return 0
	}
	return time.Duration(cfg.ProxyVideoTaskRetentionDays) * 24 * time.Hour
}

// videoTaskCacheEntryExpired reports whether a line is past its TTL. Reads use
// it to treat an expired line as a miss; deletion still belongs to the sweep,
// so the read path never needs the write lock.
func videoTaskCacheEntryExpired(entry *videoTaskEntry) bool {
	ttl := videoTaskCacheTTL()
	if ttl <= 0 || entry == nil {
		return false
	}
	return videoTaskNow().UTC().Sub(entry.storedAt) >= ttl
}

func upsertProxyVideoTaskDB(task *ProxyVideoTask, at time.Time) error {
	db := store.GetDB()
	if db == nil || task == nil {
		return nil
	}
	if strings.TrimSpace(task.PublicID) == "" || strings.TrimSpace(task.UpstreamVideoID) == "" {
		return nil
	}
	now := at.UTC().Format(time.RFC3339)
	// store.DB rebinds ? → $N for postgres.
	const q = `
INSERT INTO proxy_video_tasks (
	public_id, upstream_video_id, site_url, token_value,
	requested_model, actual_model, channel_id, account_id,
	created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(public_id) DO UPDATE SET
	upstream_video_id = excluded.upstream_video_id,
	site_url = excluded.site_url,
	token_value = excluded.token_value,
	requested_model = excluded.requested_model,
	actual_model = excluded.actual_model,
	channel_id = excluded.channel_id,
	account_id = excluded.account_id,
	updated_at = excluded.updated_at
`
	var reqModel, actModel any
	if strings.TrimSpace(task.RequestedModel) != "" {
		reqModel = task.RequestedModel
	}
	if strings.TrimSpace(task.ActualModel) != "" {
		actModel = task.ActualModel
	}
	var channelID, accountID any
	if task.ChannelID > 0 {
		channelID = task.ChannelID
	}
	if task.AccountID > 0 {
		accountID = task.AccountID
	}
	_, err := db.Exec(q,
		task.PublicID,
		task.UpstreamVideoID,
		task.SiteURL,
		task.TokenValue,
		reqModel,
		actModel,
		channelID,
		accountID,
		now,
		now,
	)
	return err
}

func loadProxyVideoTaskDB(publicID string) (*ProxyVideoTask, error) {
	db := store.GetDB()
	if db == nil {
		return nil, nil
	}
	const q = `
SELECT public_id, upstream_video_id, site_url, token_value,
       requested_model, actual_model, channel_id, account_id
FROM proxy_video_tasks
WHERE public_id = ?
LIMIT 1
`
	var (
		rowPublic, upstream, siteURL, token string
		reqModel, actModel                  *string
		channelID, accountID                *int64
	)
	err := db.QueryRow(q, publicID).Scan(
		&rowPublic, &upstream, &siteURL, &token,
		&reqModel, &actModel, &channelID, &accountID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	task := &ProxyVideoTask{
		PublicID:        rowPublic,
		UpstreamVideoID: upstream,
		SiteURL:         siteURL,
		TokenValue:      token,
	}
	if reqModel != nil {
		task.RequestedModel = *reqModel
	}
	if actModel != nil {
		task.ActualModel = *actModel
	}
	if channelID != nil {
		task.ChannelID = *channelID
	}
	if accountID != nil {
		task.AccountID = *accountID
	}
	return task, nil
}

func deleteProxyVideoTaskDB(publicID string) error {
	db := store.GetDB()
	if db == nil {
		return nil
	}
	_, err := db.Exec(`DELETE FROM proxy_video_tasks WHERE public_id = ?`, publicID)
	return err
}

// maybeRewriteVideosCreateResponse rewrites a successful POST /v1/videos body so
// clients see an opaque publicId, and seeds the process-local mapping used by
// GET/DELETE path rewrite. Best-effort: any parse/shape miss returns body unchanged.

// Multi-instance durable store and sticky site pin are known limitations (follow-ups).
func maybeRewriteVideosCreateResponse(
	ctx *Ctx,
	selected *routing.SelectedChannel,
	upstreamPath string,
	body []byte,
) []byte {
	if ctx == nil || selected == nil || len(body) == 0 {
		return body
	}
	path := strings.TrimSpace(upstreamPath)
	if path == "" {
		path = strings.TrimSpace(ctx.DownstreamPath)
	}
	path = strings.TrimSuffix(path, "/")
	if !strings.EqualFold(path, "/v1/videos") {
		return body
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	upstreamID, _ := payload["id"].(string)
	upstreamID = strings.TrimSpace(upstreamID)
	if upstreamID == "" {
		return body
	}

	publicID := newPublicVideoID()
	actualModel := selected.ActualModel
	if actualModel == "" {
		actualModel = ctx.RequestedModel
	}
	SaveProxyVideoTask(&ProxyVideoTask{
		PublicID:        publicID,
		UpstreamVideoID: upstreamID,
		SiteURL:         selected.Site.URL,
		TokenValue:      selected.TokenValue,
		RequestedModel:  ctx.RequestedModel,
		ActualModel:     actualModel,
		ChannelID:       selected.Channel.ID,
		AccountID:       selected.Account.ID,
	})

	payload["id"] = publicID
	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

func newPublicVideoID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "video_" + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return "video_" + hex.EncodeToString(b[:])
}
