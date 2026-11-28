package backup

import (
	"encoding/json"
)

func decodeAxonHubSystemConfigs(raw json.RawMessage) ([]AxonHubSourceSystemConfig, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: system_configs must be an array")
	}
	out := make([]AxonHubSourceSystemConfig, 0, len(rows))
	for i, row := range rows {
		var entry AxonHubSourceSystemConfig
		if err := json.Unmarshal(row, &entry); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: system_configs[%d] is invalid", i)
		}
		out = append(out, entry)
	}
	return out, nil
}

func decodeAxonHubProjects(raw json.RawMessage) ([]AxonHubSourceProject, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: projects must be an array")
	}
	out := make([]AxonHubSourceProject, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			ID          int             `json:"id"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Status      string          `json:"status"`
			DeletedAt   int             `json:"deleted_at"`
			Profiles    json.RawMessage `json:"profiles"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: projects[%d] is invalid", i)
		}
		profiles, err := decodeAxonHubAccessProfiles(shape.Profiles, true)
		if err != nil {
			return nil, err
		}
		out = append(out, AxonHubSourceProject{
			Profiles: profiles, DeletedAt: shape.DeletedAt,
			ID: shape.ID, Name: shape.Name, Description: shape.Description,
			Status: shape.Status, ProfileRows: profileRowCount(shape.Profiles),
		})
	}
	return out, nil
}

func profileRowCount(raw json.RawMessage) int {
	var shape struct {
		Profiles []json.RawMessage `json:"profiles"`
	}
	if json.Unmarshal(raw, &shape) != nil {
		return 0
	}
	return len(shape.Profiles)
}

func decodeAxonHubAPIKeys(raw json.RawMessage) ([]AxonHubSourceAPIKey, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: api_keys must be an array")
	}
	out := make([]AxonHubSourceAPIKey, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			Key        string          `json:"key"`
			ID         int             `json:"id"`
			ProjectID  int             `json:"project_id"`
			Name       string          `json:"name"`
			Type       string          `json:"type"`
			Status     string          `json:"status"`
			DeletedAt  int             `json:"deleted_at"`
			Scopes     []string        `json:"scopes"`
			Profiles   json.RawMessage `json:"profiles"`
			AllowedIPs []string        `json:"allowed_ips"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: api_keys[%d] is invalid", i)
		}
		profiles, err := decodeAxonHubAccessProfiles(shape.Profiles, false)
		if err != nil {
			return nil, err
		}
		out = append(out, AxonHubSourceAPIKey{
			Key: shape.Key, Profiles: profiles, IPAllowlist: shape.AllowedIPs,
			ID: shape.ID, ProjectID: shape.ProjectID, Name: shape.Name, Type: shape.Type,
			Status: shape.Status, DeletedAt: shape.DeletedAt, Scopes: shape.Scopes,
			ProfileRows: profileRowCount(shape.Profiles), AllowedIPs: len(shape.AllowedIPs),
			HasQuota: apiKeyProfilesHaveQuota(shape.Profiles),
		})
	}
	return out, nil
}

func apiKeyProfilesHaveQuota(raw json.RawMessage) bool {
	var shape struct {
		Profiles []struct {
			Quota json.RawMessage `json:"quota"`
		} `json:"profiles"`
	}
	if json.Unmarshal(raw, &shape) != nil {
		return false
	}
	for _, profile := range shape.Profiles {
		if present(profile.Quota) {
			return true
		}
	}
	return false
}
