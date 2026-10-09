package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
)

func IsOctopusV5Payload(raw []byte) bool {
	var top map[string]json.RawMessage
	return json.Unmarshal(raw, &top) == nil && top != nil && bytes.Equal(bytes.TrimSpace(top["version"]), []byte("5")) && len(top["exported_at"]) > 0
}

// ParseOctopusV5 accepts only the published v5 envelope and rejects unknown
// fields in executable configuration sections. Credentials never appear in a
// preview or error.
func ParseOctopusV5(raw []byte) (*OctopusV5Dump, error) {
	if len(raw) > 20<<20 {
		return nil, fmt.Errorf("external backup exceeds 20 MiB")
	}
	if err := checkSourceJSONKeys(raw); err != nil {
		return nil, fmt.Errorf("invalid Octopus v5 backup: %w", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return nil, fmt.Errorf("invalid Octopus v5 backup: expected a JSON object")
	}
	if !bytes.Equal(bytes.TrimSpace(top["version"]), []byte("5")) {
		return nil, fmt.Errorf("unsupported Octopus backup version")
	}
	if err := rejectUnknownJSONFields(top, "version", "exported_at", "channels", "channel_keys", "channel_models", "channel_grants", "groups", "group_items", "api_keys", "llm_infos", "settings", "stats_total", "stats_daily", "stats_hourly", "stats_api_key"); err != nil {
		return nil, err
	}
	var d OctopusV5Dump
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("invalid Octopus v5 backup structure")
	}
	if _, _, err := octopusProxyURL(&d); err != nil {
		return nil, err
	}
	// Octopus omits zero-length slices because the v5 dump fields use
	// `omitempty`; absent optional arrays therefore mean an empty collection.
	if d.Channels == nil {
		return nil, fmt.Errorf("invalid Octopus v5 backup: channels must be an array")
	}
	if d.Credentials == nil {
		d.Credentials = []octopusKey{}
	}
	if d.Models == nil {
		d.Models = []octopusModel{}
	}
	if d.Grants == nil {
		d.Grants = []octopusGrant{}
	}
	if d.Groups == nil {
		d.Groups = []octopusGroup{}
	}
	if d.GroupItems == nil {
		d.GroupItems = []octopusItem{}
	}
	if len(d.Channels) == 0 {
		return nil, fmt.Errorf("Octopus v5 backup has no channels")
	}
	for _, rawSection := range []struct {
		name string
		rows []json.RawMessage
	}{
		{"channels", rawRows(top["channels"])}, {"channel_keys", rawRows(top["channel_keys"])}, {"channel_models", rawRows(top["channel_models"])}, {"channel_grants", rawRows(top["channel_grants"])}, {"groups", rawRows(top["groups"])}, {"group_items", rawRows(top["group_items"])},
	} {
		allowed := octopusAllowedFields[rawSection.name]
		for _, row := range rawSection.rows {
			var obj map[string]json.RawMessage
			if json.Unmarshal(row, &obj) != nil {
				return nil, fmt.Errorf("invalid Octopus v5 %s row", rawSection.name)
			}
			if err := rejectUnknownJSONFields(obj, allowed...); err != nil {
				return nil, fmt.Errorf("unsupported field in Octopus v5 %s", rawSection.name)
			}
		}
	}
	if err := validateOctopusV5(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

var octopusAllowedFields = map[string][]string{
	"channels":       {"id", "name", "dialect", "enabled", "base_url", "openai_chat_completion_path", "openai_response_path", "anthropic_message_path", "proxy", "channel_proxy", "custom_header", "param_override", "match_regex", "input_token", "output_token", "input_cost", "output_cost", "wait_time", "request_success", "request_failed"},
	"channel_keys":   {"id", "channel_id", "name", "key", "enabled", "input_token", "output_token", "input_cost", "output_cost", "wait_time", "request_success", "request_failed"},
	"channel_models": {"id", "channel_id", "name", "input_token", "output_token", "input_cost", "output_cost", "wait_time", "request_success", "request_failed"},
	"channel_grants": {"id", "channel_model_id", "channel_key_id", "protocols", "input_token", "output_token", "input_cost", "output_cost", "wait_time", "request_success", "request_failed"},
	"groups":         {"id", "name", "mode", "active_item_id", "relay_config", "items"},
	"group_items":    {"id", "group_id", "channel_grant_id", "priority", "weight", "channel_id", "channel_name", "model_name", "key_name", "protocols", "available"},
}

func rawRows(raw json.RawMessage) []json.RawMessage {
	var rows []json.RawMessage
	_ = json.Unmarshal(raw, &rows)
	return rows
}
func rejectUnknownJSONFields(row map[string]json.RawMessage, allowed ...string) error {
	set := map[string]bool{}
	for _, k := range allowed {
		set[k] = true
	}
	for k := range row {
		if !set[k] {
			return fmt.Errorf("unsupported field")
		}
	}
	return nil
}

func validateOctopusV5(d *OctopusV5Dump) error {
	channels := map[int64]octopusChannel{}
	keys := map[int64]octopusKey{}
	models := map[int64]octopusModel{}
	grants := map[int64]bool{}
	groups := map[int64]octopusGroup{}
	for _, c := range d.Channels {
		if c.ID <= 0 || c.Name == "" || c.Dialect != "generic" || strings.TrimSpace(c.BaseURL) == "" {
			return fmt.Errorf("Octopus channels contain an unsupported dialect or invalid identity/configuration")
		}
		if !safeOctopusEndpointPath(c.ChatPath) || !safeOctopusEndpointPath(c.ResponsesPath) || !safeOctopusEndpointPath(c.AnthropicPath) {
			return fmt.Errorf("Octopus channel contains an invalid protocol endpoint path")
		}
		if len(c.CustomHeader) > 0 && string(c.CustomHeader) != "null" {
			var headers []struct {
				HeaderKey   string `json:"header_key"`
				HeaderValue string `json:"header_value"`
			}
			if json.Unmarshal(c.CustomHeader, &headers) != nil {
				return fmt.Errorf("Octopus channel contains invalid custom headers")
			}
		}
		if _, ok := channels[c.ID]; ok {
			return fmt.Errorf("Octopus channels contain a duplicate id")
		}
		u, e := url.Parse(c.BaseURL)
		if e != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("Octopus channel contains an invalid base URL")
		}
		if c.ParamOverride != "" {
			var m map[string]json.RawMessage
			if json.Unmarshal([]byte(c.ParamOverride), &m) != nil || m == nil {
				return fmt.Errorf("Octopus channel has invalid parameter overrides")
			}
		}
		if c.Proxy && strings.TrimSpace(c.ChannelProxy) != "" && !validOctopusProxyURL(c.ChannelProxy) {
			return fmt.Errorf("Octopus channel contains an invalid proxy URL")
		}
		if c.Proxy && service.IsForbiddenSiteTargetURL(c.ChannelProxy) {
			return fmt.Errorf("Octopus channel proxy URL targets a forbidden metadata or link-local address")
		}
		if service.IsForbiddenSiteTargetURL(c.BaseURL) {
			return fmt.Errorf("Octopus channel base URL targets a forbidden metadata or link-local address")
		}
		channels[c.ID] = c
	}
	for _, k := range d.Credentials {
		if k.ID <= 0 || channels[k.ChannelID].ID == 0 || k.Name == "" || k.Key == "" {
			return fmt.Errorf("Octopus channel_keys contain an invalid id, name or channel reference")
		}
		if _, ok := keys[k.ID]; ok {
			return fmt.Errorf("Octopus channel_keys contain a duplicate id")
		}
		keys[k.ID] = k
	}
	for _, m := range d.Models {
		if m.ID <= 0 || channels[m.ChannelID].ID == 0 || m.Name == "" {
			return fmt.Errorf("Octopus channel_models contain an invalid id, name or channel reference")
		}
		if _, ok := models[m.ID]; ok {
			return fmt.Errorf("Octopus channel_models contain a duplicate id")
		}
		models[m.ID] = m
	}
	for _, g := range d.Grants {
		m, mok := models[g.ChannelModelID]
		k, kok := keys[g.ChannelKeyID]
		if g.ID <= 0 || !mok || !kok || m.ChannelID != k.ChannelID || g.Protocols <= 0 || g.Protocols&^14 != 0 {
			return fmt.Errorf("Octopus channel_grants contain an invalid model/key/protocol relationship")
		}
		if grants[g.ID] {
			return fmt.Errorf("Octopus channel_grants contain a duplicate id")
		}
		grants[g.ID] = true
	}
	for _, g := range d.Groups {
		if g.ID <= 0 || g.Name == "" || (g.Mode != "manual" && g.Mode != "failover") {
			return fmt.Errorf("Octopus groups contain an unsupported mode or invalid identity")
		}
		if !routing.IsExactRouteModelPattern(g.Name) {
			return fmt.Errorf("Octopus group name contains model-pattern syntax and cannot be imported as an exact route")
		}
		if groups[g.ID].ID != 0 {
			return fmt.Errorf("Octopus groups contain a duplicate id")
		}
		groups[g.ID] = g
	}
	itemGroups := map[int64]int64{}
	for _, i := range d.GroupItems {
		if i.ID <= 0 || groups[i.GroupID].ID == 0 || !grants[i.GrantID] || i.Priority < 0 || i.Weight < 0 || i.ChannelID != 0 || i.ChannelName != "" || i.ModelName != "" || i.KeyName != "" || i.Protocols != 0 || i.Available {
			return fmt.Errorf("Octopus group_items contain an invalid group/grant relationship")
		}
		if itemGroups[i.ID] != 0 {
			return fmt.Errorf("Octopus group_items contain a duplicate id")
		}
		itemGroups[i.ID] = i.GroupID
	}
	for _, g := range d.Groups {
		if g.ActiveItemID != 0 && itemGroups[g.ActiveItemID] != g.ID {
			return fmt.Errorf("Octopus groups reference a missing active item or an item from another group")
		}
	}
	return nil
}

func PreviewOctopusV5(raw []byte, originKey string) (*OctopusV5Preview, error) {
	d, err := ParseOctopusV5(raw)
	if err != nil {
		return nil, err
	}
	if err := validateOriginKey(originKey); err != nil {
		return nil, err
	}
	sections := map[string]int{"channels": len(d.Channels), "channelKeys": len(d.Credentials), "channelModels": len(d.Models), "channelGrants": len(d.Grants), "groups": len(d.Groups), "groupItems": len(d.GroupItems), "statsTotal": len(d.StatsTotal), "statsDaily": len(d.StatsDaily), "statsHourly": len(d.StatsHourly), "statsAPIKey": len(d.StatsAPIKey)}
	for _, channel := range d.Channels {
		if channel.OctopusStats.nonZero() {
			sections["channelStats"]++
		}
	}
	for _, key := range d.Credentials {
		if key.OctopusStats.nonZero() {
			sections["channelKeyStats"]++
		}
	}
	for _, model := range d.Models {
		if model.OctopusStats.nonZero() {
			sections["channelModelStats"]++
		}
	}
	for _, grant := range d.Grants {
		if grant.OctopusStats.nonZero() {
			sections["channelGrantStats"]++
		}
	}
	sections["statsRecords"] = sections["channelStats"] + sections["channelKeyStats"] + sections["channelModelStats"] + sections["channelGrantStats"] + len(d.StatsTotal) + len(d.StatsDaily) + len(d.StatsHourly) + len(d.StatsAPIKey)
	unsupported := unsupportedOctopusV5Sections(d)
	blocking := []string{}
	for _, channel := range d.Channels {
		if channelHasClientHeaderTemplate(channel.CustomHeader) {
			blocking = append(blocking, "clientHeaderTemplates")
			break
		}
		if channel.Proxy && strings.TrimSpace(channel.ChannelProxy) == "" {
			globalProxy, _, _ := octopusProxyURL(d)
			if globalProxy == "" {
				blocking = append(blocking, "channelProxyUnavailable")
				break
			}
		}
	}
	for _, group := range d.Groups {
		if groupRelayConfigBlocking(group.RelayConfig) {
			blocking = append(blocking, "groupRelayConfig")
			break
		}
	}
	adaptations := []string{}
	for _, group := range d.Groups {
		if groupRelayConfigNeedsAdaptation(group.RelayConfig) {
			adaptations = append(adaptations, "groupRelayConfigDefaults")
			break
		}
	}
	return &OctopusV5Preview{Source: OctopusV5Origin, OriginKey: originKey, Sections: sections, NotImported: unsupported, Adaptations: adaptations, Blocking: blocking}, nil
}

func safeOctopusEndpointPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return true
	}
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.Contains(path, "..") {
		return false
	}
	u, err := url.Parse(path)
	return err == nil && !u.IsAbs() && u.Host == "" && u.RawQuery == "" && u.Fragment == ""
}

func unsupportedOctopusV5Sections(d *OctopusV5Dump) map[string]int {
	unsupported := map[string]int{}
	_, importedSettings, _ := octopusProxyURL(d)
	for name, count := range map[string]int{"apiKeys": len(d.APIKeys), "llmInfos": len(d.LLMInfos), "settings": len(d.Settings) - importedSettings} {
		if count > 0 {
			unsupported[name] = count
		}
	}
	return unsupported
}

func groupRelayConfigBlocking(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null")) && !bytes.Equal(raw, []byte("{}")) && !groupRelayConfigNeedsAdaptation(raw)
}

func groupRelayConfigNeedsAdaptation(raw json.RawMessage) bool {
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || len(values) != 6 {
		return false
	}
	expected := map[string]int64{
		"member_max_attempts":                        2,
		"member_retry_interval_seconds":              3,
		"member_non_stream_response_timeout_seconds": 120,
		"member_stream_first_event_timeout_seconds":  30,
		"member_cooldown_seconds":                    60,
		"member_affinity_seconds":                    300,
	}
	for name, want := range expected {
		value, ok := values[name]
		if !ok {
			return false
		}
		var got int64
		if json.Unmarshal(value, &got) != nil || got != want {
			return false
		}
	}
	return true
}

func octopusProxyURL(d *OctopusV5Dump) (string, int, error) {
	if d == nil {
		return "", 0, nil
	}
	proxyURL := ""
	imported := 0
	seen := false
	for _, raw := range d.Settings {
		var setting octopusSetting
		if json.Unmarshal(raw, &setting) != nil {
			continue
		}
		if setting.Key != "proxy_url" {
			continue
		}
		var rawSetting map[string]json.RawMessage
		if json.Unmarshal(raw, &rawSetting) == nil {
			for key := range rawSetting {
				if key != "key" && key != "value" {
					return "", imported, fmt.Errorf("Octopus proxy_url setting contains unsupported fields")
				}
			}
		}
		if seen {
			return "", imported, fmt.Errorf("Octopus settings contain duplicate proxy_url entries")
		}
		seen = true
		imported++
		proxyURL = strings.TrimSpace(setting.Value)
		if proxyURL != "" && !validOctopusProxyURL(proxyURL) {
			return "", imported, fmt.Errorf("Octopus settings contain an invalid proxy_url")
		}
		if proxyURL != "" && service.IsForbiddenSiteTargetURL(proxyURL) {
			return "", imported, fmt.Errorf("Octopus proxy_url targets a forbidden metadata or link-local address")
		}
	}
	used := false
	for _, channel := range d.Channels {
		if channel.Proxy && strings.TrimSpace(channel.ChannelProxy) == "" {
			used = true
			break
		}
	}
	if imported > 0 && (proxyURL == "" || !used) {
		imported = 0
	}
	return proxyURL, imported, nil
}

func validOctopusProxyURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "socks5" || u.Scheme == "socks5h") && u.Host != "" && u.RawQuery == "" && u.Fragment == ""
}

func channelHasClientHeaderTemplate(raw json.RawMessage) bool {
	var headers []struct {
		Key   string `json:"header_key"`
		Value string `json:"header_value"`
	}
	if json.Unmarshal(raw, &headers) != nil {
		return false
	}
	for _, header := range headers {
		if _, ok := httpclient.ExpandClientHeaderTemplate(header.Value, nil); !ok {
			return true
		}
	}
	return false
}

func sortedSectionNames(sections map[string]int) []string {
	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validateOriginKey(v string) error {
	if v == "" || len(v) > 128 {
		return fmt.Errorf("originKey must be between 1 and 128 characters")
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return fmt.Errorf("originKey may contain only letters, digits, dot, underscore and hyphen")
		}
	}
	return nil
}
