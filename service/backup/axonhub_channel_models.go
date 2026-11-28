package backup

import (
	"strings"
)

func axonHubNativeChatMapping(channel AxonHubSourceChannel) bool {
	if len(channel.Settings.TransformOptions.ReasoningEffortMapping) == 0 {
		return false
	}
	for _, endpoint := range channel.Endpoints {
		if endpoint.APIFormat == "openai/chat_completions" {
			return false
		}
	}
	switch channel.Type {
	case "openai", "atlascloud", "deepinfra", "qiniu", "minimax", "ppio", "siliconflow", "vercel",
		"aihubmix", "burncloud", "github", "evolink", "groq":
		return true
	default:
		return false
	}
}

// axonHubChannelEntries reproduces AxonHub's Channel.GetModelEntries: the
// request-model names a channel can accept, and the upstream model each one
// resolves to. Re-imports depend on this being the same walk.
func axonHubChannelEntries(channel AxonHubSourceChannel) map[string]string {
	type entry struct {
		actual string
		source string
	}
	entries := map[string]entry{}
	add := func(request, actual, source string) {
		if request == "" {
			return
		}
		if _, exists := entries[request]; exists {
			return
		}
		entries[request] = entry{actual: actual, source: source}
	}
	for _, model := range channel.SupportedModels {
		add(model, model, "direct")
	}
	if prefix := channel.Settings.ExtraModelPrefix; prefix != "" {
		for _, model := range channel.SupportedModels {
			add(prefix+"/"+model, model, "prefix")
		}
	}
	for _, prefix := range channel.Settings.AutoTrimedModelPrefixes {
		if prefix == "" {
			continue
		}
		prefixed := prefix + "/"
		for _, model := range channel.SupportedModels {
			trimmed, ok := strings.CutPrefix(model, prefixed)
			if !ok {
				continue
			}
			add(trimmed, model, "auto_trim")
		}
	}
	supported := make(map[string]bool, len(channel.SupportedModels))
	for _, model := range channel.SupportedModels {
		supported[model] = true
	}
	for _, mapping := range channel.Settings.ModelMappings {
		if mapping.From == "" || !supported[mapping.To] {
			continue
		}
		if _, exists := entries[mapping.From]; exists {
			continue
		}
		add(mapping.From, mapping.To, "mapping")
		if channel.Settings.HideMappedModels {
			for key, value := range entries {
				if value.actual == mapping.To && value.source != "mapping" {
					delete(entries, key)
				}
			}
		}
	}
	if channel.Settings.HideOriginalModels {
		for key, value := range entries {
			if value.source == "direct" {
				delete(entries, key)
			}
		}
	}
	out := make(map[string]string, len(entries))
	if channel.Settings.LowercaseModelID {
		rank := map[string]int{"direct": 4, "auto_trim": 3, "mapping": 2, "prefix": 1}
		best := map[string]entry{}
		for key, value := range entries {
			lower := strings.ToLower(key)
			current, exists := best[lower]
			if !exists || rank[value.source] > rank[current.source] {
				best[lower] = value
			}
		}
		for key, value := range best {
			out[key] = value.actual
		}
		return out
	}
	for key, value := range entries {
		out[key] = value.actual
	}
	return out
}
