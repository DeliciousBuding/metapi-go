package backup

import (
	"encoding/json"
	"sort"
	"strings"
)

// Developer associations are compiled into the imported route graph. Other
// source system settings never overwrite this deployment's settings.
func decodeAxonHubDeveloperSettings(configs []AxonHubSourceSystemConfig) (map[string][]AxonHubSourceAssociation, error) {
	out := map[string][]AxonHubSourceAssociation{}
	seen := false
	for _, config := range configs {
		if config.Key != "system_model_settings" {
			continue
		}
		if seen {
			return nil, axonHubErr("invalid AxonHub backup: duplicate system model settings")
		}
		seen = true
		raw := []byte(config.Value)
		if err := checkSourceJSONKeys(raw); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: malformed system model settings")
		}
		var settings struct {
			Developers []*struct {
				Developer    string            `json:"developer"`
				Associations []json.RawMessage `json:"associations"`
			} `json:"developer_settings"`
		}
		if err := json.Unmarshal(raw, &settings); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: malformed developer settings")
		}
		for _, developer := range settings.Developers {
			if developer == nil {
				continue
			}
			name := strings.TrimSpace(developer.Developer)
			if name == "" {
				return nil, axonHubErr("invalid AxonHub backup: developer name is missing")
			}
			if _, ok := out[name]; ok {
				return nil, axonHubErr("invalid AxonHub backup: duplicate developer settings")
			}
			out[name] = nil
			for _, rawAssoc := range developer.Associations {
				if string(rawAssoc) == "null" {
					continue
				}
				assoc, err := decodeAxonHubAssociation(rawAssoc)
				if err != nil {
					return nil, err
				}
				switch assoc.Type {
				case "channel_model":
					if assoc.ChannelModel == nil || assoc.ChannelModel.ChannelID <= 0 {
						return nil, axonHubErr("invalid AxonHub backup: developer channel association requires a channel")
					}
				case "channel_tags_model":
					if assoc.ChannelTagsModel == nil || len(assoc.ChannelTagsModel.ChannelTags) == 0 {
						return nil, axonHubErr("invalid AxonHub backup: developer tag association requires tags")
					}
				default:
					return nil, axonHubErr("invalid AxonHub backup: unsupported developer association type")
				}
				out[name] = append(out[name], assoc)
			}
		}
	}
	return out, nil
}

func axonHubEffectiveAssociations(src *AxonHubSource, model AxonHubSourceModel) []AxonHubSourceAssociation {
	out := append([]AxonHubSourceAssociation(nil), model.Settings.Associations...)
	if !model.Settings.DisableDeveloperSettingsInheritance {
		for _, assoc := range src.DeveloperAssociations[model.Developer] {
			// Copy nested selectors before filling the concrete model so siblings
			// cannot mutate the shared developer rule or each other's routes.
			if assoc.ChannelModel != nil {
				selector := *assoc.ChannelModel
				selector.ModelID = model.ModelID
				assoc.ChannelModel = &selector
			}
			if assoc.ChannelTagsModel != nil {
				selector := *assoc.ChannelTagsModel
				selector.ModelID = model.ModelID
				assoc.ChannelTagsModel = &selector
			}
			out = append(out, assoc)
		}
	}
	// Local rules precede inherited rules at equal priority, as in the source.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}
