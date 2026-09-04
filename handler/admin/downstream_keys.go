package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterDownstreamKeysRoutes registers all /api/downstream-keys routes.
func RegisterDownstreamKeysRoutes(r chi.Router, db *sqlx.DB) {
	handler := &downstreamKeysHandler{db: db}

	r.Get("/api/downstream-keys/summary", handler.summary)
	r.Get("/api/downstream-keys", handler.listKeys)
	r.Post("/api/downstream-keys", handler.createKey)
	r.Post("/api/downstream-keys/batch", handler.batchKeys)
	r.Get("/api/downstream-keys/{id}/overview", handler.overview)
	r.Get("/api/downstream-keys/{id}/export", handler.exportKey)
	r.Get("/api/downstream-keys/{id}/trend", handler.trend)
	r.Put("/api/downstream-keys/{id}", handler.updateKey)
	r.Post("/api/downstream-keys/{id}/reset-usage", handler.resetUsage)
	r.Delete("/api/downstream-keys/{id}", handler.deleteKey)
}

type downstreamKeysHandler struct {
	db *sqlx.DB
}

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
		if err := h.db.Get(&total, h.db.Rebind("SELECT COUNT(*) FROM downstream_api_keys")); err != nil {
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

	if paginate {
		writeJSON(w, http.StatusOK, map[string]any{
			"items":    normalizeSlice(rows),
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"items":   normalizeSlice(rows),
	})
}

// POST /api/downstream-keys
func (h *downstreamKeysHandler) createKey(w http.ResponseWriter, r *http.Request) {
	// maxCost/maxRequests use any so clients can send number|string|null (TS parity).
	// Explicit null/0/"" clears to unlimited (NULL); omitted also stores NULL on create.
	var body struct {
		Name                   string   `json:"name"`
		Key                    string   `json:"key"`
		Description            *string  `json:"description"`
		GroupName              *string  `json:"groupName"`
		Tags                   []string `json:"tags"`
		Enabled                *bool    `json:"enabled"`
		ExpiresAt              *string  `json:"expiresAt"`
		MaxCost                any      `json:"maxCost"`
		MaxRequests            any      `json:"maxRequests"`
		MaxRpm                 any      `json:"maxRpm"`
		MaxTpm                 any      `json:"maxTpm"`
		SupportedModels        []string `json:"supportedModels"`
		AllowedRouteIds        []int64  `json:"allowedRouteIds"`
		SiteWeightMultipliers  any      `json:"siteWeightMultipliers"`
		KeyWeight              any      `json:"keyWeight"`
		ExcludedSiteIds        []int64  `json:"excludedSiteIds"`
		ExcludedCredentialRefs []any    `json:"excludedCredentialRefs"`
		AllowedSiteIds         []int64  `json:"allowedSiteIds"`
		AllowedCredentialRefs  []any    `json:"allowedCredentialRefs"`
		ProxyURL               *string  `json:"proxyUrl"`
		IPAllowlist            *string  `json:"ipAllowlist"`
		IPBlocklist            *string  `json:"ipBlocklist"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Key = strings.TrimSpace(body.Key)

	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	if !strings.HasPrefix(body.Key, "sk-") || len(body.Key) < 6 {
		writeError(w, http.StatusBadRequest, "key must start with sk- and be at least 6 characters long")
		return
	}

	// Check for duplicate key
	var count int
	h.db.Get(&count, rebindAdminQuery(h.db, "SELECT COUNT(*) FROM downstream_api_keys WHERE key = ?"), body.Key)
	if count > 0 {
		writeError(w, http.StatusConflict, "API key already exists")
		return
	}

	// Normalize policy fields.
	normalizedTags := normalizeTagsInput(body.Tags)
	normalizedModels := normalizeSupportedModelsInput(body.SupportedModels)
	normalizedRouteIds := normalizeAllowedRouteIdsInput(body.AllowedRouteIds)
	normalizedSWM := normalizeSiteWeightMultipliersInput(body.SiteWeightMultipliers)
	normalizedExcludedSites := normalizeInt64Set(body.ExcludedSiteIds)
	normalizedCredRefs, credRefsErr := normalizeExcludedCredentialRefsInput(body.ExcludedCredentialRefs)
	if credRefsErr != "" {
		writeError(w, http.StatusBadRequest, "excludedCredentialRefs: "+credRefsErr)
		return
	}
	normalizedAllowedSites := normalizeInt64Set(body.AllowedSiteIds)
	normalizedAllowedCredRefs, allowedCredRefsErr := normalizeExcludedCredentialRefsInput(body.AllowedCredentialRefs)
	if allowedCredRefsErr != "" {
		writeError(w, http.StatusBadRequest, "allowedCredentialRefs: "+allowedCredRefsErr)
		return
	}
	maxCost := normalizeQuotaFloatOrNull(body.MaxCost)
	maxRequests := normalizeQuotaIntOrNull(body.MaxRequests)
	maxRpm := normalizeQuotaIntOrNull(body.MaxRpm)
	maxTpm := normalizeQuotaIntOrNull(body.MaxTpm)
	keyWeight := normalizeKeyWeightInput(body.KeyWeight)

	// Policy reference validation.
	refErr, refDbErr := h.validateDownstreamPolicyReferences(normalizedRouteIds, normalizedSWM, normalizedExcludedSites, normalizedCredRefs, normalizedAllowedSites, normalizedAllowedCredRefs)
	if refDbErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate policy references")
		return
	}
	if refErr != "" {
		writeError(w, http.StatusBadRequest, refErr)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	tagsJSON := toPersistenceJSON(normalizedTags)
	modelsJSON := toPersistenceJSON(normalizedModels)
	routeIdsJSON := toPersistenceJSON(normalizedRouteIds)
	swmJSON := toPersistenceJSON(normalizedSWM)
	excludedSitesJSON := toPersistenceJSON(normalizedExcludedSites)
	credRefsJSON := toPersistenceJSON(normalizedCredRefs)
	allowedSitesJSON := toPersistenceJSON(normalizedAllowedSites)
	allowedCredRefsJSON := toPersistenceJSON(normalizedAllowedCredRefs)
	normalizedGroupName := normalizeGroupNameInput(body.GroupName)
	var desc interface{}
	desc = nil
	if body.Description != nil && strings.TrimSpace(*body.Description) != "" {
		s := strings.TrimSpace(*body.Description)
		desc = &s
	}

	// Per-key egress proxy. NULL/empty inherits site/system.
	proxyURL, proxyErr := normalizeDownstreamProxyURL(body.ProxyURL)
	if proxyErr != "" {
		writeError(w, http.StatusBadRequest, proxyErr)
		return
	}

	// Per-key IP allow/block. NULL = unrestricted.
	ipAllowlist := normalizeIPListPtr(body.IPAllowlist)
	ipBlocklist := normalizeIPListPtr(body.IPBlocklist)

	id, err := execInsertID(h.db,
		`INSERT INTO downstream_api_keys
		(name, key, description, group_name, tags, enabled, expires_at, max_cost, used_cost, max_requests, used_requests,
		 supported_models, allowed_route_ids, site_weight_multipliers, key_weight, excluded_site_ids, excluded_credential_refs,
		 allowed_site_ids, allowed_credential_refs,
		 proxy_url, max_rpm, max_tpm, ip_allowlist, ip_blocklist, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		body.Name, body.Key, desc, normalizedGroupName, tagsJSON, enabled, body.ExpiresAt,
		maxCost, maxRequests,
		modelsJSON, routeIdsJSON, swmJSON, keyWeight, excludedSitesJSON, credRefsJSON,
		allowedSitesJSON, allowedCredRefsJSON,
		proxyURL, maxRpm, maxTpm, ipAllowlist, ipBlocklist, now, now,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "API key already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "creation failed")
		return
	}
	created := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if created != nil {
		if key, ok := created["key"].(string); ok {
			created["keyMasked"] = maskSecret(key)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"item":    created,
	})
}

// PUT /api/downstream-keys/:id
func (h *downstreamKeysHandler) updateKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	existing := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if existing == nil {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}

	// Parse body into typed struct with pointers to detect field presence.
	// nil pointer = field not present in JSON body for most fields.
	// maxCost/maxRequests are decoded from the raw map so number|string|null all work.
	var body struct {
		Name                   *string  `json:"name"`
		Key                    *string  `json:"key"`
		Description            *string  `json:"description"`
		GroupName              *string  `json:"groupName"`
		Tags                   []string `json:"tags"`
		Enabled                *bool    `json:"enabled"`
		ExpiresAt              *string  `json:"expiresAt"`
		SupportedModels        []string `json:"supportedModels"`
		AllowedRouteIds        []int64  `json:"allowedRouteIds"`
		SiteWeightMultipliers  any      `json:"siteWeightMultipliers"`
		KeyWeight              any      `json:"keyWeight"`
		ExcludedSiteIds        []int64  `json:"excludedSiteIds"`
		ExcludedCredentialRefs []any    `json:"excludedCredentialRefs"`
		AllowedSiteIds         []int64  `json:"allowedSiteIds"`
		AllowedCredentialRefs  []any    `json:"allowedCredentialRefs"`
		ProxyURL               *string  `json:"proxyUrl"`
		IPAllowlist            *string  `json:"ipAllowlist"`
		IPBlocklist            *string  `json:"ipBlocklist"`
	}

	bodyBytes, err := decodeJSONRequestRaw(r, &body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// First unmarshal into map to detect which fields were present in JSON.
	// Omitted maxCost/maxRequests keep existing values; present null/0/"" clear to NULL.
	rawBody := map[string]any{}
	if err := json.Unmarshal(bodyBytes, &rawBody); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	hasField := make(map[string]bool)
	if _, ok := rawBody["name"]; ok {
		hasField["name"] = true
	}
	if _, ok := rawBody["key"]; ok {
		hasField["key"] = true
	}
	if _, ok := rawBody["description"]; ok {
		hasField["description"] = true
	}
	if _, ok := rawBody["groupName"]; ok {
		hasField["groupName"] = true
	}
	if _, ok := rawBody["tags"]; ok {
		hasField["tags"] = true
	}
	if _, ok := rawBody["enabled"]; ok {
		hasField["enabled"] = true
	}
	if _, ok := rawBody["expiresAt"]; ok {
		hasField["expiresAt"] = true
	}
	if _, ok := rawBody["maxCost"]; ok {
		hasField["maxCost"] = true
	}
	if _, ok := rawBody["maxRequests"]; ok {
		hasField["maxRequests"] = true
	}
	if _, ok := rawBody["maxRpm"]; ok {
		hasField["maxRpm"] = true
	}
	if _, ok := rawBody["maxTpm"]; ok {
		hasField["maxTpm"] = true
	}
	if _, ok := rawBody["supportedModels"]; ok {
		hasField["supportedModels"] = true
	}
	if _, ok := rawBody["allowedRouteIds"]; ok {
		hasField["allowedRouteIds"] = true
	}
	if _, ok := rawBody["siteWeightMultipliers"]; ok {
		hasField["siteWeightMultipliers"] = true
	}
	if _, ok := rawBody["keyWeight"]; ok {
		hasField["keyWeight"] = true
	}
	if _, ok := rawBody["excludedSiteIds"]; ok {
		hasField["excludedSiteIds"] = true
	}
	if _, ok := rawBody["excludedCredentialRefs"]; ok {
		hasField["excludedCredentialRefs"] = true
	}
	if _, ok := rawBody["allowedSiteIds"]; ok {
		hasField["allowedSiteIds"] = true
	}
	if _, ok := rawBody["allowedCredentialRefs"]; ok {
		hasField["allowedCredentialRefs"] = true
	}
	if _, ok := rawBody["proxyUrl"]; ok {
		hasField["proxyUrl"] = true
	}
	if _, ok := rawBody["ipAllowlist"]; ok {
		hasField["ipAllowlist"] = true
	}
	if _, ok := rawBody["ipBlocklist"]; ok {
		hasField["ipBlocklist"] = true
	}

	// Merge: present fields from body, missing fields from existing record.
	name := existingString(existing, "name")
	if hasField["name"] && body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}

	key := existingString(existing, "key")
	if hasField["key"] && body.Key != nil {
		key = strings.TrimSpace(*body.Key)
	}

	description := existingStringPtr(existing, "description")
	if hasField["description"] {
		if body.Description != nil && strings.TrimSpace(*body.Description) != "" {
			s := strings.TrimSpace(*body.Description)
			description = &s
		} else {
			description = nil
		}
	}

	existingGroupName := existingStringPtr(existing, "group_name")
	groupName := normalizeGroupNameInput(existingGroupName)
	if hasField["groupName"] {
		groupName = normalizeGroupNameInput(body.GroupName)
	}

	existingTags := parseStringArrayFromDB(existing, "tags")
	tags := existingTags
	if hasField["tags"] {
		tags = normalizeTagsInput(body.Tags)
	}

	enabled := existingBool(existing, "enabled")
	if hasField["enabled"] && body.Enabled != nil {
		enabled = *body.Enabled
	}

	expiresAt := existingStringPtr(existing, "expires_at")
	if hasField["expiresAt"] {
		expiresAt = normalizeExpiresAt(body.ExpiresAt)
	}

	maxCost := existingFloat64Ptr(existing, "max_cost")
	if hasField["maxCost"] {
		// Explicit clear (null/0/"") → NULL unlimited; positive → set.
		maxCost = normalizeQuotaFloatOrNull(rawBody["maxCost"])
	}

	maxRequests := existingInt64Ptr(existing, "max_requests")
	maxRpm := existingInt64Ptr(existing, "max_rpm")
	maxTpm := existingInt64Ptr(existing, "max_tpm")
	if hasField["maxRequests"] {
		maxRequests = normalizeQuotaIntOrNull(rawBody["maxRequests"])
	}
	if hasField["maxRpm"] {
		maxRpm = normalizeQuotaIntOrNull(rawBody["maxRpm"])
	}
	if hasField["maxTpm"] {
		maxTpm = normalizeQuotaIntOrNull(rawBody["maxTpm"])
	}

	existingSupportedModels := parseStringArrayFromDB(existing, "supported_models")
	supportedModels := existingSupportedModels
	if hasField["supportedModels"] {
		supportedModels = normalizeSupportedModelsInput(body.SupportedModels)
	}

	existingAllowedRouteIds := parseIntArrayFromDB(existing, "allowed_route_ids")
	allowedRouteIds := existingAllowedRouteIds
	if hasField["allowedRouteIds"] {
		allowedRouteIds = normalizeAllowedRouteIdsInput(body.AllowedRouteIds)
	}
	// Heal an allowlist this request did not choose. A route deleted before
	// pruneDeletedRouteIDsFromDownstreamKeys existed left its id behind here, and
	// the validation below then rejected EVERY save of this key — including a
	// rename, because an omitted allowedRouteIds carries the stored list forward.
	// The UI has no control for this field, so there was no way to drop the dead
	// id: the key became permanently uneditable. Ids the request inherits are
	// pruned and reported; ids it introduces stay in the list so validation still
	// rejects a mistyped route id loudly.
	grants, healErr := h.healInheritedRouteGrants(allowedRouteIds, existingAllowedRouteIds)
	if healErr != nil {
		slog.Warn("downstream key update could not check inherited route grants",
			"downstreamKeyId", id, "error", healErr)
		writeError(w, http.StatusInternalServerError, "failed to validate policy references")
		return
	}
	allowedRouteIds = grants.Persist

	existingSiteWeightMultipliers := parseMapFromDB(existing, "site_weight_multipliers")
	siteWeightMultipliers := existingSiteWeightMultipliers
	if hasField["siteWeightMultipliers"] {
		siteWeightMultipliers = normalizeSiteWeightMultipliersInput(body.SiteWeightMultipliers)
	}

	keyWeight := existingFloat64Ptr(existing, "key_weight")
	if hasField["keyWeight"] {
		keyWeight = normalizeKeyWeightInput(rawBody["keyWeight"])
	}

	existingExcludedSiteIds := parseIntArrayFromDB(existing, "excluded_site_ids")
	excludedSiteIds := existingExcludedSiteIds
	if hasField["excludedSiteIds"] {
		excludedSiteIds = normalizeInt64Set(body.ExcludedSiteIds)
	}

	existingExcludedCredentialRefs := parseAnyArrayFromDB(existing, "excluded_credential_refs")
	excludedCredentialRefs := existingExcludedCredentialRefs
	if hasField["excludedCredentialRefs"] {
		normalized, refErr := normalizeExcludedCredentialRefsInput(body.ExcludedCredentialRefs)
		if refErr != "" {
			writeError(w, http.StatusBadRequest, "excludedCredentialRefs: "+refErr)
			return
		}
		excludedCredentialRefs = normalized
	}

	existingAllowedSiteIds := parseIntArrayFromDB(existing, "allowed_site_ids")
	allowedSiteIds := existingAllowedSiteIds
	if hasField["allowedSiteIds"] {
		allowedSiteIds = normalizeInt64Set(body.AllowedSiteIds)
	}

	existingAllowedCredentialRefs := parseAnyArrayFromDB(existing, "allowed_credential_refs")
	allowedCredentialRefs := existingAllowedCredentialRefs
	if hasField["allowedCredentialRefs"] {
		normalized, refErr := normalizeExcludedCredentialRefsInput(body.AllowedCredentialRefs)
		if refErr != "" {
			writeError(w, http.StatusBadRequest, "allowedCredentialRefs: "+refErr)
			return
		}
		allowedCredentialRefs = normalized
	}

	// proxyUrl: absent keeps existing; present empty/null clears to inherit site/system.
	proxyURL := existingStringPtr(existing, "proxy_url")
	if hasField["proxyUrl"] {
		normalized, proxyErr := normalizeDownstreamProxyURL(body.ProxyURL)
		if proxyErr != "" {
			writeError(w, http.StatusBadRequest, proxyErr)
			return
		}
		proxyURL = normalized
	}

	// IP allow/block: absent keeps existing; present empty/null clears to unrestricted.
	ipAllowlist := existingStringPtr(existing, "ip_allowlist")
	if hasField["ipAllowlist"] {
		ipAllowlist = normalizeIPListPtr(body.IPAllowlist)
	}
	ipBlocklist := existingStringPtr(existing, "ip_blocklist")
	if hasField["ipBlocklist"] {
		ipBlocklist = normalizeIPListPtr(body.IPBlocklist)
	}

	// Validate.
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	if !strings.HasPrefix(key, "sk-") || len(key) < 6 {
		writeError(w, http.StatusBadRequest, "key must start with sk- and be at least 6 characters long")
		return
	}

	// Duplicate key check: if key is being changed, verify new key doesn't collide.
	if hasField["key"] && key != existingString(existing, "key") {
		var dupCount int
		h.db.Get(&dupCount, rebindAdminQuery(h.db, "SELECT COUNT(*) FROM downstream_api_keys WHERE key = ? AND id != ?"), key, id)
		if dupCount > 0 {
			writeError(w, http.StatusConflict, "API key already exists")
			return
		}
	}

	// Policy reference validation.
	// Validation vouches for what this save is responsible for: live grants and
	// ids the request introduced. Grants left behind by a route deletion that
	// cannot be pruned without widening the key are reported instead of rejected,
	// which is what made such a key uneditable before.
	refErr, refDbErr := h.validateDownstreamPolicyReferences(grants.Validate, siteWeightMultipliers, excludedSiteIds, excludedCredentialRefs, allowedSiteIds, allowedCredentialRefs)
	if refDbErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate policy references")
		return
	}
	if refErr != "" {
		writeError(w, http.StatusBadRequest, refErr)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON := toPersistenceJSON(tags)
	modelsJSON := toPersistenceJSON(supportedModels)
	routeIdsJSON := toPersistenceJSON(allowedRouteIds)
	swmJSON := toPersistenceJSON(siteWeightMultipliers)
	excludedSitesJSON := toPersistenceJSON(excludedSiteIds)
	credRefsJSON := toPersistenceJSON(excludedCredentialRefs)
	allowedSitesJSON := toPersistenceJSON(allowedSiteIds)
	allowedCredRefsJSON := toPersistenceJSON(allowedCredentialRefs)
	_, err = h.db.Exec(
		rebindAdminQuery(h.db,
			`UPDATE downstream_api_keys SET
			name = ?, key = ?, description = ?, group_name = ?, tags = ?,
			enabled = ?, expires_at = ?, max_cost = ?, max_requests = ?, max_rpm = ?, max_tpm = ?,
			supported_models = ?, allowed_route_ids = ?, site_weight_multipliers = ?, key_weight = ?,
			excluded_site_ids = ?, excluded_credential_refs = ?,
				allowed_site_ids = ?, allowed_credential_refs = ?, proxy_url = ?, ip_allowlist = ?, ip_blocklist = ?, updated_at = ?
		WHERE id = ?`),
		name, key, description, groupName, tagsJSON,
		enabled, expiresAt, maxCost, maxRequests, maxRpm, maxTpm,
		modelsJSON, routeIdsJSON, swmJSON, keyWeight,
		excludedSitesJSON, credRefsJSON,
		allowedSitesJSON, allowedCredRefsJSON, proxyURL, ipAllowlist, ipBlocklist, now, id,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "API key already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	updated := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	// Update must not return full key; only keyMasked.
	redactDownstreamKeySecret(updated)

	response := map[string]any{
		"success": true,
		"item":    updated,
	}
	if len(grants.Healed) > 0 {
		// The save succeeded and the allowlist got smaller; say so rather than
		// let the caller believe it wrote the list it sent.
		response["prunedRouteIds"] = grants.Healed
		slog.Warn("downstream key update dropped route grants that no longer exist",
			"downstreamKeyId", id, "prunedRouteIds", grants.Healed)
	}
	if len(grants.Stale) > 0 {
		response["staleRouteIds"] = grants.Stale
		response["staleRouteGrantNotice"] = "this key authorizes only routes that no longer exist, so it serves nothing; the grants were kept because dropping them would widen the key to every route — replace them with live routes, or clear the list to allow all routes"
		slog.Warn("downstream key still authorizes only deleted routes",
			"downstreamKeyId", id, "staleRouteIds", grants.Stale)
	}
	writeJSON(w, http.StatusOK, response)
}

// routeGrantHeal is the outcome of reconciling a key's route allowlist with the
// routes that still exist.
//
// The rule, stated once because both the save path and the route-delete path
// depend on it: a dead grant is removed only when removing it cannot widen the
// key. An empty allowlist means "no route restriction", so pruning the LAST
// grant of a restricted key would silently promote it from "serves nothing" to
// "serves every route" — the same fail-open shape as an allowlist that falls
// back to empty. When every grant is dead the ids stay (the key keeps denying),
// the save still succeeds, and the ids are reported so an operator can decide:
// widening a credential has to be an explicit act.
type routeGrantHeal struct {
	Persist  []int64 // what goes to the database
	Validate []int64 // what reference validation must still vouch for
	Healed   []int64 // dead grants removed
	Stale    []int64 // dead grants kept, because removing them would widen the key
}

// healInheritedRouteGrants applies that rule to one key. Inherited ids are the
// ones already stored: a dead id this request did not introduce is a leftover of
// a route deletion, not a mistake to reject. Ids the request introduces stay in
// Validate, so a mistyped route id is still rejected loudly instead of being
// quietly dropped — dropping what an operator just typed would also widen the
// policy they asked for.
func (h *downstreamKeysHandler) healInheritedRouteGrants(requested, inherited []int64) (routeGrantHeal, error) {
	out := routeGrantHeal{Persist: requested, Validate: requested}
	if len(requested) == 0 {
		return out, nil
	}
	existing := make(map[int64]bool, len(requested))
	query, args, err := sqlx.In("SELECT id FROM token_routes WHERE id IN (?)", requested)
	if err != nil {
		return out, err
	}
	rows, err := queryRowsErr(h.db, query, args...)
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		if routeID, ok := rowInt64(row["id"]); ok {
			existing[routeID] = true
		}
	}
	inheritedSet := make(map[int64]bool, len(inherited))
	for _, routeID := range inherited {
		inheritedSet[routeID] = true
	}

	persist := make([]int64, 0, len(requested))
	validate := make([]int64, 0, len(requested))
	for _, routeID := range requested {
		switch {
		case existing[routeID]:
			persist = append(persist, routeID)
			validate = append(validate, routeID)
		case !inheritedSet[routeID]:
			// Chosen by this request and unknown: keep it in both lists so
			// validation rejects it with the reason.
			persist = append(persist, routeID)
			validate = append(validate, routeID)
		default:
			// Inherited and dead. Dropped only when something live survives.
			out.Healed = append(out.Healed, routeID)
		}
	}
	if len(persist) == 0 {
		// Every grant is dead: keep them all so the key still denies every
		// route, and exempt them from validation so the key stays editable.
		out.Stale = out.Healed
		out.Healed = nil
		out.Persist = requested
		out.Validate = nil
		return out, nil
	}
	out.Persist, out.Validate = persist, validate
	return out, nil
}

// pruneDeletedRouteIDsFromDownstreamKeys drops deleted routes from downstream
// key allowlists inside the transaction that deletes them, and reports the keys
// it could not prune without widening.
//
// Route deletion cleaned route_group_sources, route_channels and token_routes
// and stopped there, so downstream_api_keys.allowed_route_ids kept the dead id
// forever. That is not a cosmetic leak: the key update path validates the list,
// carries the stored list forward when a request omits it, and the admin UI has
// no control for the field — so one deleted route made every later edit of that
// key fail with "allowedRouteIds contains unknown routes: <id>", with no way to
// remove the id. Reported by an operator running 100+ sites, where route churn
// is normal rather than exceptional.
//
// Route ids are never reused (SQLite AUTOINCREMENT, PostgreSQL SERIAL), so a
// stale grant can never come back to life pointing at a different route.
func pruneDeletedRouteIDsFromDownstreamKeys(tx *sqlx.Tx, routeIDs ...int64) (onlyDeleted []int64, err error) {
	if len(routeIDs) == 0 {
		return nil, nil
	}
	gone := make(map[int64]bool, len(routeIDs))
	for _, routeID := range routeIDs {
		gone[routeID] = true
	}

	rows, err := tx.Queryx(tx.Rebind(
		"SELECT id, name, allowed_route_ids FROM downstream_api_keys WHERE allowed_route_ids IS NOT NULL AND allowed_route_ids <> ''"))
	if err != nil {
		return nil, fmt.Errorf("read downstream key route grants: %w", err)
	}
	type heal struct {
		keyID   int64
		kept    []int64
		removed []int64
		allDead bool
	}
	var heals []heal
	for rows.Next() {
		row := map[string]any{}
		if scanErr := rows.MapScan(row); scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("scan downstream key: %w", scanErr)
		}
		ids := parseIntArrayFromDB(row, "allowed_route_ids")
		if len(ids) == 0 {
			continue
		}
		kept := make([]int64, 0, len(ids))
		var removed []int64
		for _, routeID := range ids {
			if gone[routeID] {
				removed = append(removed, routeID)
				continue
			}
			kept = append(kept, routeID)
		}
		if len(removed) == 0 {
			continue
		}
		keyID, ok := rowInt64(row["id"])
		if !ok {
			rows.Close()
			return nil, fmt.Errorf("downstream key row has no usable id")
		}
		heals = append(heals, heal{keyID: keyID, kept: kept, removed: removed, allDead: len(kept) == 0})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate downstream keys: %w", err)
	}
	rows.Close()

	for _, h := range heals {
		if h.allDead {
			// Pruning would empty the allowlist, and empty means "every route".
			// Leave the dead grant in place so the key keeps serving nothing; the
			// save path tolerates and reports it, so the key stays editable.
			onlyDeleted = append(onlyDeleted, h.keyID)
			slog.Warn("route delete left a downstream key authorizing only deleted routes",
				"downstreamKeyId", h.keyID, "deletedRouteIds", h.removed,
				"reason", "pruning the last grant would widen the key to every route; re-authorize it explicitly")
			continue
		}
		if _, execErr := tx.Exec(tx.Rebind("UPDATE downstream_api_keys SET allowed_route_ids = ? WHERE id = ?"),
			toPersistenceJSON(h.kept), h.keyID); execErr != nil {
			return nil, fmt.Errorf("prune downstream key %d route grants: %w", h.keyID, execErr)
		}
		slog.Info("route delete pruned downstream key route grants",
			"downstreamKeyId", h.keyID, "removedRouteIds", h.removed, "remainingRouteIds", len(h.kept))
	}
	return onlyDeleted, nil
}

// rowInt64 reads an integer column across both dialects: SQLite scans into
// int64, PostgreSQL into int64 as well but a driver change or a NUMERIC column
// must not silently read as "no id".
func rowInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	case []byte:
		parsed, err := strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

// POST /api/downstream-keys/:id/reset-usage
func (h *downstreamKeysHandler) resetUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE downstream_api_keys SET used_cost = 0, used_requests = 0, updated_at = ? WHERE id = ?"), now, id); err != nil {
		writeError(w, http.StatusInternalServerError, "reset failed")
		return
	}

	updated := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	// Reset-usage must not return full key; only keyMasked.
	redactDownstreamKeySecret(updated)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"item":    updated,
	})
}

// DELETE /api/downstream-keys/:id
func (h *downstreamKeysHandler) deleteKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if _, err := h.db.Exec(rebindAdminQuery(h.db, "DELETE FROM downstream_api_keys WHERE id = ?"), id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// GET /api/downstream-keys/:id/overview
func (h *downstreamKeysHandler) overview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	row := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
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

	row := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
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

// POST /api/downstream-keys/batch
func (h *downstreamKeysHandler) batchKeys(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs            []int64  `json:"ids"`
		Action         string   `json:"action"`
		GroupName      *string  `json:"groupName"`
		GroupOperation *string  `json:"groupOperation"`
		Tags           []string `json:"tags"`
		TagOperation   *string  `json:"tagOperation"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}

	action := strings.TrimSpace(body.Action)
	validActions := map[string]bool{
		"enable": true, "disable": true, "delete": true,
		"resetUsage": true, "updateMetadata": true,
	}
	if !validActions[action] {
		writeError(w, http.StatusBadRequest, "Invalid action")
		return
	}

	var successIDs []int64
	var failedItems []map[string]any

	for _, id := range body.IDs {
		row := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
		if row == nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "API key not found"})
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		switch action {
		case "delete":
			if _, err := h.db.Exec(rebindAdminQuery(h.db, "DELETE FROM downstream_api_keys WHERE id = ?"), id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "delete failed"})
				continue
			}
		case "resetUsage":
			if _, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE downstream_api_keys SET used_cost = 0, used_requests = 0, updated_at = ? WHERE id = ?"), now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "reset failed"})
				continue
			}
		case "enable":
			if _, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE downstream_api_keys SET enabled = ?, updated_at = ? WHERE id = ?"), true, now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "enable failed"})
				continue
			}
		case "disable":
			if _, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE downstream_api_keys SET enabled = ?, updated_at = ? WHERE id = ?"), false, now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "disable failed"})
				continue
			}
		case "updateMetadata":
			if body.GroupOperation != nil && *body.GroupOperation == "set" && body.GroupName != nil {
				if _, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE downstream_api_keys SET group_name = ?, updated_at = ? WHERE id = ?"), *body.GroupName, now, id); err != nil {
					failedItems = append(failedItems, map[string]any{"id": id, "message": "metadata update failed"})
					continue
				}
			} else if body.GroupOperation != nil && *body.GroupOperation == "clear" {
				if _, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE downstream_api_keys SET group_name = NULL, updated_at = ? WHERE id = ?"), now, id); err != nil {
					failedItems = append(failedItems, map[string]any{"id": id, "message": "metadata update failed"})
					continue
				}
			}
		}
		successIDs = append(successIDs, id)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"successIds":  successIDs,
		"failedItems": failedItems,
	})
}

// redactDownstreamKeySecret sets keyMasked from the stored secret and removes
// plaintext "key" so list/summary/overview/update/reset-usage JSON never leaks
// the full secret. Create and export keep intentional full-key
// returns and do not call this helper.
func (h *downstreamKeysHandler) validateDownstreamPolicyReferences(
	allowedRouteIds []int64,
	siteWeightMultipliers map[string]float64,
	excludedSiteIds []int64,
	excludedCredentialRefs []any,
	allowedSiteIds []int64,
	allowedCredentialRefs []any,
) (string, error) {
	// Validate allowedRouteIds exist in token_routes.
	if len(allowedRouteIds) > 0 {
		query, args, err := sqlx.In("SELECT id FROM token_routes WHERE id IN (?)", allowedRouteIds)
		if err == nil {
			rows, qErr := queryRowsErr(h.db, query, args...)
			if qErr != nil {
				return "", qErr
			}
			existingIds := make(map[int64]bool)
			for _, row := range rows {
				if id, ok := row["id"].(int64); ok {
					existingIds[id] = true
				}
			}
			var missing []string
			for _, rid := range allowedRouteIds {
				if !existingIds[rid] {
					missing = append(missing, strconv.FormatInt(rid, 10))
				}
			}
			if len(missing) > 0 {
				return fmt.Sprintf("allowedRouteIds contains unknown routes: %s", strings.Join(missing, ", ")), nil
			}
		}
	}

	// Collect all site IDs to validate.
	siteIdSet := make(map[int64]bool)
	for k := range siteWeightMultipliers {
		id, err := strconv.ParseInt(k, 10, 64)
		if err == nil && id > 0 {
			siteIdSet[id] = true
		}
	}
	for _, id := range excludedSiteIds {
		if id > 0 {
			siteIdSet[id] = true
		}
	}
	for _, id := range allowedSiteIds {
		if id > 0 {
			siteIdSet[id] = true
		}
	}
	if len(siteIdSet) > 0 {
		ids := make([]int64, 0, len(siteIdSet))
		for id := range siteIdSet {
			ids = append(ids, id)
		}
		query, args, err := sqlx.In("SELECT id FROM sites WHERE id IN (?)", ids)
		if err == nil {
			rows, qErr := queryRowsErr(h.db, query, args...)
			if qErr != nil {
				return "", qErr
			}
			existingIds := make(map[int64]bool)
			for _, row := range rows {
				if id, ok := row["id"].(int64); ok {
					existingIds[id] = true
				}
			}
			var missing []string
			for _, sid := range ids {
				if !existingIds[sid] {
					missing = append(missing, strconv.FormatInt(sid, 10))
				}
			}
			if len(missing) > 0 {
				return fmt.Sprintf("policy contains unknown sites: %s", strings.Join(missing, ", ")), nil
			}
		}
	}

	// Validate excludedCredentialRefs + allowedCredentialRefs.
	for _, ref := range append(append([]any{}, excludedCredentialRefs...), allowedCredentialRefs...) {
		obj, ok := ref.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := obj["kind"].(string)
		if kind == "account_token" {
			tokenId := coerceInt64(obj["tokenId"])
			accountId := coerceInt64(obj["accountId"])
			siteId := coerceInt64(obj["siteId"])
			if tokenId <= 0 {
				return fmt.Sprintf("credentialRefs contains an invalid token reference: %v", obj["tokenId"]), nil
			}
			row := queryRow(h.db,
				`SELECT at.id as token_id, at.account_id, a.site_id
				 FROM account_tokens at
				 INNER JOIN accounts a ON at.account_id = a.id
				 WHERE at.id = ?`, tokenId)
			if row == nil {
				return fmt.Sprintf("credentialRefs contains an unknown token: %d", tokenId), nil
			}
			dbAccountId := coerceInt64(mustRowValue(row, "account_id"))
			dbSiteId := coerceInt64(mustRowValue(row, "site_id"))
			if dbAccountId != accountId || dbSiteId != siteId {
				return fmt.Sprintf("credentialRefs account_token reference does not match the account/site: %d", tokenId), nil
			}
		} else if kind == "default_api_key" {
			accountId := coerceInt64(obj["accountId"])
			siteId := coerceInt64(obj["siteId"])
			row := queryRow(h.db,
				`SELECT id as account_id, site_id, api_token
				 FROM accounts WHERE id = ?`, accountId)
			if row == nil {
				return fmt.Sprintf("credentialRefs contains an unknown account: %d", accountId), nil
			}
			dbSiteId := coerceInt64(mustRowValue(row, "site_id"))
			if dbSiteId != siteId {
				return fmt.Sprintf("credentialRefs default_api_key reference does not match the site: %d", accountId), nil
			}
			apiToken, _ := mustRowValue(row, "api_token").(string)
			if strings.TrimSpace(apiToken) == "" {
				return fmt.Sprintf("credentialRefs default_api_key account has no default API key: %d", accountId), nil
			}
		}
	}

	return "", nil
}

func mustRowValue(row map[string]any, key string) any {
	v, _ := rowValue(row, key)
	return v
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
