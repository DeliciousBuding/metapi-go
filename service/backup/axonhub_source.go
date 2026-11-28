package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// AxonHubBackupVersion is the only AxonHub backup format this importer has
// audited. A newer format must be reviewed against the provider enum and the
// endpoint contract before it can be assumed to carry the same semantics.
const AxonHubBackupVersion = "1.4"

// AxonHubSource is the parsed, source-side view of an AxonHub backup.
//
// It is not an import plan and it is not a claim of compatibility: an AxonHub
// feature has a Metapi equivalent only after CompileAxonHubPlan says so.
// Sections this importer never executes are counted rather than modeled, so a
// payload cannot smuggle routing semantics through a field nobody reads.
type AxonHubSource struct {
	Version               string
	Timestamp             string
	SystemConfigs         []AxonHubSourceSystemConfig
	DeveloperAssociations map[string][]AxonHubSourceAssociation
	Projects              []AxonHubSourceProject
	Channels              []AxonHubSourceChannel
	Models                []AxonHubSourceModel
	ChannelModelPrices    []AxonHubSourceModelPrice
	APIKeys               []AxonHubSourceAPIKey
	// UsageRequests/UsageLogs are history, not configuration; only their row
	// counts are retained so a preview can report them honestly.
	UsageRequests       int
	UsageLogs           int
	QuotaUsage          []AxonHubQuotaUsage
	QuotaHistoryPresent bool
	// UnknownSections counts recognized top-level sections outside this
	// importer's contract.
	UnknownSections map[string]int
}

// ParseAxonHubSource parses the configuration-bearing sections of an AxonHub
// backup. Structural problems are errors; a well-formed feature this importer
// cannot execute is reported by the compiler, not here.
func ParseAxonHubSource(raw []byte) (*AxonHubSource, error) {
	if len(raw) > 64<<20 {
		return nil, axonHubErr("AxonHub backup exceeds 64 MiB")
	}
	if err := checkSourceJSONKeys(raw); err != nil {
		return nil, axonHubErr("invalid AxonHub backup JSON: ambiguous or malformed JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var top map[string]json.RawMessage
	if err := dec.Decode(&top); err != nil {
		return nil, axonHubErr("invalid AxonHub backup JSON: %s", safeJSONDecodeError(err))
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		return nil, axonHubErr("invalid AxonHub backup JSON: trailing JSON value")
	} else if err != io.EOF {
		return nil, axonHubErr("invalid AxonHub backup JSON: trailing data")
	}
	if top == nil {
		return nil, axonHubErr("invalid AxonHub backup JSON: expected an object")
	}
	if _, ok := top["channels"]; !ok {
		return nil, axonHubErr("invalid AxonHub backup: missing channels field")
	}
	if _, ok := top["models"]; !ok {
		return nil, axonHubErr("invalid AxonHub backup: missing models field")
	}

	src := &AxonHubSource{UnknownSections: map[string]int{}}
	var err error
	if src.Version, err = decodeAxonHubString(top["version"], "version"); err != nil {
		return nil, err
	}
	src.Version = strings.TrimSpace(src.Version)
	if src.Version == "" {
		return nil, axonHubErr("invalid AxonHub backup: missing version")
	}
	if src.Version != AxonHubBackupVersion {
		return nil, axonHubErr("unsupported AxonHub backup version (only %s is supported)", AxonHubBackupVersion)
	}
	if src.Timestamp, err = decodeAxonHubString(top["timestamp"], "timestamp"); err != nil {
		return nil, err
	}
	if src.Channels, err = decodeAxonHubChannels(top["channels"]); err != nil {
		return nil, err
	}
	if src.Models, err = decodeAxonHubModels(top["models"]); err != nil {
		return nil, err
	}
	if src.SystemConfigs, err = decodeAxonHubSystemConfigs(top["system_configs"]); err != nil {
		return nil, err
	}
	if src.DeveloperAssociations, err = decodeAxonHubDeveloperSettings(src.SystemConfigs); err != nil {
		return nil, err
	}
	if src.Projects, err = decodeAxonHubProjects(top["projects"]); err != nil {
		return nil, err
	}
	if src.ChannelModelPrices, err = decodeAxonHubPrices(top["channel_model_prices"]); err != nil {
		return nil, err
	}
	if src.APIKeys, err = decodeAxonHubAPIKeys(top["api_keys"]); err != nil {
		return nil, err
	}
	if src.UsageRequests, err = sectionRowCount(top["usage_requests"]); err != nil {
		return nil, axonHubErr("invalid AxonHub backup: usage_requests must be an array")
	}
	if src.QuotaUsage, src.QuotaHistoryPresent, err = decodeAxonHubQuotaUsage(top["usage_logs"]); err != nil {
		return nil, err
	}
	if src.UsageLogs, err = sectionRowCount(top["usage_logs"]); err != nil {
		return nil, axonHubErr("invalid AxonHub backup: usage_logs must be an array")
	}
	for _, name := range axonHubTopLevelSections {
		delete(top, name)
	}
	for name := range top {
		src.UnknownSections[sanitizeIdentifier(name)] = 1
	}
	if err := validateAxonHubSource(src); err != nil {
		return nil, err
	}
	return src, nil
}

var axonHubTopLevelSections = []string{
	"version", "timestamp", "channels", "models", "system_configs", "projects",
	"channel_model_prices", "api_keys", "usage_requests", "usage_logs",
}

func decodeAxonHubString(raw json.RawMessage, name string) (string, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", axonHubErr("invalid AxonHub backup: %s must be a string", name)
	}
	return value, nil
}

// sectionRowCount accepts an absent, null, or array section and returns its row
// count. Anything else is malformed.
func sectionRowCount(raw json.RawMessage) (int, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

func rawSectionRows(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, axonHubErr("expected an array section")
	}
	return rows, nil
}

// rowObject decodes one section row and rejects duplicate keys: a duplicate key
// is how a crafted payload hides a credential behind a later value.
func rowObject(row json.RawMessage) (map[string]json.RawMessage, error) {
	if err := checkSourceJSONKeys(row); err != nil {
		return nil, axonHubErr("ambiguous JSON object")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(row, &obj); err != nil || obj == nil {
		return nil, axonHubErr("expected an object")
	}
	return obj, nil
}
