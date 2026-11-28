package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
)

type downstreamKeyUpdate struct {
	name, key              string
	description, groupName *string
	tags                   []string
	enabled                bool
	expiresAt              *string
	maxCost                *float64
	maxRequests            *int64
	maxRPM, maxTPM         *int64

	supportedModels        []string
	routeGrants            routeGrantHeal
	siteWeightMultipliers  map[string]float64
	keyWeight              *float64
	excludedSiteIDs        []int64
	excludedCredentialRefs []any
	allowedSiteIDs         []int64
	allowedCredentialRefs  []any

	proxyURL, ipAllowlist, ipBlocklist *string
	accessPolicy                       *string
}

func (h *downstreamKeysHandler) prepareKeyUpdate(w http.ResponseWriter, r *http.Request, id int64, existing map[string]any) *downstreamKeyUpdate {
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
		return nil
	}

	// Use exact JSON field names to distinguish omissions from explicit nulls.
	rawBody := map[string]any{}
	if err := json.Unmarshal(bodyBytes, &rawBody); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return nil
	}
	if _, present := rawBody["ownerUserId"]; present {
		writeError(w, http.StatusBadRequest, "Key ownership is immutable; create a new Key")
		return nil
	}
	if _, present := rawBody["keyOwnerType"]; present {
		writeError(w, http.StatusBadRequest, "Key ownership is immutable; create a new Key")
		return nil
	}

	update := &downstreamKeyUpdate{}
	// Merge: present fields from body, missing fields from existing record.
	update.name = existingString(existing, "name")
	if _, present := rawBody["name"]; present && body.Name != nil {
		update.name = strings.TrimSpace(*body.Name)
	}

	update.key = existingString(existing, "key")
	if _, present := rawBody["key"]; present && body.Key != nil {
		update.key = strings.TrimSpace(*body.Key)
	}

	update.description = existingStringPtr(existing, "description")
	if _, present := rawBody["description"]; present {
		if body.Description != nil && strings.TrimSpace(*body.Description) != "" {
			s := strings.TrimSpace(*body.Description)
			update.description = &s
		} else {
			update.description = nil
		}
	}

	update.groupName = normalizeGroupNameInput(existingStringPtr(existing, "group_name"))
	if _, present := rawBody["groupName"]; present {
		update.groupName = normalizeGroupNameInput(body.GroupName)
	}

	update.tags = parseStringArrayFromDB(existing, "tags")
	if _, present := rawBody["tags"]; present {
		update.tags = normalizeTagsInput(body.Tags)
	}

	update.enabled = existingBool(existing, "enabled")
	if _, present := rawBody["enabled"]; present && body.Enabled != nil {
		update.enabled = *body.Enabled
	}

	update.expiresAt = existingStringPtr(existing, "expires_at")
	if _, present := rawBody["expiresAt"]; present {
		update.expiresAt = normalizeExpiresAt(body.ExpiresAt)
	}

	update.maxCost = existingFloat64Ptr(existing, "max_cost")
	if value, present := rawBody["maxCost"]; present {
		// Explicit clear (null/0/"") → NULL unlimited; positive → set.
		update.maxCost = normalizeQuotaFloatOrNull(value)
	}

	update.maxRequests = existingInt64Ptr(existing, "max_requests")
	update.maxRPM = existingInt64Ptr(existing, "max_rpm")
	update.maxTPM = existingInt64Ptr(existing, "max_tpm")
	if value, present := rawBody["maxRequests"]; present {
		update.maxRequests = normalizeQuotaIntOrNull(value)
	}
	if value, present := rawBody["maxRpm"]; present {
		update.maxRPM = normalizeQuotaIntOrNull(value)
	}
	if value, present := rawBody["maxTpm"]; present {
		update.maxTPM = normalizeQuotaIntOrNull(value)
	}

	update.supportedModels = parseStringArrayFromDB(existing, "supported_models")
	if _, present := rawBody["supportedModels"]; present {
		update.supportedModels = normalizeSupportedModelsInput(body.SupportedModels)
	}

	existingAllowedRouteIds := parseIntArrayFromDB(existing, "allowed_route_ids")
	allowedRouteIds := existingAllowedRouteIds
	if _, present := rawBody["allowedRouteIds"]; present {
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
		return nil
	}
	update.routeGrants = grants

	update.siteWeightMultipliers = parseMapFromDB(existing, "site_weight_multipliers")
	if _, present := rawBody["siteWeightMultipliers"]; present {
		update.siteWeightMultipliers = normalizeSiteWeightMultipliersInput(body.SiteWeightMultipliers)
	}

	update.keyWeight = existingFloat64Ptr(existing, "key_weight")
	if value, present := rawBody["keyWeight"]; present {
		update.keyWeight = normalizeKeyWeightInput(value)
	}

	update.excludedSiteIDs = parseIntArrayFromDB(existing, "excluded_site_ids")
	if _, present := rawBody["excludedSiteIds"]; present {
		update.excludedSiteIDs = normalizeInt64Set(body.ExcludedSiteIds)
	}

	update.excludedCredentialRefs = parseAnyArrayFromDB(existing, "excluded_credential_refs")
	if _, present := rawBody["excludedCredentialRefs"]; present {
		normalized, refErr := normalizeExcludedCredentialRefsInput(body.ExcludedCredentialRefs)
		if refErr != "" {
			writeError(w, http.StatusBadRequest, "excludedCredentialRefs: "+refErr)
			return nil
		}
		update.excludedCredentialRefs = normalized
	}

	update.allowedSiteIDs = parseIntArrayFromDB(existing, "allowed_site_ids")
	if _, present := rawBody["allowedSiteIds"]; present {
		update.allowedSiteIDs = normalizeInt64Set(body.AllowedSiteIds)
	}

	update.allowedCredentialRefs = parseAnyArrayFromDB(existing, "allowed_credential_refs")
	if _, present := rawBody["allowedCredentialRefs"]; present {
		normalized, refErr := normalizeExcludedCredentialRefsInput(body.AllowedCredentialRefs)
		if refErr != "" {
			writeError(w, http.StatusBadRequest, "allowedCredentialRefs: "+refErr)
			return nil
		}
		update.allowedCredentialRefs = normalized
	}

	// proxyUrl: absent keeps existing; present empty/null clears to inherit site/system.
	update.proxyURL = existingStringPtr(existing, "proxy_url")
	if _, present := rawBody["proxyUrl"]; present {
		normalized, proxyErr := normalizeDownstreamProxyURL(body.ProxyURL)
		if proxyErr != "" {
			writeError(w, http.StatusBadRequest, proxyErr)
			return nil
		}
		if normalized != nil && !auth.ReauthConfirmed(r) {
			auth.WriteReauthRequired(w)
			return nil
		}
		update.proxyURL = normalized
	}

	// IP allow/block: absent keeps existing; present empty/null clears to unrestricted.
	update.ipAllowlist = existingStringPtr(existing, "ip_allowlist")
	if _, present := rawBody["ipAllowlist"]; present {
		update.ipAllowlist = normalizeIPListPtr(body.IPAllowlist)
	}
	update.ipBlocklist = existingStringPtr(existing, "ip_blocklist")
	if _, present := rawBody["ipBlocklist"]; present {
		update.ipBlocklist = normalizeIPListPtr(body.IPBlocklist)
	}

	update.accessPolicy = existingStringPtr(existing, "access_policy")
	if _, present := rawBody["accessPolicy"]; present {
		var policyErr error
		update.accessPolicy, policyErr = normalizeDownstreamAccessPolicy(body.AccessPolicy)
		if policyErr != nil {
			writeError(w, http.StatusBadRequest, policyErr.Error())
			return nil
		}
	}
	if update.name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return nil
	}
	if update.key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return nil
	}
	if update.key != existingString(existing, "key") && !h.allowKeyRotation(w, r, id, update.key) {
		return nil
	}
	// Policy reference validation.
	// Validation vouches for what this save is responsible for: live grants and
	// ids the request introduced. Grants left behind by a route deletion that
	// cannot be pruned without widening the key are reported instead of rejected,
	// which is what made such a key uneditable before.
	refErr, refDbErr := h.validateDownstreamPolicyReferences(update.routeGrants.Validate, update.siteWeightMultipliers, update.excludedSiteIDs, update.excludedCredentialRefs, update.allowedSiteIDs, update.allowedCredentialRefs)
	if refDbErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate policy references")
		return nil
	}
	if refErr != "" {
		writeError(w, http.StatusBadRequest, refErr)
		return nil
	}
	return update
}
