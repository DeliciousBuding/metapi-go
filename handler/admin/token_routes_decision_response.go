package admin

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
)

const (
	routeDecisionBatchMaxItems     = 500
	routeDecisionRefreshTaskType   = "route-decision.refresh"
	routeDecisionRefreshTaskTitle  = "Refresh route selection probabilities"
	routeDecisionRefreshDedupeKey  = "route-decision-refresh"
	routeDecisionRouterUnavailable = "route decision engine is not configured"
)

func routeDecisionToMap(d routing.RouteDecisionExplanation) map[string]any {
	candidates := make([]map[string]any, 0, len(d.Candidates))
	for _, c := range d.Candidates {
		candidates = append(candidates, map[string]any{
			"candidateRef":           c.CandidateRef,
			"label":                  c.Label,
			"sourceModel":            c.SourceModel,
			"actualModel":            c.ActualModel,
			"eligibility":            c.Eligibility,
			"blockers":               c.Blockers,
			"probabilityEvaluated":   c.ProbabilityEvaluated,
			"channelId":              c.ChannelID,
			"accountId":              c.AccountID,
			"username":               c.Username,
			"siteName":               c.SiteName,
			"tokenName":              c.TokenName,
			"priority":               c.Priority,
			"weight":                 c.Weight,
			"eligible":               c.Eligible,
			"recentlyFailed":         c.RecentlyFailed,
			"avoidedByRecentFailure": c.AvoidedByRecentFailure,
			"probability":            c.Probability,
			"reason":                 c.Reason,
			"cooldownUntil":          c.CooldownUntil,
			"oauthMembers":           c.OAuthMembers,
		})
	}
	out := map[string]any{
		"mode":                            "adminPreview",
		"checkedAt":                       d.CheckedAt,
		"requiredUpstreamProtocol":        d.RequiredUpstreamProtocol,
		"allowUpstreamProtocolConversion": d.AllowUpstreamProtocolConversion,
		"notEvaluated":                    d.NotEvaluated,
		"routingStrategy":                 d.RoutingStrategy,
		"requestedModel":                  d.RequestedModel,
		"actualModel":                     d.ActualModel,
		"matched":                         d.Matched,
		"modelPattern":                    d.ModelPattern,
		"selectedLabel":                   d.SelectedLabel,
		"summary":                         d.Summary,
		"candidates":                      candidates,
	}
	if d.RouteID != nil {
		out["routeId"] = *d.RouteID
	}
	if d.SelectedChannelID != nil {
		out["selectedChannelId"] = *d.SelectedChannelID
	}
	if d.SelectedAccountID != nil {
		out["selectedAccountId"] = *d.SelectedAccountID
	}
	if d.Summary == nil {
		out["summary"] = []string{}
	}
	return out
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func uniquePositiveInt64(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
