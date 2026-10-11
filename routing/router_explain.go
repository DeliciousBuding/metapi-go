package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Selection explanation ----

// ExplainSelection returns a decision explanation for the requested model.
func (tr *TokenRouter) ExplainSelection(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy DownstreamRoutingPolicy) (decision RouteDecisionExplanation, err error) {
	defer func() { setExplanationContext(&decision, policy) }()
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return RouteDecisionExplanation{}, err
	}
	if !IsModelAllowedByDownstreamPolicy(requestedModel, policy) {
		return RouteDecisionExplanation{RequestedModel: requestedModel, Matched: false}, nil
	}
	requestedModel = policy.AccessPolicy.MapModel(requestedModel)
	match, err := tr.selector.findRoute(ctx, requestedModel, policy)
	return explainSelectionFromMatch(tr.selector, match, requestedModel, excludeChannelIDs, policy, tr.configuredMaxSec), err
}

// ExplainSelectionForRoute returns a decision explanation for a specific route.
func (tr *TokenRouter) ExplainSelectionForRoute(ctx context.Context, routeID int64, requestedModel string, excludeChannelIDs []int64, policy DownstreamRoutingPolicy) (decision RouteDecisionExplanation, err error) {
	defer func() { setExplanationContext(&decision, policy) }()
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
func (tr *TokenRouter) ExplainSelectionRouteWide(ctx context.Context, routeID int64, policy DownstreamRoutingPolicy) (decision RouteDecisionExplanation, err error) {
	defer func() {
		setExplanationContext(&decision, policy)
		decision.NotEvaluated = append(decision.NotEvaluated, "concreteModel")
		for i := range decision.Candidates {
			candidate := &decision.Candidates[i]
			filtered := make([]CandidateBlocker, 0, len(candidate.Blockers))
			for _, blocker := range candidate.Blockers {
				if blocker.Code != "source_model_mismatch" {
					filtered = append(filtered, blocker)
				}
			}
			candidate.Blockers = filtered
			candidate.Eligible = false
			candidate.Eligibility = "notEvaluated"
			candidate.Reason = "具体请求模型未指定，候选资格未完整评估"
			if len(filtered) > 0 {
				candidate.Eligibility = "blocked"
				reasons := make([]string, 0, len(filtered))
				for _, blocker := range filtered {
					reasons = append(reasons, blocker.Message)
				}
				candidate.Reason = formatReasons(reasons)
			}
			if candidate.CandidateRef.Kind != "direct" {
				candidate.ActualModel = ""
			}
		}
	}()
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

func setExplanationContext(decision *RouteDecisionExplanation, policy DownstreamRoutingPolicy) {
	decision.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	decision.RequiredUpstreamProtocol = policy.RequiredUpstreamProtocol
	decision.AllowUpstreamProtocolConversion = policy.AllowUpstreamProtocolConversion
	// This preview evaluates candidate filters, not admission, request-size
	// limits, strategy selection/leases, or the upstream's actual response.
	decision.NotEvaluated = []string{"downstreamAdmission", "requestContextLength", "strategySelection", "upstreamAvailability"}
	if policy.RequiredUpstreamProtocol == 0 {
		decision.NotEvaluated = append(decision.NotEvaluated, "protocol")
	}
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
		// The execution owner produces codes and legacy messages together.
		// Missing protocol remains fail-closed in execution, but is an unknown
		// dimension in a preview, not proof that a grant is unauthorized.
		blockers := selector.getCandidateBlockers(row, requestedModel, bypassSourceModelCheck, excludeChannelIDs, nowISO, policy)
		reasons := make([]string, 0, len(blockers))
		filtered := make([]CandidateBlocker, 0, len(blockers))
		for _, blocker := range blockers {
			if blocker.Code != "protocol_context_missing" {
				filtered = append(filtered, blocker)
				reasons = append(reasons, blocker.Message)
			}
		}
		eligibility := "eligible"
		if policy.RequiredUpstreamProtocol == 0 {
			eligibility = "notEvaluated"
		}
		if len(filtered) > 0 {
			eligibility = "blocked"
		}
		eligible := eligibility == "eligible"
		recentlyFailed := false
		if strategy != StrategyRoundRobin {
			recentlyFailed = IsChannelRecentlyFailed(row.Channel.FailCount, row.Channel.LastFailAt, nowMs, configuredMaxSec)
		}
		candidate := RouteDecisionCandidate{
			Eligibility:            eligibility,
			Blockers:               filtered,
			ChannelID:              row.Channel.ID,
			AccountID:              row.Account.ID,
			Username:               formatUsername(row.Account.Username, row.Account.ID),
			SiteName:               formatSiteName(row.Site.Name),
			TokenName:              getTokenName(row.Token),
			Priority:               row.Channel.PriorityOrZero(),
			Weight:                 row.Channel.WeightOrZero(),
			Eligible:               eligible,
			RecentlyFailed:         recentlyFailed,
			AvoidedByRecentFailure: false,
			Probability:            0,
			Reason:                 formatReasons(reasons),
		}
		if eligibility == "notEvaluated" {
			candidate.Reason = "请求协议未指定，候选资格未完整评估"
		}
		setCandidateIdentity(&candidate, row, requestedModel, match.Route.DisplayName, mappedModel)
		candidates = append(candidates, candidate)
		if eligible {
			available = append(available, row)
		}
	}

	if len(available) == 0 {
		if policy.RequiredUpstreamProtocol == 0 {
			summary = append(summary, "缺少请求协议，候选资格未完整评估")
		} else {
			summary = append(summary, "候选过滤检查未发现可用通道")
		}
		return RouteDecisionExplanation{
			RequestedModel:  requestedModel,
			ActualModel:     mappedModel,
			Matched:         true,
			RouteID:         &match.Route.ID,
			ModelPattern:    match.Route.ModelPattern,
			Summary:         summary,
			Candidates:      candidates,
			RoutingStrategy: strategy,
		}
	}

	// Build a decision based on strategy
	explanation := RouteDecisionExplanation{
		RequestedModel:  requestedModel,
		ActualModel:     mappedModel,
		Matched:         true,
		RouteID:         &match.Route.ID,
		ModelPattern:    match.Route.ModelPattern,
		Summary:         summary,
		Candidates:      candidates,
		RoutingStrategy: strategy,
	}

	return explanation
}

func setCandidateIdentity(candidate *RouteDecisionCandidate, row RouteChannelCandidate, requestedModel string, displayName *string, mappedModel string) {
	if direct := row.Direct; direct != nil {
		candidate.CandidateRef = RouteDecisionCandidateRef{Kind: "direct", ID: direct.ItemID, GroupID: direct.GroupID,
			ChannelID: direct.ChannelID, GrantID: direct.GrantID, CredentialID: direct.CredentialID}
		candidate.Label = direct.ChannelName + " · " + direct.CredentialName
		candidate.Username = ""
		candidate.SiteName = direct.ChannelName
		candidate.TokenName = direct.CredentialName
		candidate.SourceModel = direct.ModelName
		candidate.ActualModel = direct.ModelName
		return
	}
	if IsOAuthRouteUnitCandidate(row) {
		unitID := int64(0)
		if row.RouteUnit != nil {
			unitID = row.RouteUnit.ID
		} else if row.Channel.OAuthRouteUnitID != nil {
			unitID = *row.Channel.OAuthRouteUnitID
		}
		candidate.CandidateRef = RouteDecisionCandidateRef{Kind: "oauth_unit", ID: unitID, ChannelID: row.Channel.ID}
		candidate.Label = "OAuth"
		if row.RouteUnit != nil {
			candidate.Label = row.RouteUnit.Name
		}
		candidate.Username, candidate.SiteName, candidate.TokenName = "", "", ""
		// Member selection is deliberately not performed, so its model is unknown.
		return
	}
	candidate.CandidateRef = RouteDecisionCandidateRef{Kind: "account", ID: row.Channel.ID, AccountID: row.Account.ID,
		SiteID: row.Site.ID, TokenID: row.Channel.TokenID}
	candidate.Label = candidate.SiteName + " · " + candidate.Username
	if row.Channel.SourceModel != nil {
		candidate.SourceModel = *row.Channel.SourceModel
	}
	candidate.ActualModel = ResolveActualModelForSelectedChannel(requestedModel, displayName, mappedModel, row.Channel.SourceModel, row.Channel.AccountID)
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
