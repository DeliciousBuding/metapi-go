package routing

import (
	"strings"
)

// NormalizeModelAlias strips the vendor prefix and lowercases.
// E.g. "anthropic/claude-sonnet" → "claude-sonnet"
func NormalizeModelAlias(modelName string) string {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	if normalized == "" {
		return ""
	}
	slashIndex := strings.LastIndex(normalized, "/")
	if slashIndex >= 0 && slashIndex < len(normalized)-1 {
		return normalized[slashIndex+1:]
	}
	return normalized
}

// IsModelAliasEquivalent checks if two model names are equivalent after alias normalization.
func IsModelAliasEquivalent(left, right string) bool {
	a := NormalizeModelAlias(left)
	b := NormalizeModelAlias(right)
	return a != "" && b != "" && a == b
}

// ChannelSupportsRequestedModel checks if a channel's source model supports the requested model.
func ChannelSupportsRequestedModel(channelSourceModel *string, requestedModel string) bool {
	if channelSourceModel == nil {
		return true
	}
	source := strings.TrimSpace(*channelSourceModel)
	if source == "" {
		return true
	}
	if source == requestedModel {
		return true
	}
	if IsModelAliasEquivalent(source, requestedModel) {
		return true
	}
	if MatchesModelPattern(requestedModel, source) {
		return true
	}
	return false
}

// ChannelSupportsRequestedModelWithRedirects is the redirect-aware variant:
// besides the
// base checks it consults the in-process redirect registry, so a channel whose
// source_model is an upstream `actual` name (e.g. claude-3-5-sonnet-20241022)
// becomes eligible for a request of its `canonical` (claude-3-5-sonnet) on the
// same account. Pure function over the registry; no DB access.
func ChannelSupportsRequestedModelWithRedirects(channelSourceModel *string, requestedModel string, accountID int64) bool {
	if ChannelSupportsRequestedModel(channelSourceModel, requestedModel) {
		return true
	}
	if channelSourceModel == nil {
		return false
	}
	source := strings.TrimSpace(*channelSourceModel)
	if source == "" {
		return false
	}
	canonical := ModelRedirectCanonical(accountID, source)
	return canonical != "" && canonical == requestedModel
}

// NormalizeRouteDisplayName trims whitespace from a display name.
func NormalizeRouteDisplayName(displayName *string) string {
	if displayName == nil {
		return ""
	}
	return strings.TrimSpace(*displayName)
}

// IsRouteDisplayNameMatch checks if a model matches a route's display name exactly.
func IsRouteDisplayNameMatch(model string, displayName *string) bool {
	alias := NormalizeRouteDisplayName(displayName)
	return alias != "" && alias == model
}

// IsExplicitGroupRoute checks if a route is an explicit_group route.
func IsExplicitGroupRoute(routeMode string) bool {
	return routeMode == "explicit_group"
}

// HasCustomDisplayName checks if a route has a non-default display name.
func HasCustomDisplayName(modelPattern string, displayName *string) bool {
	dn := NormalizeRouteDisplayName(displayName)
	mp := strings.TrimSpace(modelPattern)
	return dn != "" && dn != mp
}

// GetExposedModelNameForRoute returns the display name or model pattern.
func GetExposedModelNameForRoute(displayName *string, modelPattern string) string {
	name := NormalizeRouteDisplayName(displayName)
	if name != "" {
		return name
	}
	return modelPattern
}

// NormalizeChannelSourceModel trims whitespace from a channel source model.
func NormalizeChannelSourceModel(channelSourceModel *string) string {
	if channelSourceModel == nil {
		return ""
	}
	return strings.TrimSpace(*channelSourceModel)
}

// ResolveActualModelForSelectedChannel resolves the actual model to forward.
// Resolution order (first match wins):
// 1. display-name hit → the channel's own source model (existing behavior);
// 2. Per-account redirect (canonical → actual) when no route-level
// model mapping already rewrote the name — the upstream only knows the
// dated/versioned actual name;
// 3. route-level model mapping (existing fallback).
func ResolveActualModelForSelectedChannel(
	requestedModel string,
	routeDisplayName *string,
	mappedModel string,
	channelSourceModel *string,
	accountID int64,
) string {
	sourceModel := NormalizeChannelSourceModel(channelSourceModel)
	if IsRouteDisplayNameMatch(requestedModel, routeDisplayName) && sourceModel != "" {
		return sourceModel
	}
	// Route-level mapping wins over redirects when it rewrote the name.
	if mappedModel == requestedModel {
		if actual := ModelRedirectActual(accountID, requestedModel); actual != "" {
			return actual
		}
	}
	return mappedModel
}
