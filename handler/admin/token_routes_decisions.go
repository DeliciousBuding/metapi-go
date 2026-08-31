package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
)

func (h *tokenRoutesHandler) routeDecision(w http.ResponseWriter, r *http.Request) {
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	if model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}
	if h.router == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, ErrorCodeResourceDisabled, routeDecisionRouterUnavailable)
		return
	}

	decision, err := h.router.ExplainSelection(r.Context(), model, nil, routing.EmptyDownstreamRoutingPolicy)
	if err != nil {
		slog.Error("route decision explain failed", "model", model, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to query route decisions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"decision": routeDecisionToMap(decision),
	})
}

// POST /api/routes/decision/batch
func (h *tokenRoutesHandler) routeDecisionBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Models                []string `json:"models"`
		RefreshPricingCatalog bool     `json:"refreshPricingCatalog"`
		PersistSnapshots      bool     `json:"persistSnapshots"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	models := uniqueNonEmptyStrings(body.Models)
	if len(models) == 0 {
		writeError(w, http.StatusBadRequest, "models must be a non-empty array")
		return
	}
	if len(models) > routeDecisionBatchMaxItems {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("models accepts at most %d items", routeDecisionBatchMaxItems))
		return
	}
	if h.router == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, ErrorCodeResourceDisabled, routeDecisionRouterUnavailable)
		return
	}

	_ = body.RefreshPricingCatalog
	_ = body.PersistSnapshots

	decisions := make(map[string]any, len(models))
	for _, model := range models {
		decision, err := h.router.ExplainSelection(r.Context(), model, nil, routing.EmptyDownstreamRoutingPolicy)
		if err != nil {
			slog.Warn("route decision batch item failed", "model", model, "error", err)
			continue
		}
		decisions[model] = routeDecisionToMap(decision)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"decisions": decisions,
	})
}

// POST /api/routes/decision/by-route/batch
func (h *tokenRoutesHandler) routeDecisionByRouteBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			RouteID int64  `json:"routeId"`
			Model   string `json:"model"`
		} `json:"items"`
		RefreshPricingCatalog bool `json:"refreshPricingCatalog"`
		PersistSnapshots      bool `json:"persistSnapshots"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items must be a non-empty array")
		return
	}
	if len(body.Items) > routeDecisionBatchMaxItems {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("items accepts at most %d items", routeDecisionBatchMaxItems))
		return
	}
	if h.router == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, ErrorCodeResourceDisabled, routeDecisionRouterUnavailable)
		return
	}

	_ = body.RefreshPricingCatalog
	_ = body.PersistSnapshots

	// Nested map: routeId -> model -> decision
	decisions := map[string]map[string]any{}
	for _, item := range body.Items {
		model := strings.TrimSpace(item.Model)
		if item.RouteID <= 0 || model == "" {
			continue
		}
		decision, err := h.router.ExplainSelectionForRoute(r.Context(), item.RouteID, model, nil, routing.EmptyDownstreamRoutingPolicy)
		if err != nil {
			slog.Warn("route decision by-route item failed", "routeId", item.RouteID, "model", model, "error", err)
			continue
		}
		routeKey := strconv.FormatInt(item.RouteID, 10)
		if decisions[routeKey] == nil {
			decisions[routeKey] = map[string]any{}
		}
		decisions[routeKey][model] = routeDecisionToMap(decision)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"decisions": decisions,
	})
}

// POST /api/routes/decision/route-wide/batch
func (h *tokenRoutesHandler) routeDecisionRouteWideBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RouteIDs              []int64 `json:"routeIds"`
		RefreshPricingCatalog bool    `json:"refreshPricingCatalog"`
		PersistSnapshots      bool    `json:"persistSnapshots"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	routeIDs := uniquePositiveInt64(body.RouteIDs)
	if len(routeIDs) == 0 {
		writeError(w, http.StatusBadRequest, "routeIds must be a non-empty array")
		return
	}
	if len(routeIDs) > routeDecisionBatchMaxItems {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("routeIds accepts at most %d items", routeDecisionBatchMaxItems))
		return
	}
	if h.router == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, ErrorCodeResourceDisabled, routeDecisionRouterUnavailable)
		return
	}

	_ = body.RefreshPricingCatalog
	_ = body.PersistSnapshots

	decisions := make(map[string]any, len(routeIDs))
	for _, routeID := range routeIDs {
		decision, err := h.router.ExplainSelectionRouteWide(r.Context(), routeID, routing.EmptyDownstreamRoutingPolicy)
		if err != nil {
			slog.Warn("route decision route-wide item failed", "routeId", routeID, "error", err)
			continue
		}
		decisions[strconv.FormatInt(routeID, 10)] = routeDecisionToMap(decision)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"decisions": decisions,
	})
}

// POST /api/routes/decision/refresh
func (h *tokenRoutesHandler) routeDecisionRefresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshPricingCatalog bool `json:"refreshPricingCatalog"`
	}
	// Empty body is allowed; ignore decode errors for {}.
	_ = decodeJSONRequest(r, &body)

	if h.decisions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"queued":  false,
			"message": routeDecisionRouterUnavailable,
		})
		return
	}

	refresher := h.decisions
	refreshPricing := body.RefreshPricingCatalog
	task, reused := StartBackgroundTask(BackgroundTaskStartOptions{
		Type:      routeDecisionRefreshTaskType,
		Title:     routeDecisionRefreshTaskTitle,
		DedupeKey: routeDecisionRefreshDedupeKey,
	}, func() (any, error) {
		exact, wildcard, err := refresher.RefreshAllRouteDecisionSnapshots(context.Background(), refreshPricing)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"exactModelCount":       exact,
			"wildcardRouteCount":    wildcard,
			"refreshPricingCatalog": refreshPricing,
		}, nil
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"queued":  true,
		"reused":  reused,
		"jobId":   task.ID,
		"taskId":  task.ID,
		"status":  string(task.Status),
		"message": "route selection probability refresh started in the background; check back later",
	})
}
