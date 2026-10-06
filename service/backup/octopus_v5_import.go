package backup

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

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
