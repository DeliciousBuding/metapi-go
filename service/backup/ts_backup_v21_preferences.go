package backup

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RuntimeLocalSettingKeys are settings that a backup import must never set or
// overwrite. Both import paths (the tables-path import in handler/admin and
// the TS v2.1 parser in this package) share this policy.
//
// Policy: deny-list, not allow-list. The settings table holds ~90 feature keys
// scattered across handler/admin, store and the schedulers, and new benign
// keys are added with most features; an allow-list would have to track every
// addition or imports would silently drop legitimate settings. The dangerous
// class — credentials, deployment-bound wiring, and state that background
// schedulers consume automatically at startup — is small and stable, so we
// deny it explicitly instead.
//
// Threat model: a backup can come from an untrusted source. A malicious
// import that could plant backup_webdav_config_v1 would turn the backup-webdav
// scheduler into a periodic exfiltration job shipping the whole database to
// an attacker endpoint after the next restart, and a planted
// monitor_ldoh_cookie pins the monitor session to an attacker credential.
//
// MAINTAINERS: any new setting that stores a credential, session token or
// encryption key, wires the database, or makes the server push data to a
// configured endpoint on its own MUST be added here.
var RuntimeLocalSettingKeys = map[string]bool{
	// Master key encrypting stored account credentials: bound to the rows
	// already in this deployment; importing it breaks their decryption and
	// lets the backup's author read every credential stored afterwards.
	"account_credential_secret": true,
	// Admin API bearer token.
	"auth_token": true,
	// WebDAV endpoint + credentials; read by the backup-webdav scheduler at
	// startup (see scheduler/backup_webdav.go). Planting it exfiltrates the
	// full database to an attacker-controlled endpoint.
	"backup_webdav_config_v1": true,
	// WebDAV sync bookkeeping; deployment-local state.
	"backup_webdav_state_v1": true,
	// Database wiring.
	"db_ssl":  true,
	"db_type": true,
	"db_url":  true,
	// Plaintext LDOH upstream session cookie (handler/admin/monitor.go);
	// planting it pins the monitor session to an attacker value.
	"monitor_ldoh_cookie": true,
	// Downstream proxy API bearer token; planting it hands the attacker a
	// known working credential for the proxy surface.
	"proxy_token": true,
	// Egress destinations and notification credentials. A planted import turns
	// the gateway into a sender to an attacker endpoint (upstream traffic via a
	// proxy, alerts via a bot/webhook/SMTP account) without any further
	// operator action.
	"system_proxy_url":          true,
	"telegram_api_base_url":     true,
	"telegram_bot_token":        true,
	"telegram_chat_id":          true,
	"telegram_use_system_proxy": true,
	"webhook_enabled":           true,
	"webhook_url":               true,
	"bark_enabled":              true,
	"bark_url":                  true,
	"serverchan_enabled":        true,
	"serverchan_key":            true,
	"feishu_enabled":            true,
	"feishu_webhook":            true,
	"feishu_secret":             true,
	"dingtalk_enabled":          true,
	"dingtalk_webhook":          true,
	"dingtalk_secret":           true,
	"wecom_enabled":             true,
	"wecom_webhook":             true,
	"ntfy_enabled":              true,
	"ntfy_url":                  true,
	"ntfy_token":                true,
	"smtp_enabled":              true,
	"smtp_host":                 true,
	"smtp_port":                 true,
	"smtp_user":                 true,
	"smtp_pass":                 true,
	"smtp_from":                 true,
	"smtp_to":                   true,
	// Admin network ACL; importing it can widen the management surface.
	"admin_ip_allowlist": true,
	// Log retention: planting a short window deletes audit/usage evidence on
	// the next cleanup run.
	"log_cleanup_enabled":              true,
	"log_cleanup_cron":                 true,
	"log_cleanup_schedule_v2":          true,
	"log_cleanup_usage_logs_enabled":   true,
	"log_cleanup_program_logs_enabled": true,
	"log_cleanup_retention_days":       true,
}

func parseTSV21PreferencesSectionInto(raw json.RawMessage, parsed *TSV21Parsed) error {
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil || section == nil {
		return TSV21ClientError{Message: "invalid import data: preferences must be a JSON object"}
	}
	for key := range section {
		if key != "settings" {
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown section preferences.%s", key))
		}
	}

	rawSettings, present := section["settings"]
	if !present || isRawJSONNull(rawSettings) {
		return TSV21ClientError{Message: "invalid import data: preferences.settings must be an array"}
	}
	var rawRows []json.RawMessage
	if err := json.Unmarshal(rawSettings, &rawRows); err != nil {
		return TSV21ClientError{Message: "invalid import data: preferences.settings must be an array"}
	}
	if TSV21MaxRowsPerTable > 0 && len(rawRows) > TSV21MaxRowsPerTable {
		return TSV21ClientError{Message: fmt.Sprintf("import failed: preferences.settings exceeds the max rows of %d", TSV21MaxRowsPerTable)}
	}

	rows := make([]map[string]any, 0, len(rawRows))
	for i, rawRow := range rawRows {
		var row map[string]any
		if err := json.Unmarshal(rawRow, &row); err != nil || row == nil {
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored preferences.settings row %d: not a JSON object", i+1))
			continue
		}

		key, _ := row["key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			parsed.SkippedSettings++
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored preferences.settings row %d: key missing or empty", i+1))
			continue
		}
		if RuntimeLocalSettingKeys[key] {
			parsed.SkippedSettings++
			continue
		}

		for field := range row {
			if field != "key" && field != "value" {
				parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown field settings.%s", field))
			}
		}

		value, hasValue := row["value"]
		if !hasValue {
			value = nil
		}
		// The TS importer stores JSON.stringify(value); the Go settings table
		// also holds JSON-encoded text, so re-marshal the parsed value.
		rawValue, err := json.Marshal(value)
		if err != nil {
			return TSV21ClientError{Message: fmt.Sprintf("import failed: preferences.settings row %d value cannot be serialized: %v", i+1, err)}
		}
		if err := checkTSV21CellBytes("value", string(rawValue)); err != nil {
			return err
		}
		rows = append(rows, map[string]any{"key": key, "value": string(rawValue)})
	}
	if len(rows) > 0 {
		parsed.Tables["settings"] = rows
		parsed.TableOrder = append(parsed.TableOrder, "settings")
	}
	return nil
}
