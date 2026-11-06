package store

// DefaultRoutingOverride means inherit the route policy. Historical empty
// values and the source system_default spelling have the same meaning.
func DefaultRoutingOverride(value string) bool {
	return value == "" || value == "default" || value == "system_default"
}

func ValidRoutingOverride(value string) bool {
	if DefaultRoutingOverride(value) {
		return true
	}
	switch value {
	case "weighted", "round_robin", "stable_first", "least_busy", "lowest_latency", "lowest_cost":
		return true
	default:
		return false
	}
}

func ValidTraceStickyMode(value string) bool {
	return DefaultRoutingOverride(value) || value == "disabled" || value == "prefer_previous_channel"
}

func EffectiveTraceStickyMode(routeMode string, key *DownstreamAccessPolicy) string {
	if key != nil && !DefaultRoutingOverride(key.TraceStickyMode) {
		routeMode = key.TraceStickyMode
	}
	if routeMode == "prefer_previous_channel" {
		return routeMode
	}
	return "disabled"
}
