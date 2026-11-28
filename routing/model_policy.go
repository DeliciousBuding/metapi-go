package routing

// IsModelAllowedByDownstreamPolicy checks whether a model is allowed by the downstream routing policy.
func IsModelAllowedByDownstreamPolicy(requestedModel string, policy DownstreamRoutingPolicy) bool {
	if policy.UserRouteIDs != nil {
		if len(*policy.UserRouteIDs) == 0 {
			return false
		}
		if len(policy.SupportedModels) > 0 && !matchesSupportedModelPolicy(requestedModel, policy.SupportedModels) {
			return false
		}
		return policy.AccessPolicy.AllowsModel(requestedModel)
	}
	if !policy.AccessPolicy.AllowsModel(requestedModel) {
		return false
	}
	hasSupportedPatterns := len(policy.SupportedModels) > 0
	hasAllowedRoutes := len(policy.AllowedRouteIDs) > 0

	if !hasSupportedPatterns && !hasAllowedRoutes {
		return !policy.DenyAllWhenEmpty
	}

	matchedSupportedPattern := false
	for _, pattern := range policy.SupportedModels {
		if MatchesModelPattern(requestedModel, pattern) {
			matchedSupportedPattern = true
			break
		}
	}
	if matchedSupportedPattern {
		return true
	}
	if hasAllowedRoutes {
		return true
	}
	return false
}

// userRouteAllowed gates published outer routes, never the source routes
// inside a composite route. Internal supply does not grant a public entrypoint.
func userRouteAllowed(routeID int64, policy DownstreamRoutingPolicy) bool {
	if policy.UserRouteIDs == nil {
		return true
	}
	for _, id := range *policy.UserRouteIDs {
		if id == routeID {
			return true
		}
	}
	return false
}
