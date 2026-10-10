package backup

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// Keep the source's merge order: a custom endpoint replaces its own default
// in place, while genuinely new formats are appended in user order.
func axonHubMergedEndpoints(ch AxonHubSourceChannel, provider axonHubProviderType) []AxonHubSourceEndpoint {
	out := make([]AxonHubSourceEndpoint, 0, len(provider.DefaultFormats)+len(ch.Endpoints))
	indices := map[string]int{}
	for _, format := range provider.DefaultFormats {
		indices[format] = len(out)
		out = append(out, AxonHubSourceEndpoint{APIFormat: format})
	}
	for _, ep := range ch.Endpoints {
		ep.APIFormat = strings.TrimSpace(ep.APIFormat)
		if i, ok := indices[ep.APIFormat]; ok {
			out[i] = ep
		} else {
			indices[ep.APIFormat] = len(out)
			out = append(out, ep)
		}
	}
	return out
}

func axonHubDeclaredFormats(ch AxonHubSourceChannel, provider axonHubProviderType) map[string]bool {
	out := map[string]bool{}
	for _, ep := range axonHubMergedEndpoints(ch, provider) {
		out[ep.APIFormat] = true
	}
	return out
}

func axonHubEndpointOrder(ch AxonHubSourceChannel, provider axonHubProviderType) store.DirectProtocolOrder {
	var out store.DirectProtocolOrder
	seen := map[int]bool{}
	add := func(bit int) {
		if bit != 0 && !seen[bit] {
			seen[bit] = true
			out = append(out, bit)
		}
	}
	for _, ep := range axonHubMergedEndpoints(ch, provider) {
		add(axonHubServableFormats[ep.APIFormat])
	}
	return out
}

// Match the source's exact-name lookup and ordered union for the selected entry.
// Store this on the route item, never union it into the shared channel grant.
func axonHubModelProtocolOrder(ch *axonHubPlanChannel, names ...string) (store.DirectProtocolOrder, bool) {
	var formats []string
	seen := map[string]bool{}
	for _, name := range names {
		for _, override := range ch.ModelProtocols {
			if override.Model != name || (override.Enabled != nil && !*override.Enabled) {
				continue
			}
			for _, format := range override.APIFormats {
				if !seen[format] {
					formats = append(formats, format)
					seen[format] = true
				}
			}
			break
		}
	}
	if len(formats) == 0 {
		return ch.ProtocolOrder, true
	}
	var out store.DirectProtocolOrder
	matchedDeclared := false
	for _, format := range formats {
		if !ch.DeclaredFormats[format] {
			continue
		}
		matchedDeclared = true
		if bit := axonHubServableFormats[format]; bit != 0 && ch.Protocols&bit != 0 {
			out = append(out, bit)
		}
	}
	if len(out) > 0 {
		return out, true
	}
	if matchedDeclared {
		return nil, false
	}
	// Source configuration drift falls back to the channel's configured endpoints.
	return ch.ProtocolOrder, true
}
