package admin

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
)

// POST /api/downstream-keys
func (h *downstreamKeysHandler) createKey(w http.ResponseWriter, r *http.Request) {
	// maxCost/maxRequests use any so clients can send number|string|null (TS parity).
	// Explicit null/0/"" clears to unlimited (NULL); omitted also stores NULL on create.
	var body struct {
		OwnerUserID            *int64          `json:"ownerUserId"`
		AccessPolicy           json.RawMessage `json:"accessPolicy"`
		Name                   string          `json:"name"`
		Key                    string          `json:"key"`
		Description            *string         `json:"description"`
		GroupName              *string         `json:"groupName"`
		Tags                   []string        `json:"tags"`
		Enabled                *bool           `json:"enabled"`
		ExpiresAt              *string         `json:"expiresAt"`
		MaxCost                any             `json:"maxCost"`
		MaxRequests            any             `json:"maxRequests"`
		MaxRpm                 any             `json:"maxRpm"`
		MaxTpm                 any             `json:"maxTpm"`
		SupportedModels        []string        `json:"supportedModels"`
		AllowedRouteIds        []int64         `json:"allowedRouteIds"`
		SiteWeightMultipliers  any             `json:"siteWeightMultipliers"`
		KeyWeight              any             `json:"keyWeight"`
		ExcludedSiteIds        []int64         `json:"excludedSiteIds"`
		ExcludedCredentialRefs []any           `json:"excludedCredentialRefs"`
		AllowedSiteIds         []int64         `json:"allowedSiteIds"`
		AllowedCredentialRefs  []any           `json:"allowedCredentialRefs"`
		ProxyURL               *string         `json:"proxyUrl"`
		IPAllowlist            *string         `json:"ipAllowlist"`
		IPBlocklist            *string         `json:"ipBlocklist"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Key = strings.TrimSpace(body.Key)
	ownerType := "system"
	if body.OwnerUserID != nil {
		if *body.OwnerUserID <= 0 {
			writeError(w, http.StatusBadRequest, "ownerUserId must be a positive user ID")
			return
		}
		var userID int64
		if err := h.db.Get(&userID, `SELECT u.id FROM gateway_users u JOIN gateway_user_groups g ON g.id=u.group_id WHERE u.id=? AND u.enabled=TRUE AND g.enabled=TRUE`, *body.OwnerUserID); err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusBadRequest, "user or group is unavailable")
			} else {
				writeError(w, http.StatusInternalServerError, "failed to validate Key owner")
			}
			return
		}
		ownerType = "user"
	}

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
	h.db.Get(&count, "SELECT COUNT(*) FROM downstream_api_keys WHERE key = ?", body.Key)
	if count > 0 {
		writeError(w, http.StatusConflict, "API key already exists")
		return
	}

	accessPolicy, policyErr := normalizeDownstreamAccessPolicy(body.AccessPolicy)
	if policyErr != nil {
		writeError(w, http.StatusBadRequest, policyErr.Error())
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

	// Per-key egress proxy. NULL/empty inherits site/system. A non-empty value
	// reroutes this key's upstream traffic, so it needs the master token (the
	// route-level gate also covers POST, this keeps the handler safe if the
	// route is mounted elsewhere).
	proxyURL, proxyErr := normalizeDownstreamProxyURL(body.ProxyURL)
	if proxyErr != "" {
		writeError(w, http.StatusBadRequest, proxyErr)
		return
	}
	if proxyURL != nil && !auth.ReauthConfirmed(r) {
		auth.WriteReauthRequired(w)
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
		 proxy_url, max_rpm, max_tpm, ip_allowlist, ip_blocklist, access_policy, created_at, updated_at, key_owner_type, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		body.Name, body.Key, desc, normalizedGroupName, tagsJSON, enabled, body.ExpiresAt,
		maxCost, maxRequests,
		modelsJSON, routeIdsJSON, swmJSON, keyWeight, excludedSitesJSON, credRefsJSON,
		allowedSitesJSON, allowedCredRefsJSON,
		proxyURL, maxRpm, maxTpm, ipAllowlist, ipBlocklist, accessPolicy, now, now, ownerType, body.OwnerUserID,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "API key already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "creation failed")
		return
	}
	created, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if err != nil || created == nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "API key created but failed to load result")
		return
	}
	if key, ok := created["key"].(string); ok {
		created["keyMasked"] = maskSecret(key)
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

	existing, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load API key")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}

	// Parse body into typed struct with pointers to detect field presence.
	// nil pointer = field not present in JSON body for most fields.
	// maxCost/maxRequests are decoded from the raw map so number|string|null all work.
	var body struct {
		AccessPolicy           json.RawMessage `json:"accessPolicy"`
		Name                   *string         `json:"name"`
		Key                    *string         `json:"key"`
		Description            *string         `json:"description"`
		GroupName              *string         `json:"groupName"`
		Tags                   []string        `json:"tags"`
		Enabled                *bool           `json:"enabled"`
		ExpiresAt              *string         `json:"expiresAt"`
		SupportedModels        []string        `json:"supportedModels"`
		AllowedRouteIds        []int64         `json:"allowedRouteIds"`
		SiteWeightMultipliers  any             `json:"siteWeightMultipliers"`
		KeyWeight              any             `json:"keyWeight"`
		ExcludedSiteIds        []int64         `json:"excludedSiteIds"`
		ExcludedCredentialRefs []any           `json:"excludedCredentialRefs"`
		AllowedSiteIds         []int64         `json:"allowedSiteIds"`
		AllowedCredentialRefs  []any           `json:"allowedCredentialRefs"`
		ProxyURL               *string         `json:"proxyUrl"`
		IPAllowlist            *string         `json:"ipAllowlist"`
		IPBlocklist            *string         `json:"ipBlocklist"`
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
	if _, present := rawBody["ownerUserId"]; present {
		writeError(w, http.StatusBadRequest, "Key ownership is immutable; create a new Key")
		return
	}
	if _, present := rawBody["keyOwnerType"]; present {
		writeError(w, http.StatusBadRequest, "Key ownership is immutable; create a new Key")
		return
	}

	hasField := make(map[string]bool)
	if _, ok := rawBody["accessPolicy"]; ok {
		hasField["accessPolicy"] = true
	}
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
	// transactional route-grant pruning existed left its id behind here, and
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
		if normalized != nil && !auth.ReauthConfirmed(r) {
			auth.WriteReauthRequired(w)
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

	accessPolicy := existingStringPtr(existing, "access_policy")
	if hasField["accessPolicy"] {
		var policyErr error
		accessPolicy, policyErr = normalizeDownstreamAccessPolicy(body.AccessPolicy)
		if policyErr != nil {
			writeError(w, http.StatusBadRequest, policyErr.Error())
			return
		}
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
	if key != existingString(existing, "key") && (!strings.HasPrefix(key, "sk-") || len(key) < 6) {
		writeError(w, http.StatusBadRequest, "key must start with sk- and be at least 6 characters long")
		return
	}

	// Duplicate key check: if key is being changed, verify new key doesn't collide.
	if hasField["key"] && key != existingString(existing, "key") {
		// Rotating the bearer secret from a live session mints a credential that
		// survives session revocation, so it keeps the re-confirmation it had
		// under the route-level gate (only proxyUrl/key rotation are gated here;
		// ordinary metadata edits save from the session).
		if !auth.ReauthConfirmed(r) {
			auth.WriteReauthRequired(w)
			return
		}
		var dupCount int
		h.db.Get(&dupCount, "SELECT COUNT(*) FROM downstream_api_keys WHERE key = ? AND id != ?", key, id)
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
		`UPDATE downstream_api_keys SET
			name = ?, key = ?, description = ?, group_name = ?, tags = ?,
			enabled = ?, expires_at = ?, max_cost = ?, max_requests = ?, max_rpm = ?, max_tpm = ?,
			supported_models = ?, allowed_route_ids = ?, site_weight_multipliers = ?, key_weight = ?,
			excluded_site_ids = ?, excluded_credential_refs = ?,
				allowed_site_ids = ?, allowed_credential_refs = ?, proxy_url = ?, ip_allowlist = ?, ip_blocklist = ?, access_policy = ?, updated_at = ?
		WHERE id = ?`,
		name, key, description, groupName, tagsJSON,
		enabled, expiresAt, maxCost, maxRequests, maxRpm, maxTpm,
		modelsJSON, routeIdsJSON, swmJSON, keyWeight,
		excludedSitesJSON, credRefsJSON,
		allowedSitesJSON, allowedCredRefsJSON, proxyURL, ipAllowlist, ipBlocklist, accessPolicy, now, id,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "API key already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	updated, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if err != nil || updated == nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "API key updated but failed to load result")
		return
	}
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

// POST /api/downstream-keys/:id/reset-usage
func (h *downstreamKeysHandler) resetUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := h.db.Exec("UPDATE downstream_api_keys SET used_cost = 0, used_requests = 0, updated_at = ? WHERE id = ?", now, id); err != nil {
		writeError(w, http.StatusInternalServerError, "reset failed")
		return
	}

	updated, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "API key usage reset but failed to load result")
		return
	}
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

	if _, err := h.db.Exec("DELETE FROM downstream_api_keys WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
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
		row, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "failed to load API key"})
			continue
		}
		if row == nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "API key not found"})
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		switch action {
		case "delete":
			if _, err := h.db.Exec("DELETE FROM downstream_api_keys WHERE id = ?", id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "delete failed"})
				continue
			}
		case "resetUsage":
			if _, err := h.db.Exec("UPDATE downstream_api_keys SET used_cost = 0, used_requests = 0, updated_at = ? WHERE id = ?", now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "reset failed"})
				continue
			}
		case "enable":
			if _, err := h.db.Exec("UPDATE downstream_api_keys SET enabled = ?, updated_at = ? WHERE id = ?", true, now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "enable failed"})
				continue
			}
		case "disable":
			if _, err := h.db.Exec("UPDATE downstream_api_keys SET enabled = ?, updated_at = ? WHERE id = ?", false, now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "disable failed"})
				continue
			}
		case "updateMetadata":
			if body.GroupOperation != nil && *body.GroupOperation == "set" && body.GroupName != nil {
				if _, err := h.db.Exec("UPDATE downstream_api_keys SET group_name = ?, updated_at = ? WHERE id = ?", *body.GroupName, now, id); err != nil {
					failedItems = append(failedItems, map[string]any{"id": id, "message": "metadata update failed"})
					continue
				}
			} else if body.GroupOperation != nil && *body.GroupOperation == "clear" {
				if _, err := h.db.Exec("UPDATE downstream_api_keys SET group_name = NULL, updated_at = ? WHERE id = ?", now, id); err != nil {
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
