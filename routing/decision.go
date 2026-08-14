package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// RouteDecisionService handles route decision snapshots.
type RouteDecisionService struct {
	router *TokenRouter
	db     DecisionDB
}

// DecisionDB defines the DB operations for decision snapshots.
// FindAllEnabledRoutes reuses the TokenRoute shape so the same store
// adapter that backs TokenRouter can also back snapshot refresh.
type DecisionDB interface {
	FindAllEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error)
	UpdateRouteDecisionSnapshot(ctx context.Context, routeID int64, snapshot string, refreshedAt string) error
	ClearRouteDecisionSnapshot(ctx context.Context, routeID int64) error
	ClearRouteDecisionSnapshots(ctx context.Context, routeIDs []int64) error
	ClearAllRouteDecisionSnapshots(ctx context.Context) error
}

// NewRouteDecisionService creates a new RouteDecisionService.
func NewRouteDecisionService(router *TokenRouter, db DecisionDB) *RouteDecisionService {
	return &RouteDecisionService{
		router: router,
		db:     db,
	}
}

// RefreshAllRouteDecisionSnapshots refreshes all route decision snapshots.
func (s *RouteDecisionService) RefreshAllRouteDecisionSnapshots(ctx context.Context, refreshPricingCatalog bool) (exactModelCount int, wildcardRouteCount int, err error) {
	routes, err := s.db.FindAllEnabledRoutes(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("refreshAllRouteDecisionSnapshots: %w", err)
	}

	var exactModels []string
	var wildcardRouteIDs []int64

	for _, route := range routes {
		if IsExactRouteModelPattern(route.ModelPattern) {
			exactModels = append(exactModels, route.ModelPattern)
		} else {
			wildcardRouteIDs = append(wildcardRouteIDs, route.ID)
		}
	}

	_ = refreshPricingCatalog

	for _, model := range exactModels {
		// Find matching exact routes
		for _, route := range routes {
			if IsExactRouteModelPattern(route.ModelPattern) && MatchesModelPattern(model, route.ModelPattern) {
				decision, err := s.router.ExplainSelectionForRoute(ctx, route.ID, model, nil, EmptyDownstreamRoutingPolicy)
				if err != nil {
					continue
				}
				_ = s.saveRouteDecisionSnapshot(ctx, route.ID, decision)
			}
		}
	}

	for _, routeID := range wildcardRouteIDs {
		decision, err := s.router.ExplainSelectionRouteWide(ctx, routeID, EmptyDownstreamRoutingPolicy)
		if err != nil {
			continue
		}
		_ = s.saveRouteDecisionSnapshot(ctx, routeID, decision)
	}

	return len(exactModels), len(wildcardRouteIDs), nil
}

func (s *RouteDecisionService) saveRouteDecisionSnapshot(ctx context.Context, routeID int64, decision RouteDecisionExplanation) error {
	json, err := marshalDecision(decision)
	if err != nil {
		return err
	}
	refreshedAt := time.Now().UTC().Format(time.RFC3339)
	return s.db.UpdateRouteDecisionSnapshot(ctx, routeID, json, refreshedAt)
}

// marshalDecision JSON-encodes a RouteDecisionExplanation.
func marshalDecision(d RouteDecisionExplanation) (string, error) {
	// Keep the historical hand-built shape: summary/candidates are always
	// arrays (never null) even when empty.
	if d.Summary == nil {
		d.Summary = []string{}
	}
	if d.Candidates == nil {
		d.Candidates = []RouteDecisionCandidate{}
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
