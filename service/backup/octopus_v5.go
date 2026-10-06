package backup

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

const OctopusV5Origin = "octopus-v5"

func IsOctopusV5Payload(raw []byte) bool {
	var top map[string]json.RawMessage
	return json.Unmarshal(raw, &top) == nil && top != nil && bytes.Equal(bytes.TrimSpace(top["version"]), []byte("5")) && len(top["exported_at"]) > 0
}

const (
	OctopusProtocolChat      = 1 << 1
	OctopusProtocolResponses = 1 << 2
	OctopusProtocolAnthropic = 1 << 3
)

type OctopusV5Dump struct {
	Version     int               `json:"version"`
	ExportedAt  string            `json:"exported_at"`
	Channels    []octopusChannel  `json:"channels"`
	Credentials []octopusKey      `json:"channel_keys"`
	Models      []octopusModel    `json:"channel_models"`
	Grants      []octopusGrant    `json:"channel_grants"`
	Groups      []octopusGroup    `json:"groups"`
	GroupItems  []octopusItem     `json:"group_items"`
	APIKeys     []json.RawMessage `json:"api_keys"`
	LLMInfos    []json.RawMessage `json:"llm_infos"`
	Settings    []json.RawMessage `json:"settings"`
	StatsTotal  []json.RawMessage `json:"stats_total"`
	StatsDaily  []json.RawMessage `json:"stats_daily"`
	StatsHourly []json.RawMessage `json:"stats_hourly"`
	StatsAPIKey []json.RawMessage `json:"stats_api_key"`
}

type octopusSetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type octopusChannel struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	Dialect       string          `json:"dialect"`
	Enabled       bool            `json:"enabled"`
	BaseURL       string          `json:"base_url"`
	ChatPath      string          `json:"openai_chat_completion_path"`
	ResponsesPath string          `json:"openai_response_path"`
	AnthropicPath string          `json:"anthropic_message_path"`
	Proxy         bool            `json:"proxy"`
	ChannelProxy  string          `json:"channel_proxy"`
	CustomHeader  json.RawMessage `json:"custom_header"`
	ParamOverride string          `json:"param_override"`
	MatchRegex    string          `json:"match_regex"`
	OctopusStats
}
type OctopusStats struct {
	InputToken     int64   `json:"input_token"`
	OutputToken    int64   `json:"output_token"`
	InputCost      float64 `json:"input_cost"`
	OutputCost     float64 `json:"output_cost"`
	WaitTime       int64   `json:"wait_time"`
	RequestSuccess int64   `json:"request_success"`
	RequestFailed  int64   `json:"request_failed"`
}

func (s OctopusStats) nonZero() bool {
	return s.InputToken != 0 || s.OutputToken != 0 || s.InputCost != 0 || s.OutputCost != 0 || s.WaitTime != 0 || s.RequestSuccess != 0 || s.RequestFailed != 0
}

type octopusKey struct {
	ID        int64  `json:"id"`
	ChannelID int64  `json:"channel_id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Enabled   bool   `json:"enabled"`
	OctopusStats
}
type octopusModel struct {
	ID        int64  `json:"id"`
	ChannelID int64  `json:"channel_id"`
	Name      string `json:"name"`
	OctopusStats
}
type octopusGrant struct {
	ID             int64 `json:"id"`
	ChannelModelID int64 `json:"channel_model_id"`
	ChannelKeyID   int64 `json:"channel_key_id"`
	Protocols      int   `json:"protocols"`
	OctopusStats
}
type octopusGroup struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	Mode         string          `json:"mode"`
	ActiveItemID int64           `json:"active_item_id"`
	RelayConfig  json.RawMessage `json:"relay_config"`
}
type octopusItem struct {
	ID          int64  `json:"id"`
	GroupID     int64  `json:"group_id"`
	GrantID     int64  `json:"channel_grant_id"`
	Priority    int64  `json:"priority"`
	Weight      int64  `json:"weight"`
	ChannelID   int64  `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ModelName   string `json:"model_name"`
	KeyName     string `json:"key_name"`
	Protocols   int    `json:"protocols"`
	Available   bool   `json:"available"`
}

type OctopusV5Preview struct {
	Source      string         `json:"source"`
	OriginKey   string         `json:"originKey"`
	Sections    map[string]int `json:"sections"`
	NotImported map[string]int `json:"notImported,omitempty"`
	Adaptations []string       `json:"adaptations,omitempty"`
	Blocking    []string       `json:"blocking,omitempty"`
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
	itemIDs := map[int64]bool{}
	for _, i := range d.GroupItems {
		if i.ID <= 0 || groups[i.GroupID].ID == 0 || !grants[i.GrantID] || i.Priority < 0 || i.Weight < 0 || i.ChannelID != 0 || i.ChannelName != "" || i.ModelName != "" || i.KeyName != "" || i.Protocols != 0 || i.Available {
			return fmt.Errorf("Octopus group_items contain an invalid group/grant relationship")
		}
		if itemIDs[i.ID] {
			return fmt.Errorf("Octopus group_items contain a duplicate id")
		}
		itemIDs[i.ID] = true
	}
	for _, g := range d.Groups {
		if g.ActiveItemID != 0 && !itemIDs[g.ActiveItemID] {
			return fmt.Errorf("Octopus groups reference a missing active item")
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
		if hasUnsupportedClientHeaderTemplate(header.Value) {
			return true
		}
	}
	return false
}

func hasUnsupportedClientHeaderTemplate(value string) bool {
	for start := 0; ; {
		relative := strings.Index(strings.ToLower(value[start:]), "{client_header:")
		if relative < 0 {
			return false
		}
		nameStart := start + relative + len("{client_header:")
		endRel := strings.IndexByte(value[nameStart:], '}')
		if endRel < 0 {
			return true
		}
		name := strings.TrimSpace(value[nameStart : nameStart+endRel])
		if !validClientHeaderName(name) {
			return true
		}
		start = nameStart + endRel + 1
	}
}

func validClientHeaderName(name string) bool {
	if name == "" {
		return false
	}
	switch strings.ToLower(name) {
	case "idempotency-key", "openai-beta", "x-request-id", "x-correlation-id", "traceparent", "tracestate":
		return true
	default:
		return false
	}
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

// ImportOctopusV5 remaps every source PK through external_source_ids inside a
// single transaction. Re-imports upsert the same source graph by origin key.
func ImportOctopusV5(db *store.DB, raw []byte, originKey string) (map[string]int64, error) {
	return ImportOctopusV5WithUnsupportedMode(db, raw, originKey, false)
}

// ImportOctopusV5WithUnsupportedMode imports the executable channel graph.
// When allowUnsupported is true, unsupported Octopus-only sections stay in
// the user's source file and are never activated; recognized default relay
// policies are normalized away after the caller acknowledges Metapi routing.
func ImportOctopusV5WithUnsupportedMode(db *store.DB, raw []byte, originKey string, allowUnsupported bool) (map[string]int64, error) {
	d, err := ParseOctopusV5(raw)
	if err != nil {
		return nil, err
	}
	if err = validateOriginKey(originKey); err != nil {
		return nil, err
	}
	if octopusHasBlockingSections(d) {
		return nil, errors.New("Octopus v5 contains unsupported client_header templates or group relay settings")
	}
	if (len(unsupportedOctopusV5Sections(d)) > 0 || octopusHasPolicyAdaptations(d)) && !allowUnsupported {
		unsupported := unsupportedOctopusV5Sections(d)
		if len(unsupported) == 0 {
			return nil, errors.New("Octopus v5 requires explicit channels-only acknowledgement for group relay policy adaptations")
		}
		return nil, fmt.Errorf("Octopus v5 backup contains unsupported non-empty sections: %s", strings.Join(sortedSectionNames(unsupported), ", "))
	}
	if db == nil {
		return nil, errors.New("database is unavailable")
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("begin Octopus import: %w", err)
	}
	defer tx.Rollback()
	statsRecords, err := persistOctopusStats(db, tx, originKey, d)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{"statsRecords": statsRecords}
	proxyURL, _, _ := octopusProxyURL(d)
	ids := map[string]map[int64]int64{}
	for _, k := range []string{"channels", "channel_keys", "channel_models", "channel_grants", "groups", "group_items"} {
		ids[k] = map[int64]int64{}
	}
	for _, c := range d.Channels {
		custom := string(c.CustomHeader)
		if custom == "" || custom == "null" {
			custom = "[]"
		}
		channelProxy := strings.TrimSpace(c.ChannelProxy)
		if !c.Proxy {
			channelProxy = ""
		} else if channelProxy == "" {
			channelProxy = proxyURL
		}
		id, e := upsertMapped(db, tx, originKey, "channels", c.ID, "upstream_channels", []string{"name", "dialect", "enabled", "base_url", "openai_chat_completion_path", "openai_response_path", "anthropic_message_path", "proxy", "channel_proxy", "custom_header", "param_override", "match_regex"}, []any{c.Name, c.Dialect, c.Enabled, c.BaseURL, defaultPath(c.ChatPath, "/v1/chat/completions"), defaultPath(c.ResponsesPath, "/v1/responses"), defaultPath(c.AnthropicPath, "/v1/messages"), c.Proxy, channelProxy, custom, c.ParamOverride, c.MatchRegex})
		if e != nil {
			return nil, e
		}
		ids["channels"][c.ID] = id
		counts["channels"]++
	}
	for _, k := range d.Credentials {
		id, e := upsertMapped(db, tx, originKey, "channel_keys", k.ID, "upstream_credentials", []string{"channel_id", "name", "secret", "enabled"}, []any{ids["channels"][k.ChannelID], k.Name, k.Key, k.Enabled})
		if e != nil {
			return nil, e
		}
		ids["channel_keys"][k.ID] = id
		counts["channelKeys"]++
	}
	for _, m := range d.Models {
		id, e := upsertMapped(db, tx, originKey, "channel_models", m.ID, "upstream_models", []string{"channel_id", "name", "enabled"}, []any{ids["channels"][m.ChannelID], m.Name, true})
		if e != nil {
			return nil, e
		}
		ids["channel_models"][m.ID] = id
		counts["channelModels"]++
	}
	for _, g := range d.Grants {
		id, e := upsertMapped(db, tx, originKey, "channel_grants", g.ID, "upstream_grants", []string{"model_id", "credential_id", "protocols", "enabled"}, []any{ids["channel_models"][g.ChannelModelID], ids["channel_keys"][g.ChannelKeyID], g.Protocols, true})
		if e != nil {
			return nil, e
		}
		ids["channel_grants"][g.ID] = id
		counts["channelGrants"]++
	}
	for _, g := range d.Groups {
		relay := string(g.RelayConfig)
		if relay == "" || relay == "null" {
			relay = "{}"
		}
		if groupRelayConfigNeedsAdaptation(g.RelayConfig) {
			// Avoid retaining a source runtime policy that Metapi will not execute.
			relay = "{}"
		}
		id, e := upsertMapped(db, tx, originKey, "groups", g.ID, "upstream_groups", []string{"name", "mode", "active_item_id", "relay_config", "enabled"}, []any{g.Name, g.Mode, int64(0), relay, true})
		if e != nil {
			return nil, e
		}
		ids["groups"][g.ID] = id
		counts["groups"]++
		routeID, routeErr := upsertOctopusRoute(db, tx, originKey, g.ID, g.Name)
		if routeErr != nil {
			return nil, routeErr
		}
		if _, routeErr = tx.Exec(db.Rebind(`INSERT INTO upstream_route_groups (route_id, group_id) VALUES (?, ?) ON CONFLICT(route_id) DO UPDATE SET group_id=excluded.group_id`), routeID, id); routeErr != nil {
			return nil, fmt.Errorf("link imported group to model route: %w", routeErr)
		}
	}
	for _, i := range d.GroupItems {
		id, e := upsertMapped(db, tx, originKey, "group_items", i.ID, "upstream_group_items", []string{"group_id", "grant_id", "priority", "weight"}, []any{ids["groups"][i.GroupID], ids["channel_grants"][i.GrantID], i.Priority, i.Weight})
		if e != nil {
			return nil, e
		}
		ids["group_items"][i.ID] = id
		counts["groupItems"]++
	}
	for _, g := range d.Groups {
		if g.ActiveItemID == 0 {
			continue
		}
		targetGroup := ids["groups"][g.ID]
		targetItem := ids["group_items"][g.ActiveItemID]
		if targetItem == 0 {
			return nil, fmt.Errorf("Octopus group %d active item mapping is missing", g.ID)
		}
		if _, e := tx.Exec(db.Rebind("UPDATE upstream_groups SET active_item_id = ? WHERE id = ?"), targetItem, targetGroup); e != nil {
			return nil, fmt.Errorf("update imported group selection: %w", e)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit Octopus import: %w", err)
	}
	return counts, nil
}

func octopusHasBlockingSections(d *OctopusV5Dump) bool {
	for _, channel := range d.Channels {
		if channelHasClientHeaderTemplate(channel.CustomHeader) {
			return true
		}
	}
	for _, group := range d.Groups {
		if groupRelayConfigBlocking(group.RelayConfig) {
			return true
		}
	}
	for _, channel := range d.Channels {
		if channel.Proxy && strings.TrimSpace(channel.ChannelProxy) == "" {
			globalProxy, _, _ := octopusProxyURL(d)
			if globalProxy == "" {
				return true
			}
		}
	}
	return false
}

func octopusHasPolicyAdaptations(d *OctopusV5Dump) bool {
	for _, group := range d.Groups {
		if groupRelayConfigNeedsAdaptation(group.RelayConfig) {
			return true
		}
	}
	return false
}

func persistOctopusStats(db *store.DB, tx *sqlx.Tx, origin string, d *OctopusV5Dump) (int64, error) {
	if _, err := tx.Exec(db.Rebind(`DELETE FROM upstream_import_stats WHERE origin_key = ?`), origin); err != nil {
		return 0, fmt.Errorf("replace imported Octopus statistics: %w", err)
	}
	type statRow struct {
		section string
		key     string
		data    json.RawMessage
	}
	rows := make([]statRow, 0)
	appendInline := func(section string, sourceID int64, stats OctopusStats) error {
		if !stats.nonZero() {
			return nil
		}
		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		rows = append(rows, statRow{section: section, key: strconv.FormatInt(sourceID, 10), data: data})
		return nil
	}
	for _, row := range d.Channels {
		if err := appendInline("channel", row.ID, row.OctopusStats); err != nil {
			return 0, err
		}
	}
	for _, row := range d.Credentials {
		if err := appendInline("channel_key", row.ID, row.OctopusStats); err != nil {
			return 0, err
		}
	}
	for _, row := range d.Models {
		if err := appendInline("channel_model", row.ID, row.OctopusStats); err != nil {
			return 0, err
		}
	}
	for _, row := range d.Grants {
		if err := appendInline("channel_grant", row.ID, row.OctopusStats); err != nil {
			return 0, err
		}
	}
	for _, section := range []struct {
		name string
		data []json.RawMessage
	}{
		{"stats_total", d.StatsTotal}, {"stats_daily", d.StatsDaily}, {"stats_hourly", d.StatsHourly}, {"stats_api_key", d.StatsAPIKey},
	} {
		for i, data := range section.data {
			rows = append(rows, statRow{section: section.name, key: "row:" + strconv.Itoa(i), data: data})
		}
	}
	for _, row := range rows {
		if _, err := tx.Exec(db.Rebind(`INSERT INTO upstream_import_stats (origin_key, section, record_key, data_json) VALUES (?, ?, ?, ?) ON CONFLICT(origin_key, section, record_key) DO UPDATE SET data_json=excluded.data_json`), origin, row.section, row.key, string(row.data)); err != nil {
			return 0, fmt.Errorf("preserve Octopus statistics: %w", err)
		}
	}
	return int64(len(rows)), nil
}

func upsertOctopusRoute(db *store.DB, tx *sqlx.Tx, origin string, sourceID int64, name string) (int64, error) {
	if !routing.IsExactRouteModelPattern(name) {
		return 0, fmt.Errorf("Octopus group name %q contains model-pattern syntax and cannot be imported as an exact route", name)
	}
	var routeID int64
	err := tx.Get(&routeID, db.Rebind(`SELECT target_id FROM external_source_ids WHERE origin_key=? AND entity_type=? AND source_id=?`), origin, "token_routes", sourceID)
	if err == nil {
		var pattern string
		if err = tx.Get(&pattern, db.Rebind(`SELECT model_pattern FROM token_routes WHERE id=?`), routeID); err != nil {
			return 0, fmt.Errorf("load imported model route: %w", err)
		}
		if pattern != name {
			return 0, fmt.Errorf("imported group route name changed; source mapping needs review")
		}
		return routeID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("lookup imported model route mapping: %w", err)
	}
	var conflicts int
	if err = tx.Get(&conflicts, db.Rebind(`SELECT COUNT(*) FROM token_routes WHERE model_pattern=? AND enabled=?`), name, true); err != nil {
		return 0, err
	}
	if conflicts > 0 {
		return 0, fmt.Errorf("model route %q already exists and cannot be claimed by an Octopus import", name)
	}
	if err = tx.Get(&routeID, db.Rebind(`INSERT INTO token_routes (model_pattern,route_mode,routing_strategy,enabled,created_at,updated_at) VALUES (?, 'pattern','weighted',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) RETURNING id`), name, true); err != nil {
		return 0, fmt.Errorf("create imported model route: %w", err)
	}
	if _, err = tx.Exec(db.Rebind(`INSERT INTO external_source_ids (origin_key,entity_type,source_id,target_id) VALUES (?,?,?,?)`), origin, "token_routes", sourceID, routeID); err != nil {
		return 0, fmt.Errorf("save imported model route mapping: %w", err)
	}
	return routeID, nil
}

func defaultPath(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func upsertMapped(db *store.DB, tx *sqlx.Tx, origin, entity string, sourceID int64, table string, columns []string, values []any) (int64, error) {
	var id int64
	err := tx.Get(&id, db.Rebind("SELECT target_id FROM external_source_ids WHERE origin_key = ? AND entity_type = ? AND source_id = ?"), origin, entity, sourceID)
	if err == nil {
		sets := make([]string, len(columns))
		for i, c := range columns {
			sets[i] = c + " = ?"
		}
		args := append(append([]any{}, values...), id)
		if _, err = tx.Exec(db.Rebind("UPDATE "+table+" SET "+strings.Join(sets, ",")+" WHERE id = ?"), args...); err != nil {
			return 0, fmt.Errorf("update imported %s: %w", entity, err)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("lookup imported %s mapping: %w", entity, err)
	}
	cols := append([]string{"origin_key", "source_id"}, columns...)
	marks := strings.TrimRight(strings.Repeat("?,", len(cols)), ",")
	args := append([]any{origin, sourceID}, values...)
	q := "INSERT INTO " + table + " (" + strings.Join(cols, ",") + ") VALUES (" + marks + ") RETURNING id"
	if err = tx.Get(&id, db.Rebind(q), args...); err != nil {
		return 0, fmt.Errorf("insert imported %s: %w", entity, err)
	}
	if _, err = tx.Exec(db.Rebind("INSERT INTO external_source_ids (origin_key,entity_type,source_id,target_id) VALUES (?,?,?,?)"), origin, entity, sourceID, id); err != nil {
		return 0, fmt.Errorf("save imported %s mapping: %w", entity, err)
	}
	return id, nil
}

// SortOctopusPreviewWarnings makes response output deterministic.
