package admin

import (
	"context"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RouteDecisionExplainer is the router surface used by decision admin APIs.
// Tests inject fakes; production wires routing.TokenRouter.
type RouteDecisionExplainer interface {
	ExplainSelection(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error)
	ExplainSelectionForRoute(ctx context.Context, routeID int64, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error)
	ExplainSelectionRouteWide(ctx context.Context, routeID int64, policy routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error)
}

// RouteDecisionRefresher refreshes persisted route decision snapshots.
type RouteDecisionRefresher interface {
	RefreshAllRouteDecisionSnapshots(ctx context.Context, refreshPricingCatalog bool) (exactModelCount int, wildcardRouteCount int, err error)
}

// TokenRoutesDeps are optional dependencies for route decision endpoints.
type TokenRoutesDeps struct {
	Router    RouteDecisionExplainer
	Decisions RouteDecisionRefresher
}

// RegisterTokenRoutesWithDeps registers routes with optional decision engine deps.
func RegisterTokenRoutesWithDeps(r chi.Router, db *sqlx.DB, deps TokenRoutesDeps) {
	handler := &tokenRoutesHandler{db: db, router: deps.Router, decisions: deps.Decisions}

	// Route list
	r.Get("/api/routes/lite", handler.listLite)
	r.Get("/api/routes/summary", handler.listSummary)
	r.Get("/api/routes", handler.listRoutes)

	// Route CRUD
	r.Post("/api/routes", handler.createRoute)
	r.Post("/api/routes/batch", handler.batchRoutes)
	r.Put("/api/routes/reorder", handler.reorderRoutes) // static path before /{id}
	r.Post("/api/routes/rebuild", handler.rebuildRoutes)
	r.Put("/api/routes/{id}", handler.updateRoute)
	r.Delete("/api/routes/{id}", handler.deleteRoute)

	// Route channels
	r.Get("/api/routes/{id}/channels", handler.getRouteChannels)
	r.Post("/api/routes/{id}/channels", handler.addChannel)
	r.Post("/api/routes/{id}/channels/batch", handler.batchAddChannels)
	r.Post("/api/routes/{id}/cooldown/clear", handler.clearCooldown)
	r.Post("/api/channels/{channelId}/cooldown/clear", handler.clearChannelCooldown)

	// Channel operations
	r.Get("/api/channels", handler.listChannels)
	r.Get("/api/channels/error-summary", handler.channelErrorSummary)
	r.Get("/api/channels/probe-history", handler.channelProbeHistory)
	r.Put("/api/channels/batch", handler.batchUpdateChannels)
	r.Put("/api/channels/{channelId}", handler.updateChannel)
	r.Delete("/api/channels/{channelId}", handler.deleteChannel)

	// Route decisions
	r.Get("/api/routes/decision", handler.routeDecision)
	r.Get("/api/routes/decision/key-context", handler.routeDecisionKeyContext)
	r.Post("/api/routes/decision/batch", handler.routeDecisionBatch)
	r.Post("/api/routes/decision/by-route/batch", handler.routeDecisionByRouteBatch)
	r.Post("/api/routes/decision/route-wide/batch", handler.routeDecisionRouteWideBatch)
	r.Post("/api/routes/decision/refresh", handler.routeDecisionRefresh)
}

type tokenRoutesHandler struct {
	db        *sqlx.DB
	router    RouteDecisionExplainer
	decisions RouteDecisionRefresher
}

func execInsertID(db *sqlx.DB, query string, args ...any) (int64, error) {
	if db.DriverName() == "pgx" {
		var id int64
		err := db.QueryRowx(db.Rebind(query+" RETURNING id"), args...).Scan(&id)
		return id, err
	}

	result, err := db.Exec(db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
