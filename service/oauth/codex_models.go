package oauth

import (
	"regexp"
	"sort"
	"strings"
)

// CodexQuotaProbeModelFallback is used when no discovered/preferred model
// is available for the reverse-engineered quota probe.
const CodexQuotaProbeModelFallback = "gpt-5.5"

// CodexSeedModels is a non-exhaustive, version-flexible seed of known Codex
// cloud models used for fixtures and allowlist regression tests. It must keep
// current models (gpt-5.5) alongside recent predecessors so allowlists are not
// obsolete-only.
var CodexSeedModels = []string{
	"gpt-5.5",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.3-codex",
	"gpt-5.2-codex",
	"gpt-5.2",
	"gpt-5.1-codex-max",
	"gpt-5.1-codex",
	"gpt-5.1",
	"gpt-5.1-codex-mini",
}

// Preferred order for quota probe model selection (newest / most capable first).
var codexQuotaProbePreference = []string{
	"gpt-5.5",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.3-codex",
	"gpt-5.2-codex",
	"gpt-5.2",
	"gpt-5.1-codex-max",
	"gpt-5.1-codex",
	"gpt-5.1",
}

var gpt5FamilyPattern = regexp.MustCompile(`(?i)^gpt-5(\.|$|-)`)

// IsCodexGPT5FamilyModel reports whether model is in the gpt-5.x family
// (gpt-5, gpt-5.4, gpt-5.5, gpt-5.2-codex, …). Version-flexible — does not
// hardcode a single minor version.
func IsCodexGPT5FamilyModel(model string) bool {
	name := strings.TrimSpace(model)
	if name == "" {
		return false
	}
	return gpt5FamilyPattern.MatchString(name)
}

// IsCodexModelAllowed reports whether a model is permitted for Codex OAuth
// routing/discovery/quota paths. gpt-5.x family models (including gpt-5.5) are
// always allowed; any other name is allowed only when present in discovered.
func IsCodexModelAllowed(model string, discovered []string) bool {
	name := strings.TrimSpace(model)
	if name == "" {
		return false
	}
	if IsCodexGPT5FamilyModel(name) {
		return true
	}
	target := strings.ToLower(name)
	for _, d := range discovered {
		if strings.ToLower(strings.TrimSpace(d)) == target {
			return true
		}
	}
	return false
}

// NormalizeDiscoveredCodexModels trims, drops empties, and case-insensitively
// dedupes while preserving first-seen original casing.
func NormalizeDiscoveredCodexModels(models []string) []string {
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, raw := range models {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	return out
}

// MergeCodexDiscoveredWithSeed merges live discovery with seed models so that
// fixtures and first-login paths keep gpt-5.5 (and family) available even when
// upstream returns a partial list. Discovered names win on case collisions.
func MergeCodexDiscoveredWithSeed(discovered []string) []string {
	merged := NormalizeDiscoveredCodexModels(discovered)
	seen := make(map[string]struct{}, len(merged)+len(CodexSeedModels))
	for _, m := range merged {
		seen[strings.ToLower(m)] = struct{}{}
	}
	for _, seed := range CodexSeedModels {
		key := strings.ToLower(seed)
		if _, ok := seen[key]; ok {
			continue
		}
		// Only seed gpt-5.x family; never invent non-family models.
		if !IsCodexGPT5FamilyModel(seed) {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, seed)
	}
	return merged
}

// FilterCodexAllowedModels keeps models that pass IsCodexModelAllowed.
// When discovered is empty, only gpt-5.x family names pass.
func FilterCodexAllowedModels(models []string, discovered []string) []string {
	out := make([]string, 0, len(models))
	for _, m := range models {
		if IsCodexModelAllowed(m, discovered) {
			out = append(out, strings.TrimSpace(m))
		}
	}
	return NormalizeDiscoveredCodexModels(out)
}

// SelectCodexQuotaProbeModel chooses a model for the Codex quota probe.
// Prefers the newest known preferred model present in discovered; otherwise
// prefers any gpt-5.x from discovered; finally falls back to gpt-5.5 so the
// probe path is not stuck on obsolete-only gpt-5.4.
func SelectCodexQuotaProbeModel(discovered []string) string {
	normalized := NormalizeDiscoveredCodexModels(discovered)
	if len(normalized) > 0 {
		index := make(map[string]string, len(normalized))
		for _, m := range normalized {
			index[strings.ToLower(m)] = m
		}
		for _, pref := range codexQuotaProbePreference {
			if actual, ok := index[strings.ToLower(pref)]; ok {
				return actual
			}
		}
		// Any gpt-5.x from discovery, sorted for stability.
		var family []string
		for _, m := range normalized {
			if IsCodexGPT5FamilyModel(m) {
				family = append(family, m)
			}
		}
		if len(family) > 0 {
			sort.Slice(family, func(i, j int) bool {
				return strings.ToLower(family[i]) > strings.ToLower(family[j])
			})
			return family[0]
		}
		return normalized[0]
	}
	return CodexQuotaProbeModelFallback
}
