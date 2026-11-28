package backup

import (
	"sort"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

type magpiePlanKey struct {
	SourceID int64
	Name     string
	Secret   string
	Enabled  bool
	Policy   store.DownstreamAccessPolicy
}

func compileMagpieGatewayKeys(src *MagpieSource, plan *magpiePlan) error {
	routePatterns := map[string]bool{}
	for _, route := range plan.routes {
		routePatterns[route.Pattern] = true
	}
	for _, key := range src.GatewayKeys {
		if key.Secret == "" {
			plan.notImported["gatewayKeysWithoutSecret"]++
			continue
		}
		name := key.Name
		if name == "" {
			name = "Magpie"
		}
		compiled := magpiePlanKey{
			SourceID: sourceTupleID("magpie-gateway-key", key.ID),
			Name:     name, Secret: key.Secret, Enabled: !key.Off,
		}
		if len(key.Accounts) > 0 {
			compiled.Policy.BlockReason = "source_key_account_scope_unsupported"
		}
		if len(key.UnsupportedFields) > 0 {
			plan.residuals = append(plan.residuals, "gateway_key_fields_not_imported")
		}
		if key.LAN {
			plan.residuals = append(plan.residuals, "gateway_key_lan_role_not_imported")
		}
		if len(key.Models) > 0 {
			models, unresolved := expandMagpieKeyModels(key.Models, routePatterns)
			if unresolved > 0 {
				compiled.Policy.BlockReason = "source_key_model_scope_unresolved"
				plan.residuals = append(plan.residuals, "gateway_key_model_scope_unresolved")
			} else {
				compiled.Policy.ModelIDs = models
			}
		}
		if key.Limit != nil {
			quota, residuals := magpieQuota(key.Limit, src.Created)
			compiled.Policy.Quota = quota
			plan.residuals = append(plan.residuals, residuals...)
			if quota == nil {
				compiled.Policy.BlockReason = "source_key_quota_unsupported"
			}
		}
		if err := compiled.Policy.Validate(); err != nil {
			compiled.Policy.ModelIDs = nil
			compiled.Policy.Quota = nil
			compiled.Policy.BlockReason = "source_key_policy_unsupported"
			plan.residuals = append(plan.residuals, "gateway_key_policy_blocked")
		}
		if compiled.Policy.BlockReason != "" {
			plan.residuals = append(plan.residuals, compiled.Policy.BlockReason)
		}
		plan.keys = append(plan.keys, compiled)
	}
	return nil
}

func expandMagpieKeyModels(patterns []string, routes map[string]bool) ([]string, int) {
	seen := map[string]bool{}
	var out []string
	unresolved := 0
	for _, pattern := range patterns {
		matches := 0
		for route := range routes {
			if route == pattern || magpieGlobMatch(pattern, route) {
				if !seen[route] {
					seen[route] = true
					out = append(out, route)
				}
				matches++
			}
		}
		if matches == 0 {
			unresolved++
		}
	}
	sort.Strings(out)
	return out, unresolved
}

func magpieQuota(limit *MagpieSourceLimit, created time.Time) (*store.DownstreamQuota, []string) {
	if limit == nil || (limit.Tokens <= 0 && limit.Cost <= 0) {
		return nil, nil
	}
	q := &store.DownstreamQuota{Period: store.DownstreamQuotaPeriod{Type: "calendar_duration"}}
	if limit.Tokens > 0 {
		v := limit.Tokens
		q.TotalTokens = &v
	}
	if limit.Cost > 0 {
		v := limit.Cost
		q.Cost = &v
	}
	var residuals []string
	switch limit.Period {
	case "", "day":
		q.Period.CalendarDuration = &store.DownstreamQuotaDuration{Value: 1, Unit: "day"}
	case "month":
		q.Period.CalendarDuration = &store.DownstreamQuotaDuration{Value: 1, Unit: "month"}
	case "week":
		q.Period = store.DownstreamQuotaPeriod{Type: "past_duration", PastDuration: &store.DownstreamQuotaDuration{Value: 7, Unit: "day"}}
		residuals = append(residuals, "gateway_key_week_quota_approximated_as_rolling_7_days")
	case "days":
		if limit.Days <= 0 {
			residuals = append(residuals, "gateway_key_quota_period_unsupported")
			return nil, residuals
		}
		q.Period = store.DownstreamQuotaPeriod{Type: "past_duration", PastDuration: &store.DownstreamQuotaDuration{Value: int64(limit.Days), Unit: "day"}}
		residuals = append(residuals, "gateway_key_cycle_quota_approximated_as_rolling")
	default:
		residuals = append(residuals, "gateway_key_quota_period_unsupported")
		return nil, residuals
	}
	if limit.CacheReads {
		residuals = append(residuals, "gateway_key_cache_read_accounting_not_imported")
	}
	if limit.HasSince {
		residuals = append(residuals, "gateway_key_quota_cycle_anchor_not_imported")
	}
	if !created.IsZero() {
		ms := created.UnixMilli()
		q.HistoryMissingBefore = &ms
		residuals = append(residuals, "gateway_key_quota_history_not_imported")
	}
	return q, residuals
}
