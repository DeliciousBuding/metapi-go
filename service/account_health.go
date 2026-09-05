package service

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// RuntimeHealthState is the health state of an account at runtime.
type RuntimeHealthState string

const (
	HealthHealthy   RuntimeHealthState = "healthy"
	HealthDegraded  RuntimeHealthState = "degraded"
	HealthUnhealthy RuntimeHealthState = "unhealthy"
	HealthDisabled  RuntimeHealthState = "disabled"
	HealthUnknown   RuntimeHealthState = "unknown"
)

// RuntimeHealthSource is the source of a runtime health update.
type RuntimeHealthSource string

const (
	HealthSourceCheckin RuntimeHealthSource = "checkin"
	HealthSourceBalance RuntimeHealthSource = "balance"
	HealthSourceAuth    RuntimeHealthSource = "auth"
	HealthSourceSystem  RuntimeHealthSource = "system"
	HealthSourceNone    RuntimeHealthSource = "none"
	HealthSourceModel   RuntimeHealthSource = "model-discovery"
)

// RuntimeHealthEntry is a single runtime health record stored in extraConfig.
type RuntimeHealthEntry struct {
	State     RuntimeHealthState  `json:"state"`
	Reason    string              `json:"reason"`
	Source    RuntimeHealthSource `json:"source"`
	CheckedAt *string             `json:"checkedAt,omitempty"`
}

// RuntimeHealthInput is the input for effective runtime-health aggregation.
// Mirrors TS buildRuntimeHealthForAccount().
type RuntimeHealthInput struct {
	AccountStatus       string
	SiteStatus          string
	ExtraConfig         *string
	SessionCapable      *bool
	HasDiscoveredModels bool
	OAuthProvider       *string
}

// SetAccountRuntimeHealth writes runtime health state into the account's extraConfig.
// Mirrors TS setAccountRuntimeHealth().
func SetAccountRuntimeHealth(db *sqlx.DB, accountID int64, entry RuntimeHealthEntry) error {
	// Fetch current extraConfig
	var extraConfig *string
	err := db.Get(&extraConfig, db.Rebind("SELECT extra_config FROM accounts WHERE id = ?"), accountID)
	if err != nil {
		return err
	}

	if entry.CheckedAt == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		entry.CheckedAt = &now
	}

	healthJSON, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	healthMap := map[string]any{}
	_ = json.Unmarshal(healthJSON, &healthMap)

	newConfig := MergeExtraConfig(extraConfig, map[string]any{
		"runtimeHealth": healthMap,
	})
	if newConfig == nil {
		// Keep empty config as null
		_, err = db.Exec(db.Rebind("UPDATE accounts SET extra_config = NULL, updated_at = ? WHERE id = ?"),
			time.Now().UTC().Format(time.RFC3339), accountID)
	} else {
		_, err = db.Exec(db.Rebind("UPDATE accounts SET extra_config = ?, updated_at = ? WHERE id = ?"),
			*newConfig, time.Now().UTC().Format(time.RFC3339), accountID)
	}
	return err
}

// ExtractRuntimeHealth reads the runtime health from an account's extraConfig.
// Mirrors TS extractRuntimeHealth().
func ExtractRuntimeHealth(extraConfig *string) *RuntimeHealthEntry {
	cfg := ParseExtraConfig(extraConfig)
	if cfg == nil {
		return nil
	}
	healthRaw, ok := cfg["runtimeHealth"]
	if !ok {
		return nil
	}
	// Re-marshal and unmarshal for type safety
	b, err := json.Marshal(healthRaw)
	if err != nil {
		return nil
	}
	var entry RuntimeHealthEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil
	}
	if entry.State == "" {
		return nil
	}
	entry.State = normalizeRuntimeHealthState(string(entry.State))
	if entry.State == "" {
		return nil
	}
	if entry.Reason == "" {
		entry.Reason = defaultHealthReason(entry.State)
	}
	if entry.Source == "" {
		entry.Source = "unknown"
	}
	return &entry
}

// IsUnsupportedCheckinRuntimeHealth checks if the runtime health state represents
// an unsupported checkin degraded state that should be preserved during balance refresh.
// Mirrors TS isUnsupportedCheckinRuntimeHealth().
func IsUnsupportedCheckinRuntimeHealth(health *RuntimeHealthEntry) bool {
	if health == nil || health.State != HealthDegraded {
		return false
	}
	if strings.ToLower(string(health.Source)) == "checkin" {
		return true
	}
	// Reason-only fallback for entries written before the source was recorded.
	// The vocabulary lives in platform so this cannot drift from what the
	// check-in runner and the failure classifier decide.
	return platform.IsUnsupportedCheckinMessage(health.Reason)
}

// BuildRuntimeHealthForAccount returns the effective runtime health for admin list/detail.
// Expired account status always reports unhealthy (auth), never healthy — expired status.
// Mirrors TS buildRuntimeHealthForAccount().
func BuildRuntimeHealthForAccount(input RuntimeHealthInput) RuntimeHealthEntry {
	accountStatus := strings.ToLower(strings.TrimSpace(input.AccountStatus))
	if accountStatus == "" {
		accountStatus = "active"
	}
	siteStatus := strings.ToLower(strings.TrimSpace(input.SiteStatus))
	if siteStatus == "" {
		siteStatus = "active"
	}

	if accountStatus == "disabled" || siteStatus == "disabled" {
		return RuntimeHealthEntry{
			State:  HealthDisabled,
			Reason: defaultHealthReason(HealthDisabled),
			Source: HealthSourceSystem,
		}
	}

	if accountStatus == "expired" {
		return RuntimeHealthEntry{
			State:  HealthUnhealthy,
			Reason: expiredHealthReason(input.ExtraConfig, input.OAuthProvider),
			Source: HealthSourceAuth,
		}
	}

	stored := ExtractRuntimeHealth(input.ExtraConfig)
	if isProxyOnlyAuthFailure(stored, input.SessionCapable) {
		stored = nil
	}
	if stored != nil {
		if input.SessionCapable != nil && !*input.SessionCapable && input.HasDiscoveredModels && stored.State == HealthUnknown {
			// Fall through to model-discovery healthy below.
		} else {
			return *stored
		}
	}

	if input.SessionCapable != nil && !*input.SessionCapable && input.HasDiscoveredModels {
		var checkedAt *string
		if stored != nil {
			checkedAt = stored.CheckedAt
		}
		return RuntimeHealthEntry{
			State:     HealthHealthy,
			Reason:    "Model probe succeeded",
			Source:    HealthSourceModel,
			CheckedAt: checkedAt,
		}
	}

	return RuntimeHealthEntry{
		State:  HealthUnknown,
		Reason: defaultHealthReason(HealthUnknown),
		Source: HealthSourceNone,
	}
}

func normalizeRuntimeHealthState(value string) RuntimeHealthState {
	switch RuntimeHealthState(strings.ToLower(strings.TrimSpace(value))) {
	case HealthHealthy, HealthUnhealthy, HealthDegraded, HealthUnknown, HealthDisabled:
		return RuntimeHealthState(strings.ToLower(strings.TrimSpace(value)))
	default:
		return ""
	}
}

func defaultHealthReason(state RuntimeHealthState) string {
	switch state {
	case HealthHealthy:
		return "Running normally"
	case HealthUnhealthy:
		return "Last check failed"
	case HealthDegraded:
		return "Performance fluctuating"
	case HealthDisabled:
		return "Account or site disabled"
	case HealthUnknown:
		fallthrough
	default:
		return "Not checked yet"
	}
}

func expiredHealthReason(extraConfig *string, oauthProvider *string) string {
	if hasOauthProvider(oauthProvider, extraConfig) {
		return "Credentials expired; update the credentials"
	}
	mode := GetCredentialModeFromExtraConfig(extraConfig)
	switch mode {
	case CredentialModeAPIKey:
		return "Connection expired; update the API key"
	case CredentialModeSession:
		return "Access token expired"
	default:
		return "Credentials expired; update the credentials"
	}
}

func isProxyOnlyAuthFailure(health *RuntimeHealthEntry, sessionCapable *bool) bool {
	if health == nil || sessionCapable == nil || *sessionCapable {
		return false
	}
	if health.State != HealthUnhealthy {
		return false
	}
	return strings.ToLower(string(health.Source)) == string(HealthSourceAuth)
}

func hasOauthProvider(oauthProvider *string, extraConfig *string) bool {
	if oauthProvider != nil && strings.TrimSpace(*oauthProvider) != "" {
		return true
	}
	cfg := ParseExtraConfig(extraConfig)
	if cfg == nil {
		return false
	}
	oauthRaw, ok := cfg["oauth"]
	if !ok {
		return false
	}
	oauthMap, ok := oauthRaw.(map[string]any)
	if !ok {
		return false
	}
	provider, _ := oauthMap["provider"].(string)
	return strings.TrimSpace(provider) != ""
}

// Ensure store.Account is compatible
var _ = (*store.Account)(nil)
