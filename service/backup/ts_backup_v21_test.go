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
