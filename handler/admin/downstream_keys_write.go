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
	update := h.prepareKeyUpdate(w, r, id, existing)
	if update == nil {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON := toPersistenceJSON(update.tags)
	modelsJSON := toPersistenceJSON(update.supportedModels)
	routeIdsJSON := toPersistenceJSON(update.routeGrants.Persist)
	swmJSON := toPersistenceJSON(update.siteWeightMultipliers)
	excludedSitesJSON := toPersistenceJSON(update.excludedSiteIDs)
	credRefsJSON := toPersistenceJSON(update.excludedCredentialRefs)
	allowedSitesJSON := toPersistenceJSON(update.allowedSiteIDs)
	allowedCredRefsJSON := toPersistenceJSON(update.allowedCredentialRefs)
	_, err = h.db.Exec(
		`UPDATE downstream_api_keys SET
			name = ?, key = ?, description = ?, group_name = ?, tags = ?,
			enabled = ?, expires_at = ?, max_cost = ?, max_requests = ?, max_rpm = ?, max_tpm = ?,
			supported_models = ?, allowed_route_ids = ?, site_weight_multipliers = ?, key_weight = ?,
			excluded_site_ids = ?, excluded_credential_refs = ?,
				allowed_site_ids = ?, allowed_credential_refs = ?, proxy_url = ?, ip_allowlist = ?, ip_blocklist = ?, access_policy = ?, updated_at = ?
		WHERE id = ?`,
		update.name, update.key, update.description, update.groupName, tagsJSON,
		update.enabled, update.expiresAt, update.maxCost, update.maxRequests, update.maxRPM, update.maxTPM,
		modelsJSON, routeIdsJSON, swmJSON, update.keyWeight,
		excludedSitesJSON, credRefsJSON,
		allowedSitesJSON, allowedCredRefsJSON, update.proxyURL, update.ipAllowlist, update.ipBlocklist, update.accessPolicy, now, id,
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
	if len(update.routeGrants.Healed) > 0 {
		// The save succeeded and the allowlist got smaller; say so rather than
		// let the caller believe it wrote the list it sent.
		response["prunedRouteIds"] = update.routeGrants.Healed
		slog.Warn("downstream key update dropped route grants that no longer exist",
			"downstreamKeyId", id, "prunedRouteIds", update.routeGrants.Healed)
	}
	if len(update.routeGrants.Stale) > 0 {
		response["staleRouteIds"] = update.routeGrants.Stale
		response["staleRouteGrantNotice"] = "this key authorizes only routes that no longer exist, so it serves nothing; the grants were kept because dropping them would widen the key to every route — replace them with live routes, or clear the list to allow all routes"
		slog.Warn("downstream key still authorizes only deleted routes",
			"downstreamKeyId", id, "staleRouteIds", update.routeGrants.Stale)
	}
	writeJSON(w, http.StatusOK, response)
}
