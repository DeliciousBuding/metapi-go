package proxyhandler

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

// defaultLogProxyWriter inserts a proxy.ProxyLogEntry into proxy_logs.
// Prefer injecting via UpstreamConfig.LogProxy so tests can capture writes.
func defaultLogProxyWriter(ctx context.Context, entry proxy.ProxyLogEntry) error {
	return InsertProxyLog(ctx, proxyScopeFromContext(ctx).db, entry)
}

// Store only known API surfaces, never model IDs, task IDs or URL parameters.
func proxyLogDownstreamPath(ctx *Ctx) *string {
	if ctx == nil {
		return nil
	}
	path := strings.TrimRight(strings.SplitN(ctx.DownstreamPath, "?", 2)[0], "/")
	switch path {
	case "/v1/messages/count_tokens", "/v1/responses/compact",
		"/v1internal::generateContent", "/v1internal::streamGenerateContent", "/v1internal::countTokens",
		"/v1/completions", "/v1/embeddings", "/v1/rerank", "/v1/search",
		"/v1/images/generations", "/v1/images/edits", "/v1/images/variations",
		"/v1/audio/speech", "/v1/audio/transcriptions", "/v1/audio/translations",
		"/v1/moderations", "/v1/videos", "/v1/systemone", "/v1/alpha/search", "/v1/decisions":
		return &path
	}
	if rest, ok := strings.CutPrefix(path, "/v1/videos/"); ok && strings.HasSuffix(rest, "/remix") {
		id := strings.TrimSuffix(rest, "/remix")
		if id != "" && !strings.Contains(id, "/") {
			normalized := "/v1/videos/{id}/remix"
			return &normalized
		}
	}
	// Embeddings are a separate non-generation protocol, so EndpointFromPath
	// deliberately does not identify them. Reuse the Gemini path parser while
	// keeping observation scope to registered surfaces and model IDs out of logs.
	if strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/gemini/") {
		version, model, action := ParseGeminiPath(path)
		if version != "" && model != "" && (action == "embedContent" || action == "batchEmbedContents") {
			path = "/v1beta/models/{model}:" + action
			return &path
		}
	}
	endpoint, ok := proxy.EndpointFromPath(path)
	if !ok {
		return nil
	}
	if endpoint == proxy.EndpointGemini {
		if strings.HasSuffix(path, ":streamGenerateContent") {
			path = "/v1beta/models/{model}:streamGenerateContent"
		} else {
			path = "/v1beta/models/{model}:generateContent"
		}
	} else {
		path = proxy.PathForEndpoint(endpoint)
	}
	return &path
}

func logProxy(ctx context.Context, cfg *UpstreamConfig, entry proxy.ProxyLogEntry) {
	if cfg == nil {
		return
	}
	if pac := auth.GetProxyAuth(ctx); pac != nil {
		entry.OwnerUserID, entry.UserGroupID = pac.OwnerUserID, pac.UserGroupID
	}
	if wire := directProviderWireFromContext(ctx); wire != nil && wire.ProtocolSession != nil {
		entry.ConversionDetails = wire.ProtocolSession.ConversionDetails()
	}
	requestLifecycleFromContext(ctx).recordAttempt(ctx, &entry)
	// Prefer explicit entry.RequestID; otherwise inherit chi/Metapi request id.
	if entry.RequestID == "" {
		entry.RequestID = proxy.RequestIDFromContext(ctx)
	}
	alreadyAccounted, _ := ctx.Value(directVideoAccountingLogKey{}).(bool)
	if !alreadyAccounted && entry.DownstreamAPIKeyID != nil && (entry.Status == "success" || entry.TotalTokens != nil) {
		var cost *float64
		if entry.TotalTokens != nil {
			cost = &entry.EstimatedCost
		}
		auth.RecordManagedKeyQuotaUsage(proxyScopeFromContext(ctx).db, *entry.DownstreamAPIKeyID, quotaReservationID(ctx, *entry.DownstreamAPIKeyID), entry.TotalTokens, cost)
	}
	writer := cfg.LogProxy
	if writer == nil {
		writer = defaultLogProxyWriter
	}
	// An attempt is a completion observation, just like its gateway request
	// record. Caller cancellation must not erase it in synchronous mode or
	// when the async queue overflows, but a failed database must remain bounded.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if err := writer(writeCtx, entry); err != nil {
		slog.Warn("LogProxy failed",
			"err", err,
			"status", entry.Status,
			"model", entry.ModelRequested,
			"request_id", entry.RequestID,
			"retry_count", entry.RetryCount,
		)
	}
}

// quotaReservationID returns the admission hold carried on this request's auth
// context when it belongs to the same managed key. A mismatch (or a missing
// auth context in tests) settles without a reservation, which keeps the usage
// row authoritative; the admission middleware releases any orphaned hold.
func quotaReservationID(ctx context.Context, keyID int64) string {
	pac := auth.GetProxyAuth(ctx)
	if pac == nil || pac.KeyID == nil || *pac.KeyID != keyID {
		return ""
	}
	return pac.QuotaReservationID
}

// recordManagedKeyCostOnSuccess increments used_cost for managed keys after a
// successful proxy attempt. Nil KeyID (global token) is a no-op; zero/NaN/Inf
// costs are skipped by auth.RecordManagedKeyCostUsage itself.
func recordManagedKeyCostOnSuccess(db *store.DB, keyID *int64, estimatedCost float64) {
	if keyID == nil {
		return
	}
	auth.RecordManagedKeyCostUsage(db, *keyID, estimatedCost)
}

// truncateErrText bounds proxy_logs.error_message size for large upstream bodies.
func truncateErrText(s string) string {
	const maxErrRunes = 2000
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= maxErrRunes {
		return s
	}
	return string(r[:maxErrRunes]) + "..."
}
