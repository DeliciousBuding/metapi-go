package backup

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

func compileMagpiePlan(src *MagpieSource) (*magpiePlan, error) {
	if src == nil {
		return nil, errors.New("Magpie source is missing")
	}
	plan := &magpiePlan{notImported: map[string]int{}}
	aliases := magpieProviderAliases(src)
	groupModels := magpieGroupReferencedModels(src, aliases)
	usedNames := map[string]int{}
	modelsByProvider := map[string]map[string]bool{}
	grantsByRef := map[magpieGrantRef]magpiePlanGrant{}

	for _, sourceProvider := range src.Providers {
		extraModels := groupModels[sourceProvider.ID]
		provider, reasons, residuals := compileMagpieProvider(sourceProvider, usedNames, extraModels)
		if len(extraModels) > 0 && provider != nil {
			plan.residuals = append(plan.residuals, "provider_models_completed_from_groups")
		}
		plan.residuals = append(plan.residuals, residuals...)
		if len(reasons) > 0 {
			plan.skippedProviders = append(plan.skippedProviders, MagpieSkippedProvider{
				SourceID: sanitizeIdentifier(sourceProvider.ID), Type: "provider", Reasons: uniquifyReasons(reasons),
			})
			continue
		}
		plan.providers = append(plan.providers, *provider)
		modelsByProvider[provider.ID] = map[string]bool{}
		for _, model := range provider.Models {
			modelsByProvider[provider.ID][model] = true
		}
		plan.credentials = append(plan.credentials, provider.Credentials...)
		for _, model := range provider.Models {
			plan.models = append(plan.models, magpiePlanModel{
				SourceID: sourceTupleID("magpie-model", provider.ID, model), ProviderID: provider.ID,
				Name: model, Enabled: provider.Enabled, ContextLength: magpieContext(sourceProvider, model),
			})
			for _, credential := range provider.Credentials {
				if !magpieCredentialServes(credential, model, provider.AccountModels) {
					continue
				}
				grant := magpiePlanGrant{
					SourceID:   sourceTupleID("magpie-grant", provider.ID, model, credential.Name),
					ProviderID: provider.ID, ModelName: model, Credential: credential.Name,
					Protocols: credential.Protocols, Enabled: provider.Enabled && credential.Enabled,
				}
				plan.grants = append(plan.grants, grant)
				grantsByRef[magpieGrantRef{provider.ID, model, credential.Name}] = grant
			}
		}
	}

	if len(plan.providers) == 0 {
		plan.blocking = append(plan.blocking, "no_servable_providers")
	}
	if len(src.Providers) > 0 && !src.KeysIncluded {
		plan.residuals = append(plan.residuals, "provider_keys_not_in_backup")
	}

	// Every provider/model stays addressable by the same "provider/model" id
	// Magpie exposed. Each public route owns one internal group.
	for i := range plan.providers {
		provider := &plan.providers[i]
		if len(provider.Models) == 0 {
			plan.notImported["providersWithoutExplicitModels"]++
			plan.residuals = append(plan.residuals, "provider_models_not_explicit")
			continue
		}
		for _, model := range provider.Models {
			grants, names := magpieGrantsForModel(plan, grantsByRef, provider.ID, model)
			if len(grants) == 0 {
				plan.notImported["modelsWithoutCredentials"]++
				continue
			}
			pattern := provider.ID + "/" + model
			groupSourceID := sourceTupleID("magpie-model-group", provider.ID, model)
			group := magpiePlanGroup{
				SourceID: groupSourceID, Name: uniqueMagpieName("model:"+pattern, model, usedNames),
				Mode: "failover", Enabled: provider.Enabled,
			}
			strategy := "weighted"
			switch provider.Routing {
			case "rotate":
				strategy = "round_robin"
			case "", "order", "usage", "pace", "weight":
			}
			for index, grant := range grants {
				credential := names[index]
				priority := 0
				if provider.Routing == "order" {
					priority = index
				}
				item := magpiePlanGroupItem{
					SourceID:    sourceTupleID("magpie-model-item", provider.ID, model, credential.Name),
					GrantSource: grant.SourceID, Priority: priority, Weight: max(1, credential.Weight),
				}
				group.Items = append(group.Items, item)
			}
			plan.routes = append(plan.routes, magpiePlanRoute{
				SourceID: sourceTupleID("magpie-route", pattern), Pattern: pattern,
				DisplayName: model, Strategy: strategy, Enabled: provider.Enabled,
				Context: magpieModelContext(plan, provider.ID, model), Group: group,
			})
		}
	}

	residuals := compileMagpieGroups(src, plan, aliases, modelsByProvider, grantsByRef, usedNames)
	plan.residuals = append(plan.residuals, residuals...)
	if err := compileMagpieGatewayKeys(src, plan); err != nil {
		return nil, err
	}

	plan.notImported["settings"] += boolCount(src.Sections["settings"] > 0)
	plan.notImported["profiles"] += src.Sections["profiles"]
	plan.notImported["agents"] += src.Sections["agents"]
	plan.notImported["library"] += boolCount(src.Sections["library"] > 0)
	plan.notImported["searches"] += src.Sections["searches"]
	plan.notImported["icons"] += src.Sections["icons"]
	plan.notImported["providerOrder"] += boolCount(src.Sections["order"] > 0)
	plan.notImported["groupOrder"] += boolCount(src.Sections["groupOrder"] > 0)
	for section, count := range src.Unknown {
		plan.notImported["section:"+section] += count
	}
	dropZeroCounts(plan.notImported)
	plan.residuals = uniqueSortedStrings(plan.residuals)
	sort.Slice(plan.skippedGroups, func(i, j int) bool { return plan.skippedGroups[i].SourceID < plan.skippedGroups[j].SourceID })
	return plan, nil
}

func magpieGrantsForModel(plan *magpiePlan, grants map[magpieGrantRef]magpiePlanGrant, providerID, model string) ([]magpiePlanGrant, []magpiePlanCredential) {
	provider := magpieProviderByID(plan, providerID)
	if provider == nil {
		return nil, nil
	}
	var out []magpiePlanGrant
	var credentials []magpiePlanCredential
	for _, credential := range provider.Credentials {
		grant, ok := grants[magpieGrantRef{providerID, model, credential.Name}]
		if !ok {
			continue
		}
		out = append(out, grant)
		credentials = append(credentials, credential)
	}
	return out, credentials
}

func magpieModelContext(plan *magpiePlan, providerID, model string) *int {
	for _, planned := range plan.models {
		if planned.ProviderID == providerID && planned.Name == model {
			return planned.ContextLength
		}
	}
	return nil
}

func magpieProviderByID(plan *magpiePlan, id string) *magpiePlanProvider {
	for i := range plan.providers {
		if plan.providers[i].ID == id {
			return &plan.providers[i]
		}
	}
	return nil
}

func uniqueMagpieName(name, fallback string, used map[string]int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s#%s", name, fallback)
		if suffix > 2 {
			candidate = fmt.Sprintf("%s#%s-%d", name, fallback, suffix)
		}
		if used[candidate] == 0 {
			used[candidate] = 1
			return candidate
		}
	}
}

func magpieIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !sourceIdentifierChar(r) {
			return false
		}
	}
	return true
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
