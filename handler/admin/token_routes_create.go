package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Create Route ----
// POST /api/routes
func (h *tokenRoutesHandler) createRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ModelPattern    string  `json:"modelPattern"`
		RouteMode       *string `json:"routeMode"`
		DisplayName     *string `json:"displayName"`
		DisplayIcon     *string `json:"displayIcon"`
		SourceRouteIds  []int64 `json:"sourceRouteIds"`
		RoutingStrategy *string `json:"routingStrategy"`
		TraceStickyMode *string `json:"traceStickyMode"`
		// ContextLength is optional route metadata (tokens). NULL/omit/0 means unknown;
		// not enforced at proxy runtime in currently (admin surface + /v1/models known limitation).
		ContextLength any   `json:"contextLength"`
		ModelMapping  any   `json:"modelMapping"`
		Enabled       *bool `json:"enabled"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	routeMode := "pattern"
	if body.RouteMode != nil {
		routeMode = strings.TrimSpace(*body.RouteMode)
	}

	modelPattern := strings.TrimSpace(body.ModelPattern)
	displayName := ""
	if body.DisplayName != nil {
		displayName = strings.TrimSpace(*body.DisplayName)
	}

	if routeMode == "explicit_group" {
		modelPattern = displayName
	}

	if routeMode != "explicit_group" && modelPattern == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "model match pattern is required")
		return
	}

	if routeMode == "explicit_group" && displayName == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "explicit groups must specify an external model name")
		return
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	routingStrategy := "weighted"
	if body.RoutingStrategy != nil {
		routingStrategy = *body.RoutingStrategy
	}
	sticky := "disabled"
	if body.TraceStickyMode != nil {
		sticky = *body.TraceStickyMode
	}
	if !store.ValidTraceStickyMode(sticky) {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid traceStickyMode")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	var modelMapping any
	if body.ModelMapping != nil {
		if b, err := json.Marshal(body.ModelMapping); err == nil {
			modelMapping = string(b)
		}
	}

	contextLength := normalizeContextLengthOrNull(body.ContextLength)

	id, err := execInsertID(h.db,
		`INSERT INTO token_routes (model_pattern, display_name, display_icon, route_mode, model_mapping, routing_strategy, trace_sticky_mode, context_length, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		modelPattern, strOrNull(&displayName), strOrNull(body.DisplayIcon), routeMode,
		modelMapping, routingStrategy, sticky, contextLength, enabled, now, now,
	)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to create route")
		return
	}

	// For explicit_group, insert source route references
	if routeMode == "explicit_group" && len(body.SourceRouteIds) > 0 {
		for _, srcID := range body.SourceRouteIds {
			if _, err := h.db.Exec(h.db.Rebind("INSERT INTO route_group_sources (group_route_id, source_route_id) VALUES (?, ?)"), id, srcID); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to create route group source")
				return
			}
		}
	}

	// Pattern routes: auto-populate channels from exact-model routes + model availability.
	if routeMode != "explicit_group" && modelPattern != "" {
		if _, err := service.PopulateRouteChannelsByModelPattern(r.Context(), h.db, id, modelPattern); err != nil {
			slog.Warn("route create: channel auto-populate failed", "routeId", id, "error", err)
		}
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	created, err := queryRow(h.db, "SELECT * FROM token_routes WHERE id = ?", id)
	if err != nil || created == nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "route created but failed to load result")
		return
	}
	created["sourceRouteIds"] = body.SourceRouteIds
	writeJSON(w, http.StatusOK, created)
}

// normalizeContextLengthOrNull parses admin camelCase contextLength.
// Contract: null / omit (caller) / "" / 0 / negative → NULL (unknown, no enforcement).
// Positive integers (or numeric strings) are stored as token window metadata only.
func normalizeContextLengthOrNull(input any) any {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case float64:
		if v <= 0 {
			return nil
		}
		return int64(v)
	case float32:
		if v <= 0 {
			return nil
		}
		return int64(v)
	case int:
		if v <= 0 {
			return nil
		}
		return int64(v)
	case int64:
		if v <= 0 {
			return nil
		}
		return v
	case json.Number:
		i, err := v.Int64()
		if err != nil || i <= 0 {
			return nil
		}
		return i
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil || i <= 0 {
			return nil
		}
		return i
	default:
		return nil
	}
}

func strOrNull(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}
