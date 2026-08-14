package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Selection explanation ----

// ExplainSelection returns a decision explanation for the requested model.
func (tr *TokenRouter) ExplainSelection(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy DownstreamRoutingPolicy) (RouteDecisionExplanation, error) {
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return RouteDecisionExplanation{}, err
	}
	match, err := tr.selector.findRoute(ctx, requestedModel, policy)
	return explainSelectionFromMatch(tr.selector, match, requestedModel, excludeChannelIDs, policy, tr.configuredMaxSec), err
}

// ExplainSelectionForRoute returns a decision explanation for a specific route.
func (tr *TokenRouter) ExplainSelectionForRoute(ctx context.Context, routeID int64, requestedModel string, excludeChannelIDs []int64, policy DownstreamRoutingPolicy) (RouteDecisionExplanation, error) {
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return RouteDecisionExplanation{}, err
	}

	match, err := tr.findRouteByID(ctx, routeID, policy)
	if err != nil || match == nil {
		if err != nil {
			return RouteDecisionExplanation{}, err
		}
		return RouteDecisionExplanation{RequestedModel: requestedModel, ActualModel: requestedModel, Matched: false, Summary: []string{"未匹配到路由"}}, nil
	}
	return explainSelectionFromMatch(tr.selector, match, requestedModel, excludeChannelIDs, policy, tr.configuredMaxSec), nil
}

// ExplainSelectionRouteWide returns a decision explanation for a route-wide view.
func (tr *TokenRouter) ExplainSelectionRouteWide(ctx context.Context, routeID int64, policy DownstreamRoutingPolicy) (RouteDecisionExplanation, error) {
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return RouteDecisionExplanation{}, err
	}

	match, err := tr.findRouteByID(ctx, routeID, policy)
	if err != nil || match == nil {
		if err != nil {
			return RouteDecisionExplanation{}, err
		}
		return RouteDecisionExplanation{RequestedModel: fmt.Sprintf("route:%d", routeID), ActualModel: fmt.Sprintf("route:%d", routeID), Matched: false, Summary: []string{"未匹配到路由"}}, nil
	}

	fallbackRequestedModel := match.Route.ModelPattern
	if fallbackRequestedModel == "" {
		fallbackRequestedModel = fmt.Sprintf("route:%d", routeID)
	}
	return explainSelectionFromMatch(tr.selector, match, fallbackRequestedModel, nil, policy, tr.configuredMaxSec), nil
}

func (tr *TokenRouter) findRouteByID(ctx context.Context, routeID int64, policy DownstreamRoutingPolicy) (*RouteMatch, error) {
	if len(policy.AllowedRouteIDs) > 0 {
		found := false
		for _, id := range policy.AllowedRouteIDs {
			if id == routeID {
				found = true
				break
			}
		}
		if !found {
			return nil, nil
		}
	}

	routes, err := tr.db.LoadEnabledRoutes(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range routes {
		if r.ID == routeID {
			return tr.selector.loadRouteMatch(ctx, r)
		}
	}
	return nil, nil
}

func explainSelectionFromMatch(selector *ChannelSelector, match *RouteMatch, requestedModel string, excludeChannelIDs []int64, policy DownstreamRoutingPolicy, configuredMaxSec int) RouteDecisionExplanation {
	if match == nil {
		return RouteDecisionExplanation{
			RequestedModel: requestedModel,
			ActualModel:    requestedModel,
			Matched:        false,
			Summary:        []string{"未匹配到启用的路由"},
			Candidates:     nil,
		}
	}

	requestedByDisplayName := IsRouteDisplayNameMatch(requestedModel, match.Route.DisplayName)
	bypassSourceModelCheck := requestedByDisplayName
	mappedModel := ResolveMappedModel(requestedModel, match.Route.ModelMapping)
	strategy := NormalizeRouteRoutingStrategy(match.Route.RoutingStrategy)

	nowISO := time.Now().UTC().Format(time.RFC3339)
	nowMs := time.Now().UnixMilli()
	summary := []string{
		"命中路由：" + match.Route.ModelPattern,
	}
	switch strategy {
	case StrategyRoundRobin:
		summary = append(summary, "路由策略：轮询")
	case StrategyStableFirst:
		summary = append(summary, "路由策略：稳定优先")
	case StrategyLeastBusy:
		summary = append(summary, "路由策略：最少繁忙")
	case StrategyLowestLatency:
		summary = append(summary, "路由策略：最低延迟")
	case StrategyLowestCost:
		summary = append(summary, "路由策略：最低成本")
	default:
		summary = append(summary, "路由策略：按权重随机")
	}
	if requestedByDisplayName {
		summary = append(summary, "按显示名命中："+NormalizeRouteDisplayName(match.Route.DisplayName))
		summary = append(summary, "显示名仅用于聚合展示，实际转发模型按选中通道来源模型决定")
	}

	var candidates []RouteDecisionCandidate
	var available []RouteChannelCandidate

	for _, row := range match.Channels {
		// Reuse the selector's single source of truth for eligibility so admin
		// explanations include OAuth-member, token-availability, and downstream
		// policy checks. selector is always non-nil (all callers route through
		// tr.selector.findRoute first); the zero DownstreamRoutingPolicy used by
		// admin flows is safe: every check is len()-guarded.
		reasons := selector.getCandidateEligibilityReasons(row, requestedModel, bypassSourceModelCheck, excludeChannelIDs, nowISO, policy)
		eligible := len(reasons) == 0
		recentlyFailed := false
		if strategy != StrategyRoundRobin {
			recentlyFailed = IsChannelRecentlyFailed(&row.Channel.FailCount, row.Channel.LastFailAt, nowMs, configuredMaxSec)
		}
		candidate := RouteDecisionCandidate{
			ChannelID:              row.Channel.ID,
			AccountID:              row.Account.ID,
			Username:               formatUsername(row.Account.Username, row.Account.ID),
			SiteName:               formatSiteName(row.Site.Name),
			TokenName:              getTokenName(row.Token),
			Priority:               row.Channel.Priority,
			Weight:                 row.Channel.Weight,
			Eligible:               eligible,
			RecentlyFailed:         recentlyFailed,
			AvoidedByRecentFailure: false,
			Probability:            0,
			Reason:                 formatReasons(reasons),
		}
		candidates = append(candidates, candidate)
		if eligible {
			available = append(available, row)
		}
	}

	if len(available) == 0 {
		summary = append(summary, "没有可用通道（全部被禁用、站点不可用、冷却或令牌不可用）")
		return RouteDecisionExplanation{
			RequestedModel: requestedModel,
			ActualModel:    mappedModel,
			Matched:        true,
			RouteID:        &match.Route.ID,
			ModelPattern:   match.Route.ModelPattern,
			Summary:        summary,
			Candidates:     candidates,
		}
	}

	// Build a decision based on strategy
	explanation := RouteDecisionExplanation{
		RequestedModel: requestedModel,
		ActualModel:    mappedModel,
		Matched:        true,
		RouteID:        &match.Route.ID,
		ModelPattern:   match.Route.ModelPattern,
		Summary:        summary,
		Candidates:     candidates,
	}

	return explanation
}

func formatUsername(username *string, accountID int64) string {
	if username != nil && *username != "" {
		return *username
	}
	return fmt.Sprintf("account-%d", accountID)
}

func formatSiteName(name string) string {
	if name == "" {
		return "unknown"
	}
	return name
}

func getTokenName(token *store.AccountToken) string {
	if token == nil {
		return "default"
	}
	return token.Name
}

func formatReasons(reasons []string) string {
	if len(reasons) == 0 {
		return "可用"
	}
	result := ""
	for i, r := range reasons {
		if i > 0 {
			result += "、"
		}
		result += r
	}
	return result
}
