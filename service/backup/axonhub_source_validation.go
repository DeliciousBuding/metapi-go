package backup

import (
	"fmt"
	"strings"
)

// AxonHubSourceError describes malformed or unsupported source data. Its text
// never includes a credential value or any other attacker-chosen payload.
type AxonHubSourceError struct{ Message string }

func (e AxonHubSourceError) Error() string { return e.Message }

func axonHubErr(format string, args ...any) error {
	return AxonHubSourceError{Message: fmt.Sprintf(format, args...)}
}

func validateAxonHubSource(src *AxonHubSource) error {
	channelIDs := make(map[int]struct{}, len(src.Channels))
	for i, ch := range src.Channels {
		if ch.ID <= 0 {
			return axonHubErr("invalid AxonHub backup: channels[%d] has a non-positive id", i)
		}
		if _, exists := channelIDs[ch.ID]; exists {
			return axonHubErr("invalid AxonHub backup: duplicate channel id %d", ch.ID)
		}
		channelIDs[ch.ID] = struct{}{}
		if strings.TrimSpace(ch.Type) == "" {
			return axonHubErr("invalid AxonHub backup: channels[%d] has no provider type", i)
		}
		switch ch.Status {
		case "", "enabled", "disabled", "archived":
		default:
			return axonHubErr("invalid AxonHub backup: channels[%d] has an unknown status", i)
		}
		formats := make(map[string]struct{}, len(ch.Endpoints))
		for j, ep := range ch.Endpoints {
			format := strings.TrimSpace(ep.APIFormat)
			if format == "" {
				return axonHubErr("invalid AxonHub backup: channel[%d].endpoints[%d] missing api_format", i, j)
			}
			if _, exists := formats[format]; exists {
				return axonHubErr("invalid AxonHub backup: duplicate endpoint format at channel[%d].endpoints[%d]", i, j)
			}
			formats[format] = struct{}{}
			switch ep.Transport {
			case "", "http", "websocket":
			default:
				return axonHubErr("invalid AxonHub backup: channel[%d].endpoints[%d] has an unknown transport", i, j)
			}
			if ep.Path != "" && !safeDirectEndpointPath(ep.Path) {
				return axonHubErr("invalid AxonHub backup: channel[%d].endpoints[%d] has an invalid path", i, j)
			}
		}
	}
	seenModelIDs := make(map[string]struct{}, len(src.Models))
	modelSourceIDs := make(map[int]struct{}, len(src.Models))
	for i, model := range src.Models {
		if model.ID <= 0 {
			return axonHubErr("invalid AxonHub backup: models[%d] has a non-positive id", i)
		}
		if _, exists := modelSourceIDs[model.ID]; exists {
			return axonHubErr("invalid AxonHub backup: duplicate model source id %d", model.ID)
		}
		modelSourceIDs[model.ID] = struct{}{}
		switch model.Status {
		case "", "enabled", "disabled", "archived":
		default:
			return axonHubErr("invalid AxonHub backup: models[%d] has an unknown status", i)
		}
		modelID := strings.TrimSpace(model.ModelID)
		if modelID == "" {
			return axonHubErr("invalid AxonHub backup: model[%d] has no model_id", i)
		}
		if model.DeletedAt != 0 {
			continue
		}
		if _, exists := seenModelIDs[modelID]; exists {
			return axonHubErr("invalid AxonHub backup: duplicate model_id %q", sanitizeIdentifier(modelID))
		}
		seenModelIDs[modelID] = struct{}{}
	}
	return nil
}

func safeJSONDecodeError(err error) string {
	// encoding/json errors identify a field/offset but never echo the document.
	return err.Error()
}
