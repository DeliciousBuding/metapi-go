package routing

import (
	"github.com/deliciousbuding/metapi-go/store"
)

// resolveAssociationCandidate works on a copy: cached membership and original
// item priority remain shared, whereas the first matching association depends
// on this request. Imported rules are stable-sorted by source priority.
func resolveAssociationCandidate(candidate RouteChannelCandidate, policy DownstreamRoutingPolicy) (RouteChannelCandidate, CandidateBlocker) {
	direct := candidate.Direct
	if direct == nil || len(direct.AssociationConditions) == 0 {
		return candidate, CandidateBlocker{}
	}
	if direct.AssociationConditions.Validate() != nil {
		return candidate, CandidateBlocker{"association_condition_invalid", "imported association condition is invalid"}
	}
	input := policy.AssociationRequest
	if input == nil && policy.ResolveAssociationRequest != nil {
		input = policy.ResolveAssociationRequest()
	}
	var selected *store.DirectAssociationRule
	var selectedIndex int
	missing := false
	for i := range direct.AssociationConditions {
		rule := &direct.AssociationConditions[i]
		match, evaluated := rule.Condition.Match(input)
		if !evaluated {
			missing = true
			continue
		}
		if match && (selected == nil || rule.Priority < selected.Priority) {
			selected, selectedIndex = rule, i
		}
	}
	if missing {
		// An unknown higher-priority association may alter both priority and
		// protocol order. Do not silently choose a lower unconditional rule.
		for i, rule := range direct.AssociationConditions {
			_, evaluated := rule.Condition.Match(input)
			if !evaluated && (selected == nil || rule.Priority < selected.Priority || rule.Priority == selected.Priority && i < selectedIndex) {
				return candidate, CandidateBlocker{"association_context_missing", "request context is required to evaluate imported association conditions"}
			}
		}
	}
	if selected == nil {
		return candidate, CandidateBlocker{"association_condition_mismatch", "request does not match an imported association condition"}
	}
	copyDirect := *direct
	copyDirect.ProtocolOrder = append(store.DirectProtocolOrder(nil), selected.ProtocolOrder...)
	if len(direct.ProtocolOrder) > 0 {
		permitted := make(map[int]bool, len(direct.ProtocolOrder))
		for _, bit := range direct.ProtocolOrder {
			permitted[bit] = true
		}
		copyDirect.ProtocolOrder = nil
		for _, bit := range selected.ProtocolOrder {
			if permitted[bit] {
				copyDirect.ProtocolOrder = append(copyDirect.ProtocolOrder, bit)
			}
		}
		if len(copyDirect.ProtocolOrder) == 0 {
			return candidate, CandidateBlocker{"protocol_not_authorized", "matching association has no authorized protocol"}
		}
	}
	candidate.Direct = &copyDirect
	candidate.Channel.Direct = &copyDirect
	priority := selected.Priority
	candidate.Channel.Priority = &priority
	return candidate, CandidateBlocker{}
}
