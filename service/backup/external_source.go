package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ExternalSource is a recognized upstream backup contract, not a Metapi backup.
// The inspector never returns credential material or changes the database.
type ExternalSource string

const (
	ExternalOctopusV5  ExternalSource = "octopus-v5"
	ExternalAxonHubV14 ExternalSource = "axonhub-v1.4"
)

type ExternalSourceManifest struct {
	Source   ExternalSource `json:"source"`
	Sections map[string]int `json:"sections"`
}

// InspectExternalSource checks the format and source graph before any mapping.
// Importers must separately validate each provider's capabilities and convert
// the entire graph; a successful inspection is not a successful import.
func InspectExternalSource(raw []byte) (*ExternalSourceManifest, error) {
	if len(raw) > 64<<20 {
		return nil, fmt.Errorf("external backup exceeds 64 MiB")
	}
	if err := checkSourceJSONKeys(raw); err != nil {
		return nil, fmt.Errorf("invalid external backup: %w", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return nil, fmt.Errorf("invalid external backup: expected a JSON object")
	}
	version := bytes.TrimSpace(top["version"])
	switch {
	case bytes.Equal(version, []byte("5")) && top["exported_at"] != nil:
		return inspectOctopusV5(top)
	case bytes.Equal(version, []byte(`"1.4"`)) && top["timestamp"] != nil:
		return inspectAxonHubV14(top)
	default:
		return nil, fmt.Errorf("unsupported external backup source or version")
	}
}

// A source backup may contain credentials. Reject ambiguous duplicate keys at
// every depth, without echoing an attacker-chosen key or value in the error.
func checkSourceJSONKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := scanSourceJSONValue(dec); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("expected a single JSON value")
	}
	return nil
}

func scanSourceJSONValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("malformed JSON")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	expectedClose := json.Delim('}')
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return fmt.Errorf("malformed JSON object")
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return fmt.Errorf("duplicate or invalid JSON object key")
			}
			seen[key] = true
			if err := scanSourceJSONValue(dec); err != nil {
				return err
			}
		}
	case '[':
		expectedClose = ']'
		for dec.More() {
			if err := scanSourceJSONValue(dec); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("malformed JSON delimiter")
	}
	closeToken, err := dec.Token()
	if err != nil || closeToken != expectedClose {
		return fmt.Errorf("malformed JSON close")
	}
	return nil
}

type octopusSourceChannel struct {
	ID int64 `json:"id"`
}

type octopusSourceChild struct {
	ID        int64  `json:"id"`
	ChannelID int64  `json:"channel_id"`
	Name      string `json:"name"`
}

type octopusSourceGrant struct {
	ID             int64 `json:"id"`
	ChannelModelID int64 `json:"channel_model_id"`
	ChannelKeyID   int64 `json:"channel_key_id"`
	Protocols      int   `json:"protocols"`
}

type octopusSourceGroup struct {
	ID int64 `json:"id"`
}

type octopusSourceGroupItem struct {
	ID             int64 `json:"id"`
	GroupID        int64 `json:"group_id"`
	ChannelGrantID int64 `json:"channel_grant_id"`
}

func inspectOctopusV5(top map[string]json.RawMessage) (*ExternalSourceManifest, error) {
	var channels []octopusSourceChannel
	var keys, models []octopusSourceChild
	var grants []octopusSourceGrant
	var groups []octopusSourceGroup
	var items []octopusSourceGroupItem
	if err := decodeSourceSection(top, "channels", &channels); err != nil {
		return nil, err
	}
	if err := decodeSourceSection(top, "channel_keys", &keys); err != nil {
		return nil, err
	}
	if err := decodeSourceSection(top, "channel_models", &models); err != nil {
		return nil, err
	}
	if err := decodeSourceSection(top, "channel_grants", &grants); err != nil {
		return nil, err
	}
	if err := decodeSourceSection(top, "groups", &groups); err != nil {
		return nil, err
	}
	if err := decodeSourceSection(top, "group_items", &items); err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("octopus backup has no channels to import")
	}

	channelIDs := map[int64]bool{}
	for _, ch := range channels {
		if ch.ID <= 0 || channelIDs[ch.ID] {
			return nil, fmt.Errorf("octopus channels contain an invalid or duplicate id")
		}
		channelIDs[ch.ID] = true
	}
	keysByID := map[int64]int64{}
	modelsByID := map[int64]int64{}
	for _, entry := range []struct {
		name string
		rows []octopusSourceChild
		ids  map[int64]int64
	}{
		{"channel_keys", keys, keysByID}, {"channel_models", models, modelsByID},
	} {
		for _, row := range entry.rows {
			if row.ID <= 0 || entry.ids[row.ID] != 0 || !channelIDs[row.ChannelID] || strings.TrimSpace(row.Name) == "" {
				return nil, fmt.Errorf("octopus %s contain an invalid id, name or channel reference", entry.name)
			}
			entry.ids[row.ID] = row.ChannelID
		}
	}
	grantIDs := map[int64]bool{}
	for _, grant := range grants {
		if grant.ID <= 0 || grantIDs[grant.ID] || keysByID[grant.ChannelKeyID] == 0 ||
			keysByID[grant.ChannelKeyID] != modelsByID[grant.ChannelModelID] || grant.Protocols <= 0 || grant.Protocols&^14 != 0 {
			return nil, fmt.Errorf("octopus channel_grants contain an invalid model/key/protocol relationship")
		}
		grantIDs[grant.ID] = true
	}
	groupIDs := map[int64]bool{}
	for _, group := range groups {
		if group.ID <= 0 || groupIDs[group.ID] {
			return nil, fmt.Errorf("octopus groups contain an invalid or duplicate id")
		}
		groupIDs[group.ID] = true
	}
	itemIDs := map[int64]bool{}
	for _, item := range items {
		if item.ID <= 0 || itemIDs[item.ID] || !groupIDs[item.GroupID] || !grantIDs[item.ChannelGrantID] {
			return nil, fmt.Errorf("octopus group_items contain an invalid group/grant reference")
		}
		itemIDs[item.ID] = true
	}
	sections := map[string]int{
		"channels": len(channels), "channelKeys": len(keys), "channelModels": len(models),
		"channelGrants": len(grants), "groups": len(groups), "groupItems": len(items),
	}
	for _, section := range []string{"api_keys", "llm_infos", "settings", "stats_total", "stats_daily", "stats_hourly", "stats_api_key"} {
		count, err := sourceSectionLength(top, section)
		if err != nil {
			return nil, err
		}
		sections[section] = count
	}
	return &ExternalSourceManifest{Source: ExternalOctopusV5, Sections: sections}, nil
}

func inspectAxonHubV14(top map[string]json.RawMessage) (*ExternalSourceManifest, error) {
	var channels []struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	}
	if err := decodeSourceSection(top, "channels", &channels); err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("axonhub backup has no channels to import")
	}
	ids := map[int64]bool{}
	for _, channel := range channels {
		if channel.ID <= 0 || ids[channel.ID] || strings.TrimSpace(channel.Type) == "" {
			return nil, fmt.Errorf("axonhub channels contain an invalid id or provider type")
		}
		ids[channel.ID] = true
	}
	sections := map[string]int{"channels": len(channels)}
	for _, section := range []string{"projects", "models", "channel_model_prices", "api_keys", "system_configs", "usage_requests", "usage_logs"} {
		count, err := sourceSectionLength(top, section)
		if err != nil {
			return nil, err
		}
		sections[section] = count
	}
	return &ExternalSourceManifest{Source: ExternalAxonHubV14, Sections: sections}, nil
}

func decodeSourceSection[T any](top map[string]json.RawMessage, section string, dst *[]T) error {
	raw := top[section]
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil || *dst == nil {
		return fmt.Errorf("invalid external backup: %s must be an array", section)
	}
	return nil
}

func sourceSectionLength(top map[string]json.RawMessage, section string) (int, error) {
	var rows []json.RawMessage
	if err := decodeSourceSection(top, section, &rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}
