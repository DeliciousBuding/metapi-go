package backup

import "encoding/json"

const OctopusV5Origin = "octopus-v5"

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
	Removals    map[string]int `json:"removals,omitempty"`
	Source      string         `json:"source"`
	OriginKey   string         `json:"originKey"`
	Sections    map[string]int `json:"sections"`
	NotImported map[string]int `json:"notImported,omitempty"`
	Adaptations []string       `json:"adaptations,omitempty"`
	Blocking    []string       `json:"blocking,omitempty"`
}
