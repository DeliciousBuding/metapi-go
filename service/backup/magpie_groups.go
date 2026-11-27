package backup

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type magpiePlanGroup struct {
	SourceID     int64
	Name         string
	Mode         string
	Enabled      bool
	ActiveItemID int64
	Items        []magpiePlanGroupItem
}

type magpiePlanGroupItem struct {
	SourceID    int64
	GrantSource int64
	Priority    int
	Weight      int
}

type MagpieSkippedGroup struct {
	SourceID string   `json:"sourceId"`
	Reasons  []string `json:"reasons"`
}

func compileMagpieGroups(src *MagpieSource, plan *magpiePlan, aliases map[string]string, models map[string]map[string]bool, grants map[magpieGrantRef]magpiePlanGrant, usedNames map[string]int) []string {
	var residuals []string
	groups := make(map[string]MagpieSourceGroup, len(src.Groups))
	for _, group := range src.Groups {
		groups[group.ID] = group
	}
	for _, group := range src.Groups {
		if group.Hidden {
			plan.notImported["hiddenGroups"]++
			continue
		}
		if !magpieIdentifier(group.ID) {
			plan.skippedGroups = append(plan.skippedGroups, MagpieSkippedGroup{SourceID: sanitizeIdentifier(group.ID), Reasons: []string{"group_id_invalid"}})
			continue
		}
		switch group.Routing {
		case "", "manual", "order", "rotate", "usage", "pace", "weight":
		default:
			plan.skippedGroups = append(plan.skippedGroups, MagpieSkippedGroup{SourceID: sanitizeIdentifier(group.ID), Reasons: []string{"group_routing_unsupported"}})
			continue
		}
		members, memberResiduals := magpieGroupMembers(group, groups, aliases, models, map[string]bool{group.ID: true})
		residuals = append(residuals, memberResiduals...)
		if len(group.Matched) > 0 {
			residuals = append(residuals, "group_matched_snapshot_ignored")
		}
		if len(group.Match) > 0 {
			residuals = append(residuals, "group_match_expanded_to_snapshot")
		}
		if group.HasRules || group.Classifier != "" || group.Effort != "" {
			residuals = append(residuals, "group_rules_and_classifier_not_imported")
		}
		if affinity := strings.ToLower(strings.TrimSpace(group.Affinity)); affinity != "off" {
			residuals = append(residuals, "group_affinity_not_imported")
		}
		if group.Sink {
			residuals = append(residuals, "group_rate_limit_sink_not_imported")
		}
		if group.FirstToken != 0 || group.Patience != 0 {
			residuals = append(residuals, "group_timeouts_not_imported")
		}
		if len(group.Levels) > 0 || len(group.Fast) > 0 {
			residuals = append(residuals, "group_reasoning_controls_not_imported")
		}
		if group.Family != "" {
			residuals = append(residuals, "group_family_not_imported")
		}
		if group.Auto {
			residuals = append(residuals, "group_auto_flag_not_imported")
		}
		if len(group.UnsupportedFields) > 0 {
			residuals = append(residuals, "group_fields_not_imported")
		}
		if group.Context == -1 {
			residuals = append(residuals, "group_smallest_context_not_imported")
		}
		if group.Pick != "" && group.Routing != "manual" {
			residuals = append(residuals, "group_manual_pick_not_imported")
		}
		if len(group.Off) > 0 {
			residuals = append(residuals, "disabled_group_members_omitted")
		}
		if len(members) == 0 {
			plan.skippedGroups = append(plan.skippedGroups, MagpieSkippedGroup{SourceID: sanitizeIdentifier(group.ID), Reasons: []string{"group_has_no_servable_members"}})
			continue
		}
		pattern := "group/" + group.ID
		groupPlan := magpiePlanGroup{
			SourceID: sourceTupleID("magpie-group", group.ID),
			Name:     uniqueMagpieName(group.Name, group.ID, usedNames), Mode: "failover",
			Enabled: !group.Disabled,
		}
		strategy := "weighted"
		switch group.Routing {
		case "rotate":
			strategy = "round_robin"
		case "manual":
			groupPlan.Mode = "manual"
		case "":
			residuals = append(residuals, "group_routing_smart_approximated_as_weighted")
		case "usage":
			residuals = append(residuals, "group_routing_usage_approximated_as_weighted")
		case "pace":
			residuals = append(residuals, "group_routing_pace_approximated_as_weighted")
		}
		seenGrants := map[int64]bool{}
		pickProvider, pickModel := "", ""
		if group.Pick != "" {
			if providerID, model, _, ok := resolveMagpieMember(group.Pick, aliases, models); ok {
				pickProvider, pickModel = providerID, model
			}
		}
		for memberIndex, member := range members {
			ref := magpieGrantRef{member.ProviderID, member.Model, ""}
			provider := magpieProviderByID(plan, member.ProviderID)
			if provider == nil {
				continue
			}
			for _, credential := range provider.Credentials {
				grant, ok := grants[magpieGrantRef{ref.ProviderID, ref.Model, credential.Name}]
				if !ok || seenGrants[grant.SourceID] {
					continue
				}
				seenGrants[grant.SourceID] = true
				priority := 0
				if group.Routing == "order" {
					priority = memberIndex
				}
				item := magpiePlanGroupItem{
					SourceID:    sourceTupleID("magpie-group-item", group.ID, fmt.Sprint(memberIndex), credential.Name),
					GrantSource: grant.SourceID, Priority: priority, Weight: max(1, credential.Weight),
				}
				groupPlan.Items = append(groupPlan.Items, item)
				if group.Routing == "manual" && member.ProviderID == pickProvider && member.Model == pickModel && groupPlan.ActiveItemID == 0 {
					groupPlan.ActiveItemID = item.SourceID
				}
			}
		}
		if len(groupPlan.Items) == 0 {
			plan.skippedGroups = append(plan.skippedGroups, MagpieSkippedGroup{SourceID: sanitizeIdentifier(group.ID), Reasons: []string{"group_has_no_importable_grants"}})
			continue
		}
		if groupPlan.Mode == "manual" && groupPlan.ActiveItemID == 0 {
			groupPlan.ActiveItemID = groupPlan.Items[0].SourceID
			residuals = append(residuals, "manual_group_pick_defaulted_to_first_member")
		}
		var context *int
		if group.Context > 0 {
			value := group.Context
			context = &value
		}
		plan.routes = append(plan.routes, magpiePlanRoute{
			SourceID: sourceTupleID("magpie-route", pattern), Pattern: pattern,
			DisplayName: groupPlan.Name, Strategy: strategy, Enabled: groupPlan.Enabled,
			Context: context, Group: groupPlan,
		})
	}
	return residuals
}

type magpieResolvedMember struct {
	ProviderID string
	Model      string
	Ref        string
}

func magpieGroupMembers(group MagpieSourceGroup, groups map[string]MagpieSourceGroup, aliases map[string]string, models map[string]map[string]bool, visiting map[string]bool) ([]magpieResolvedMember, []string) {
	var out []magpieResolvedMember
	var residuals []string
	seen := map[string]bool{}
	off := map[string]bool{}
	for _, member := range group.Off {
		if providerID, model, _, ok := resolveMagpieMember(member, aliases, models); ok {
			off[providerID+"/"+model] = true
		}
	}
	var add func(ref string) bool = func(ref string) bool {
		ref = strings.TrimSpace(ref)
		if groupID, ok := strings.CutPrefix(ref, "group/"); ok {
			nested, exists := groups[groupID]
			if !exists || visiting[groupID] || nested.Hidden || nested.Disabled {
				residuals = append(residuals, "nested_group_not_imported")
				return false
			}
			visiting[groupID] = true
			nestedMembers, nestedResiduals := magpieGroupMembers(nested, groups, aliases, models, visiting)
			delete(visiting, groupID)
			residuals = append(residuals, nestedResiduals...)
			residuals = append(residuals, "nested_group_flattened")
			for _, member := range nestedMembers {
				if !seen[member.Ref] {
					seen[member.Ref] = true
					out = append(out, member)
				}
			}
			return len(nestedMembers) > 0
		}
		providerID, model, effort, ok := resolveMagpieMember(ref, aliases, models)
		if !ok {
			residuals = append(residuals, "group_member_not_servable")
			return false
		}
		if effort != "" {
			residuals = append(residuals, "group_member_effort_not_imported")
			ref = providerID + "/" + model
		}
		if off[providerID+"/"+model] {
			residuals = append(residuals, "disabled_group_members_omitted")
			return false
		}
		if seen[ref] {
			residuals = append(residuals, "duplicate_group_member_ignored")
			return false
		}
		seen[ref] = true
		out = append(out, magpieResolvedMember{ProviderID: providerID, Model: model, Ref: ref})
		return true
	}
	for _, member := range group.Members {
		add(member)
	}
	if len(group.Match) > 0 {
		var patterns []*regexp.Regexp
		for _, pattern := range group.Match {
			if re, err := compileMagpiePattern(pattern); err == nil {
				patterns = append(patterns, re)
			} else {
				residuals = append(residuals, "group_match_pattern_invalid")
			}
		}
		for _, providerID := range sortedStringKeys(models) {
			for _, model := range sortedStringKeys(models[providerID]) {
				id := providerID + "/" + model
				for _, pattern := range patterns {
					if pattern.MatchString(id) {
						add(id)
						break
					}
				}
			}
		}
	}
	return out, residuals
}

func resolveMagpieMember(ref string, aliases map[string]string, models map[string]map[string]bool) (string, string, string, bool) {
	ref = strings.TrimSpace(ref)
	if _, _, ok := strings.Cut(ref, "/"); !ok {
		return "", "", "", false
	}
	base, effort := trimMagpieEffort(ref)
	for _, candidate := range []string{ref, base} {
		providerID, model, ok := strings.Cut(candidate, "/")
		if !ok {
			continue
		}
		if mapped := aliases[strings.ToLower(providerID)]; mapped != "" {
			providerID = mapped
		}
		if models[providerID][model] {
			if candidate == ref && effort != "" {
				effort = ""
			}
			return providerID, model, effort, true
		}
	}
	return "", "", "", false
}

var magpieEfforts = map[string]bool{"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}

func trimMagpieEffort(ref string) (string, string) {
	index := strings.LastIndex(ref, ":")
	if index <= 0 {
		return ref, ""
	}
	effort := strings.ToLower(ref[index+1:])
	if !magpieEfforts[effort] {
		return ref, ""
	}
	return ref[:index], effort
}

func compileMagpiePattern(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(pattern)
	if re, ok := strings.CutPrefix(pattern, "re:"); ok {
		if strings.TrimSpace(re) == "" {
			return nil, errors.New("empty regex")
		}
		return regexp.Compile("^(?:" + re + ")$")
	}
	parts := strings.Split(pattern, "*")
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	return regexp.Compile("(?i)^" + strings.Join(parts, ".*") + "$")
}

func magpieGlobMatch(pattern, value string) bool {
	re, err := compileMagpiePattern(pattern)
	return err == nil && re.MatchString(value)
}

func magpieProviderAliases(src *MagpieSource) map[string]string {
	aliases := map[string]string{}
	for _, provider := range src.Providers {
		aliases[provider.ID] = provider.ID
	}
	for _, provider := range src.Providers {
		for _, alias := range provider.Was {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias != "" {
				aliases[alias] = provider.ID
			}
		}
	}
	return aliases
}

// magpieGroupReferencedModels collects every "provider/model" a routing group
// names explicitly. Magpie serves those models even when the provider left
// them out of its exposed Models list (an unlisted provider serves only
// through its groups), so dropping them would silently empty the group.
func magpieGroupReferencedModels(src *MagpieSource, aliases map[string]string) map[string][]string {
	known := make(map[string]string, len(src.Providers))
	for _, provider := range src.Providers {
		known[provider.ID] = provider.ID
		lower := strings.ToLower(provider.ID)
		if _, exists := known[lower]; !exists {
			known[lower] = provider.ID
		}
	}
	referenced := map[string]map[string]bool{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || strings.HasPrefix(ref, "group/") {
			return
		}
		base, _ := trimMagpieEffort(ref)
		providerID, model, ok := strings.Cut(base, "/")
		if !ok {
			return
		}
		providerID, model = strings.TrimSpace(providerID), strings.TrimSpace(model)
		if providerID == "" || model == "" {
			return
		}
		if mapped := aliases[strings.ToLower(providerID)]; mapped != "" {
			providerID = mapped
		}
		canonical, exists := known[providerID]
		if !exists {
			canonical, exists = known[strings.ToLower(providerID)]
		}
		if !exists {
			return
		}
		if referenced[canonical] == nil {
			referenced[canonical] = map[string]bool{}
		}
		referenced[canonical][model] = true
	}
	for _, group := range src.Groups {
		if group.Hidden {
			continue
		}
		for _, member := range group.Members {
			add(member)
		}
		for _, member := range group.Off {
			add(member)
		}
		add(group.Pick)
	}
	models := make(map[string][]string, len(referenced))
	for providerID, set := range referenced {
		models[providerID] = sortedStringKeys(set)
	}
	return models
}
