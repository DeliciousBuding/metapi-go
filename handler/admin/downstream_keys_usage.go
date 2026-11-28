package admin

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
)

// GET /api/downstream-keys/summary?range=&status=&search=&group=&tags=&tagMatch=
func (h *downstreamKeysHandler) summary(w http.ResponseWriter, r *http.Request) {
	rangeFilter := normalizeRange(r.URL.Query().Get("range"))
	statusFilter := normalizeStatus(r.URL.Query().Get("status"))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 80 {
		search = search[:80]
	}
	group := strings.TrimSpace(r.URL.Query().Get("group"))

	query := "SELECT * FROM downstream_api_keys"
	var conditions []string
	var args []any

	if statusFilter == "enabled" {
		conditions = append(conditions, "enabled = ?")
		args = append(args, true)
	} else if statusFilter == "disabled" {
		conditions = append(conditions, "enabled = ?")
		args = append(args, false)
	}
	if search != "" {
		conditions = append(conditions, "(LOWER(name) LIKE ? OR LOWER(COALESCE(description, '')) LIKE ?)")
		like := "%" + strings.ToLower(search) + "%"
		args = append(args, like, like)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY id DESC"

	rows, err := queryRowsErr(h.db, query, args...)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load downstream keys")
		return
	}

	// Apply group filter and enrich with usage data
	items := make([]map[string]any, 0)
	for _, row := range rows {
		groupName := existingString(row, "group_name")
		if group == "__ungrouped__" && groupName != "" {
			continue
		}
		if group != "" && group != "__ungrouped__" && groupName != group {
			continue
		}
		// Mask then drop plaintext key before list surfaces.
		redactDownstreamKeySecret(row)
		enrichKeyRateWindow(row)
		items = append(items, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"range":    rangeFilter,
		"status":   statusFilter,
		"search":   search,
		"group":    group,
		"tags":     []string{},
		"tagMatch": "any",
		"items":    normalizeSlice(items),
	})
}

// GET /api/downstream-keys
//
// Defensive pagination (#719/#711 parity): the endpoint is unbounded by
// default, but operators can opt into server-side paging with ?page/&pageSize.
// When ?page is absent the behavior and response shape are byte-identical to
// the legacy surface ({success, items}) so existing frontend code keeps working
// untouched. Only when a non-empty ?page is supplied does the handler apply
// LIMIT/OFFSET and return the {items,total,page,pageSize} envelope used by
// /api/channels and /api/checkin/logs.
func (h *downstreamKeysHandler) listKeys(w http.ResponseWriter, r *http.Request) {
	pageStr := strings.TrimSpace(r.URL.Query().Get("page"))
	paginate := pageStr != ""

	var rows []map[string]any
	var total int64
	var queryErr error
	page, pageSize := 1, 50
	if paginate {
		page = config.ClampInt(getQueryInt(r, "page", 1), 1, 1_000_000)
		pageSize = config.ClampInt(getQueryInt(r, "pageSize", 50), 1, 200)
		offset := (page - 1) * pageSize
		rows, queryErr = queryRowsErr(h.db,
			"SELECT * FROM downstream_api_keys ORDER BY id DESC LIMIT ? OFFSET ?",
			pageSize, offset)
		if queryErr != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load downstream keys")
			return
		}
		if err := h.db.Get(&total, "SELECT COUNT(*) FROM downstream_api_keys"); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to count downstream keys")
			return
		}
	} else {
		rows, queryErr = queryRowsErr(h.db, "SELECT * FROM downstream_api_keys ORDER BY id DESC")
		if queryErr != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load downstream keys")
			return
		}
	}

	for _, row := range rows {
		// List must not return full key; only keyMasked.
		redactDownstreamKeySecret(row)
		enrichKeyRateWindow(row)
	}
	h.enrichKeyUsage24h(rows)
	var migratedKeyID int64
	if err := h.db.Get(&migratedKeyID, `SELECT value FROM settings WHERE key = 'downstream.root_token_migrated_v1'`); err != nil && err != sql.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "failed to load key migration status")
		return
	}

	if paginate {
		writeJSON(w, http.StatusOK, map[string]any{
			"items":         normalizeSlice(rows),
			"total":         total,
			"page":          page,
			"pageSize":      pageSize,
			"migratedKeyId": migratedKeyID,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"items":         normalizeSlice(rows),
		"migratedKeyId": migratedKeyID,
	})
}

// GET /api/downstream-keys/:id/overview
func (h *downstreamKeysHandler) overview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	row, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load API key")
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}

	// Overview must not return full key; only keyMasked.
	redactDownstreamKeySecret(row)

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"item":    row,
		"usage": map[string]any{
			"last24h": nil,
			"last7d":  nil,
			"all":     nil,
		},
	})
}

// GET /api/downstream-keys/:id/trend?range=&timeZone=
func (h *downstreamKeysHandler) trend(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	row, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load API key")
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}

	rangeFilter := normalizeRange(r.URL.Query().Get("range"))

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"range":         rangeFilter,
		"item":          map[string]any{"id": id, "name": row["name"]},
		"bucketSeconds": 3600,
		"timeZone":      "UTC",
		"buckets":       []any{},
	})
}

// enrichKeyRateWindow attaches process-local RPM/TPM window usage for admin display.
func enrichKeyRateWindow(row map[string]any) {
	idVal, ok := row["id"]
	if !ok || idVal == nil {
		return
	}
	var id int64
	switch v := idVal.(type) {
	case int64:
		id = v
	case int:
		id = int64(v)
	case float64:
		id = int64(v)
	default:
		return
	}
	usedRPM, usedTPM := keyAdmissionSnapshot(id)
	row["windowUsedRpm"] = usedRPM
	row["windowUsedTpm"] = usedTPM
}

// enrichKeyUsage24h attaches a per-key 24h proxy_logs aggregate to every list
// row as usage24h {requests, tokens, cost}. One grouped query serves all keys;
// keys without traffic in the window get a zero-valued object so the JSON
// shape is uniform. Token sums reuse EffectiveProxyTokensSQL (total_tokens
// with prompt+completion fallback), matching the stats proxy24h metric. The
// enrichment is best-effort: a failed aggregate leaves usage24h unset rather
// than breaking the key list.
func (h *downstreamKeysHandler) enrichKeyUsage24h(rows []map[string]any) {
	if len(rows) == 0 {
		return
	}
	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	aggRows, err := queryRowsErr(h.db, `
		SELECT
			pl.downstream_api_key_id AS key_id,
			COUNT(*) AS requests,
			COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens,
			COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS cost
		FROM proxy_logs pl
		WHERE pl.created_at >= ? AND pl.downstream_api_key_id IS NOT NULL
		GROUP BY pl.downstream_api_key_id`, since)
	if err != nil {
		return
	}

	usageByKey := make(map[int64]map[string]any, len(aggRows))
	for _, agg := range aggRows {
		keyID := coerceInt64(agg["keyId"])
		if keyID <= 0 {
			continue
		}
		usageByKey[keyID] = map[string]any{
			"requests": coerceInt64(agg["requests"]),
			"tokens":   coerceInt64(agg["tokens"]),
			"cost":     roundMicro(coerceFloat(agg["cost"])),
		}
	}

	for _, row := range rows {
		id := coerceInt64(mustRowValue(row, "id"))
		usage := usageByKey[id]
		if usage == nil {
			usage = map[string]any{"requests": int64(0), "tokens": int64(0), "cost": float64(0)}
		}
		row["usage24h"] = usage
	}
}
