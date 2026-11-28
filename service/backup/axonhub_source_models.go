package backup

import (
	"bytes"
	"encoding/json"

	"github.com/deliciousbuding/metapi-go/internal/routecondition"
)

var axonHubModelFields = []string{
	"id", "created_at", "updated_at", "deleted_at", "developer", "model_id", "type", "name",
	"icon", "group", "model_card", "status", "settings", "remark", "edges",
}

var axonHubModelSettingsFields = []string{
	"disableDeveloperSettingsInheritance", "associations", "loadBalancerStrategy", "traceStickyMode",
}

var axonHubAssociationFields = []string{
	"type", "priority", "disabled", "when", "channelModel", "channelRegex", "regex", "modelId",
	"channelTagsModel", "channelTagsRegex",
}

var axonHubExcludeFields = []string{"channelNamePattern", "channelIds", "channelTags"}

func decodeAxonHubModels(raw json.RawMessage) ([]AxonHubSourceModel, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: models must be an array")
	}
	out := make([]AxonHubSourceModel, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] is not an object", i)
		}
		if extra := unknownKeys(obj, axonHubModelFields); len(extra) > 0 {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] has unsupported fields", i)
		}
		var model AxonHubSourceModel
		if err := json.Unmarshal(row, &model); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] has an invalid field type", i)
		}
		if err := decodeAxonHubModelSettings(obj["settings"], &model.Settings); err != nil {
			return nil, err
		}
		out = append(out, model)
	}
	return out, nil
}

func decodeAxonHubModelSettings(raw json.RawMessage, dst *AxonHubSourceModelSettings) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: model settings must be an object")
	}
	dst.UnsupportedFields = unknownKeys(obj, axonHubModelSettingsFields)
	if err := json.Unmarshal(raw, dst); err != nil {
		return axonHubErr("invalid AxonHub backup: model settings has an invalid field type")
	}
	rows, err := rawSectionRows(obj["associations"])
	if err != nil {
		return axonHubErr("invalid AxonHub backup: model associations must be an array")
	}
	for i, row := range rows {
		assoc, err := decodeAxonHubAssociation(row)
		if err != nil {
			return axonHubErr("invalid AxonHub backup: model associations[%d] is invalid", i)
		}
		dst.Associations = append(dst.Associations, assoc)
	}
	return nil
}

func decodeAxonHubAssociation(raw json.RawMessage) (AxonHubSourceAssociation, error) {
	var assoc AxonHubSourceAssociation
	obj, err := rowObject(raw)
	if err != nil {
		return assoc, err
	}
	if extra := unknownKeys(obj, axonHubAssociationFields); len(extra) > 0 {
		assoc.UnsupportedField = extra[0]
	}
	var shape struct {
		Type         string                     `json:"type"`
		Priority     int                        `json:"priority"`
		Disabled     bool                       `json:"disabled"`
		When         json.RawMessage            `json:"when"`
		ChannelModel *AxonHubSourceChannelModel `json:"channelModel"`
		ChannelRegex *AxonHubSourceChannelRegex `json:"channelRegex"`
		Regex        *struct {
			Pattern string            `json:"pattern"`
			Exclude []json.RawMessage `json:"exclude"`
		} `json:"regex"`
		ModelID *struct {
			ModelID string            `json:"modelId"`
			Exclude []json.RawMessage `json:"exclude"`
		} `json:"modelId"`
		ChannelTagsModel *AxonHubSourceTagsModel `json:"channelTagsModel"`
		ChannelTagsRegex *AxonHubSourceTagsRegex `json:"channelTagsRegex"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return assoc, err
	}
	assoc.Type = shape.Type
	assoc.Priority = shape.Priority
	assoc.Disabled = shape.Disabled
	if present(shape.When) {
		whenObject, err := rowObject(shape.When)
		if err != nil {
			return assoc, axonHubErr("invalid association condition")
		}
		var when struct {
			Enabled   bool            `json:"enabled"`
			Condition json.RawMessage `json:"condition"`
		}
		if err := json.Unmarshal(shape.When, &when); err != nil {
			return assoc, axonHubErr("invalid association condition")
		}
		if raw, exists := whenObject["enabled"]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return assoc, axonHubErr("invalid association condition enabled flag")
		}
		if extra := unknownKeys(whenObject, []string{"enabled", "condition"}); len(extra) > 0 && assoc.UnsupportedField == "" {
			assoc.UnsupportedField = "when." + sanitizeIdentifier(extra[0])
		}
		// AxonHub evaluates an enabled rule without a predicate as true.
		condition := bytes.TrimSpace(when.Condition)
		if when.Enabled && len(condition) > 0 && !bytes.Equal(condition, []byte("null")) {
			assoc.Condition, err = routecondition.Decode(condition)
			if err != nil {
				assoc.UnsupportedField = "when.condition"
			}
		}
	}
	assoc.ChannelModel = shape.ChannelModel
	assoc.ChannelRegex = shape.ChannelRegex
	assoc.ChannelTagsModel = shape.ChannelTagsModel
	assoc.ChannelTagsRegex = shape.ChannelTagsRegex
	if shape.Regex != nil {
		excludes, err := decodeAxonHubExcludes(shape.Regex.Exclude)
		if err != nil {
			return assoc, err
		}
		assoc.Regex = &AxonHubSourceRegex{Pattern: shape.Regex.Pattern, Exclude: excludes}
	}
	if shape.ModelID != nil {
		excludes, err := decodeAxonHubExcludes(shape.ModelID.Exclude)
		if err != nil {
			return assoc, err
		}
		assoc.ModelID = &AxonHubSourceModelIDMatch{ModelID: shape.ModelID.ModelID, Exclude: excludes}
	}
	return assoc, nil
}

func decodeAxonHubExcludes(rows []json.RawMessage) ([]AxonHubSourceExclude, error) {
	out := make([]AxonHubSourceExclude, 0, len(rows))
	for _, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return nil, err
		}
		if extra := unknownKeys(obj, axonHubExcludeFields); len(extra) > 0 {
			return nil, axonHubErr("unsupported exclude field")
		}
		var exclude AxonHubSourceExclude
		if err := json.Unmarshal(row, &exclude); err != nil {
			return nil, err
		}
		out = append(out, exclude)
	}
	return out, nil
}

func decodeAxonHubPrices(raw json.RawMessage) ([]AxonHubSourceModelPrice, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: channel_model_prices must be an array")
	}
	out := make([]AxonHubSourceModelPrice, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			ChannelName string          `json:"channel_name"`
			ModelID     string          `json:"model_id"`
			Price       json.RawMessage `json:"price"`
			ReferenceID string          `json:"reference_id"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channel_model_prices[%d] is invalid", i)
		}
		out = append(out, AxonHubSourceModelPrice{
			ChannelName: shape.ChannelName, ModelID: shape.ModelID,
			Price: shape.Price, ReferenceID: shape.ReferenceID,
		})
	}
	return out, nil
}
