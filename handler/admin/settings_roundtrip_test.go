package admin

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestSettingsRuntimeWritePathSurvivesRestart is the round-trip half of the
// settings persistence contract: every value the admin API accepts must still
// be in effect after a restart. The write side persists through
// upsertSettingDB (JSON-encoded), the boot side reads through
// store.ApplyRuntimeSettings, and the two halves used to disagree — a cleared
// site name came back as the env value, and the routing/proxy/debug knobs were
// not read back at all, so a restart silently reverted them.
//
// The key-coverage half of the contract (no persisted key may lack a hydration
// case) is enforced by store/settings_rehydration_gate_test.go.
func TestSettingsRuntimeWritePathSurvivesRestart(t *testing.T) {
	db, r, _ := setupEdgeTest(t)

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		// Access control + failure judgement.
		"adminIpAllowlist":   []any{"203.0.113.7", "198.51.100.9"},
		"proxyErrorKeywords": []any{"rate limit", "overloaded"},
		// Routing cost model.
		"routingWeights":          map[string]any{"baseWeightFactor": 0.7, "costWeight": 0.2},
		"routingFallbackUnitCost": 0.25,
		// Proxy transport + token router.
		"proxyFirstByteTimeoutSec":            45,
		"tokenRouterFailureCooldownMaxSec":    3600,
		"proxySessionChannelConcurrencyLimit": 4,
		"proxySessionChannelQueueWaitMs":      2500,
		// Fallback policy toggles.
		"disableCrossProtocolFallback":               true,
		"responsesCompactFallbackToResponsesEnabled": true,
		"proxyEmptyContentFailEnabled":               true,
		// Debug tracing.
		"proxyDebugCaptureHeaders":      false,
		"proxyDebugCaptureBodies":       true,
		"proxyDebugCaptureStreamChunks": true,
		"proxyDebugTargetSessionId":     "sess-round-trip",
		"proxyDebugTargetClientKind":    "codex",
		"proxyDebugTargetModel":         "gpt-5",
		"proxyDebugRetentionHours":      12,
		"proxyDebugMaxBodyBytes":        4096,
		// Log cleanup: the write side uses underscore keys, the boot side used
		// to look for dotted ones.
		"logCleanupUsageLogsEnabled":   true,
		"logCleanupProgramLogsEnabled": false,
		"logCleanupRetentionDays":      9,
		// Branding cleared on purpose: the restart env below sets all five, so
		// a hydration path that drops explicit blanks resurrects them.
		"systemName":    "",
		"logo":          "",
		"footer":        "",
		"about":         "",
		"serverAddress": "",
		// Credential: persisted JSON-encoded, must be decoded on the way back.
		"proxyToken": "sk-round-trip-token",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings/runtime: %d %s", resp.Code, resp.Body.String())
	}
	hot := *config.Runtime()

	// Simulated restart: env resolves the deployment defaults (deliberately
	// different from everything the operator just saved), then boot hydration
	// reads back exactly the rows the write path persisted.
	persisted := readAllPersistedSettings(t, db)
	if len(persisted) == 0 {
		t.Fatal("write path persisted no settings rows")
	}
	_, restarted := config.Load(map[string]string{
		"ACCOUNT_CREDENTIAL_SECRET":        "restart-test-secret",
		"PROXY_TOKEN":                      "sk-env-token",
		"SYSTEM_NAME":                      "Metapi",
		"LOGO":                             "https://cdn.example/env-logo.png",
		"FOOTER":                           "env footer",
		"ABOUT":                            "env about",
		"SERVER_ADDRESS":                   "https://env.example.com",
		"ADMIN_IP_ALLOWLIST":               "192.0.2.1",
		"PROXY_ERROR_KEYWORDS":             "env keyword",
		"LOG_CLEANUP_USAGE_LOGS_ENABLED":   "false",
		"LOG_CLEANUP_PROGRAM_LOGS_ENABLED": "true",
		"LOG_CLEANUP_RETENTION_DAYS":       "30",
	})
	store.ApplyRuntimeSettings(&config.Config{}, restarted, persisted)

	checks := []struct {
		name string
		get  func(*config.RuntimeSettings) any
	}{
		{"AdminIpAllowlist", func(rt *config.RuntimeSettings) any { return rt.AdminIpAllowlist }},
		{"ProxyErrorKeywords", func(rt *config.RuntimeSettings) any { return rt.ProxyErrorKeywords }},
		{"RoutingWeights", func(rt *config.RuntimeSettings) any { return rt.RoutingWeights }},
		{"RoutingFallbackUnitCost", func(rt *config.RuntimeSettings) any { return rt.RoutingFallbackUnitCost }},
		{"ProxyFirstByteTimeoutSec", func(rt *config.RuntimeSettings) any { return rt.ProxyFirstByteTimeoutSec }},
		{"TokenRouterFailureCooldownMaxSec", func(rt *config.RuntimeSettings) any { return rt.TokenRouterFailureCooldownMaxSec }},
		{"ProxySessionChannelConcurrencyLimit", func(rt *config.RuntimeSettings) any { return rt.ProxySessionChannelConcurrencyLimit }},
		{"ProxySessionChannelQueueWaitMs", func(rt *config.RuntimeSettings) any { return rt.ProxySessionChannelQueueWaitMs }},
		{"DisableCrossProtocolFallback", func(rt *config.RuntimeSettings) any { return rt.DisableCrossProtocolFallback }},
		{"ResponsesCompactFallbackToResponsesEnabled", func(rt *config.RuntimeSettings) any { return rt.ResponsesCompactFallbackToResponsesEnabled }},
		{"ProxyEmptyContentFailEnabled", func(rt *config.RuntimeSettings) any { return rt.ProxyEmptyContentFailEnabled }},
		{"ProxyDebugCaptureHeaders", func(rt *config.RuntimeSettings) any { return rt.ProxyDebugCaptureHeaders }},
		{"ProxyDebugCaptureBodies", func(rt *config.RuntimeSettings) any { return rt.ProxyDebugCaptureBodies }},
		{"ProxyDebugCaptureStreamChunks", func(rt *config.RuntimeSettings) any { return rt.ProxyDebugCaptureStreamChunks }},
		{"ProxyDebugTargetSessionId", func(rt *config.RuntimeSettings) any { return rt.ProxyDebugTargetSessionId }},
		{"ProxyDebugTargetClientKind", func(rt *config.RuntimeSettings) any { return rt.ProxyDebugTargetClientKind }},
		{"ProxyDebugTargetModel", func(rt *config.RuntimeSettings) any { return rt.ProxyDebugTargetModel }},
		{"ProxyDebugRetentionHours", func(rt *config.RuntimeSettings) any { return rt.ProxyDebugRetentionHours }},
		{"ProxyDebugMaxBodyBytes", func(rt *config.RuntimeSettings) any { return rt.ProxyDebugMaxBodyBytes }},
		{"LogCleanupUsageLogsEnabled", func(rt *config.RuntimeSettings) any { return rt.LogCleanupUsageLogsEnabled }},
		{"LogCleanupProgramLogsEnabled", func(rt *config.RuntimeSettings) any { return rt.LogCleanupProgramLogsEnabled }},
		{"LogCleanupRetentionDays", func(rt *config.RuntimeSettings) any { return rt.LogCleanupRetentionDays }},
		{"SystemName", func(rt *config.RuntimeSettings) any { return rt.SystemName }},
		{"Logo", func(rt *config.RuntimeSettings) any { return rt.Logo }},
		{"Footer", func(rt *config.RuntimeSettings) any { return rt.Footer }},
		{"About", func(rt *config.RuntimeSettings) any { return rt.About }},
		{"ServerAddress", func(rt *config.RuntimeSettings) any { return rt.ServerAddress }},
		{"ProxyToken", func(rt *config.RuntimeSettings) any { return rt.ProxyToken }},
	}
	for _, check := range checks {
		want, got := check.get(&hot), check.get(restarted)
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s did not survive the restart: saved %#v, rehydrated %#v", check.name, want, got)
		}
	}

	// Guard against a vacuous pass: the restart env above must actually
	// disagree with what the operator saved, otherwise the comparison proves
	// nothing.
	if restarted.SystemName != "" {
		t.Fatalf("SystemName = %q, want the cleared value (env said Metapi)", restarted.SystemName)
	}
	if restarted.ProxyToken != "sk-round-trip-token" {
		t.Fatalf("ProxyToken = %q, want the saved token (env said sk-env-token)", restarted.ProxyToken)
	}
	if len(restarted.AdminIpAllowlist) != 2 {
		t.Fatalf("AdminIpAllowlist = %#v, want the two saved entries (env said 192.0.2.1)", restarted.AdminIpAllowlist)
	}
}

// readAllPersistedSettings returns the settings table exactly as boot
// hydration sees it (raw JSON-encoded values keyed by setting name).
func readAllPersistedSettings(t *testing.T, db *store.DB) map[string]string {
	t.Helper()
	rows, err := db.Query("SELECT key, value FROM settings")
	if err != nil {
		t.Fatalf("query settings: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatalf("scan setting: %v", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("settings rows: %v", err)
	}
	return out
}
