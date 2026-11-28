package backup

type magpiePlan struct {
	providers   []magpiePlanProvider
	credentials []magpiePlanCredential
	models      []magpiePlanModel
	grants      []magpiePlanGrant
	routes      []magpiePlanRoute
	keys        []magpiePlanKey

	notImported      map[string]int
	residuals        []string
	skippedProviders []MagpieSkippedProvider
	skippedGroups    []MagpieSkippedGroup
	blocking         []string
}

type magpiePlanModel struct {
	SourceID      int64
	ProviderID    string
	Name          string
	Enabled       bool
	ContextLength *int
}

type magpiePlanGrant struct {
	SourceID   int64
	ProviderID string
	ModelName  string
	Credential string
	Protocols  int
	Enabled    bool
}

type magpiePlanRoute struct {
	SourceID    int64
	Pattern     string
	DisplayName string
	Strategy    string
	Enabled     bool
	Context     *int
	Group       magpiePlanGroup
}

type magpieGrantRef struct {
	ProviderID string
	Model      string
	Credential string
}

func magpiePlanSourceIDs(plan *magpiePlan) map[string]map[int64]bool {
	ids := map[string]map[int64]bool{}
	mark := func(entity string, sourceID int64) {
		if ids[entity] == nil {
			ids[entity] = map[int64]bool{}
		}
		ids[entity][sourceID] = true
	}
	for _, provider := range plan.providers {
		mark("upstream_channels", provider.SourceID)
	}
	for _, credential := range plan.credentials {
		mark("upstream_credentials", credential.SourceID)
	}
	for _, model := range plan.models {
		mark("upstream_models", model.SourceID)
	}
	for _, grant := range plan.grants {
		mark("upstream_grants", grant.SourceID)
	}
	for _, route := range plan.routes {
		mark("token_routes", route.SourceID)
		mark("upstream_groups", route.Group.SourceID)
		for _, item := range route.Group.Items {
			mark("upstream_group_items", item.SourceID)
		}
	}
	for _, key := range plan.keys {
		mark("downstream_api_keys", key.SourceID)
	}
	return ids
}

func magpiePlanCounts(plan *magpiePlan) map[string]int {
	return map[string]int{
		"channels": len(plan.providers), "credentials": len(plan.credentials),
		"models": len(plan.models), "grants": len(plan.grants),
		"routes": len(plan.routes), "groups": len(plan.routes), "gatewayKeys": len(plan.keys),
	}
}

func magpieSourceSections(src *MagpieSource) map[string]int {
	out := map[string]int{}
	for key, value := range src.Sections {
		out[key] = value
	}
	for key, value := range src.Unknown {
		out["section:"+key] = value
	}
	return out
}
