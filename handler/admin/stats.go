package admin

import (
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterStatsRoutes registers all /api/stats and /api/models routes.
func RegisterStatsRoutes(r chi.Router, db *store.DB) {
	handler := &statsHandler{db: db}

	r.Get("/api/stats/dashboard", handler.dashboard)
	r.Get("/api/stats/overview", handler.overview)
	r.Get("/api/stats/flow", handler.flow)
	r.Get("/api/stats/requests", handler.gatewayRequests)
	r.Get("/api/stats/requests/{id}", handler.gatewayRequestDetail)
	r.Get("/api/stats/proxy-logs", handler.proxyLogs)
	r.Get("/api/stats/proxy-logs/{id}", handler.proxyLogDetail)
	r.Get("/api/stats/site-distribution", handler.siteDistribution)
	r.Get("/api/stats/site-trend", handler.siteTrend)
	r.Get("/api/stats/model-by-site", handler.modelBySite)
	// Usage density heatmap + slow-request ranking.
	r.Get("/api/stats/usage-heatmap", handler.usageHeatmap)
	r.Get("/api/stats/slow-requests", handler.slowRequests)
	// A1: per-account daily balance history series.
	r.Get("/api/stats/balance-history", handler.balanceHistory)
	// A3: income vs outcome balance analysis.
	r.Get("/api/stats/balance-income-outcome", handler.balanceIncomeOutcome)
	// B1: severity-ranked actionable attention items.
	r.Get("/api/stats/attention", handler.attention)
	// A2: model cost distribution + latency chart gallery.
	r.Get("/api/stats/model-cost-distribution", handler.modelCostDistribution)
	r.Get("/api/stats/latency-histogram", handler.latencyHistogram)
	r.Get("/api/stats/latency-trend", handler.latencyTrend)
	// Cross-site effective model price comparison (admin).
	// Both paths are registered for discoverability; they share one handler.
	r.Get("/api/stats/model-prices", handler.modelPriceCompare)

	// Model routes under /api/models
	r.Get("/api/models/marketplace", handler.marketplace)
	r.Get("/api/models/marketplace/detail", handler.marketplaceDetail)
	r.Get("/api/models/price-compare", handler.modelPriceCompare)
	r.Post("/api/models/price-compare/accounts/{accountId}/refresh", handler.refreshPriceCompareAccount)
	r.Get("/api/models/token-candidates", handler.tokenCandidates)
	r.Post("/api/models/check/{accountId}", handler.modelCheck)
	r.Post("/api/models/probe", handler.modelProbe)
	// G1: batch model verification + history.
	r.Post("/api/models/verify-batch", handler.verifyBatch)
	r.Get("/api/models/verify-history", handler.verifyHistory)
}

// RegisterDownstreamPricingRoutes mounts the cross-site price catalog behind
// downstream-key (ProxyAuth) auth, NOT admin auth. Productized so that a
// downstream consumer (中转站) holding a managed key can query effective
// cross-site model pricing for its own planning — the aggregator's
// transparent-pricing differentiator. Uses a dedicated handler that applies
// the key's routing policy to sites, models and credential refs and strips
// account identities; the admin modelPriceCompare surface stays admin-only.

// Mounted under /v1/* so it inherits ProxyAuth + CORS from the /v1 route group.
func RegisterDownstreamPricingRoutes(r chi.Router, db *store.DB) {
	handler := &statsHandler{db: db}
	// Paths are relative: this router is mounted inside r.Route("/v1", ...),
	// so absolute "/v1/..." here would double the prefix.
	r.Get("/pricing", handler.downstreamModelPriceCompare)
	r.Get("/models/price-compare", handler.downstreamModelPriceCompare)
}

type statsHandler struct {
	db *store.DB
}

// usageHeatmapCellLimit caps density rows returned to admin clients.
// 31d * 24h * ~50 keys is more than enough for a dense operator view.
const usageHeatmapCellLimit = 2000
