package admin

import (
	"errors"
	"strings"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// buildAccountUpdate merges the patch against the locked row and determines whether
// model refresh must succeed before an expired API-key account can be reactivated.
func buildAccountUpdate(row *service.AccountWithSite, body payloads.AccountUpdatePayload) (map[string]any, bool, error) {
	// #1176: credentialMode is an explicit operator choice on the edit form.
	// Resolve it before the credential fields are touched so the api_token
	// mirror (an apikey-mode convenience) cannot stamp a session credential as
	// an API key, and so the mode that gets persisted is the one requested.
	var requestedMode service.AccountCredentialMode
	hasRequestedMode := false
	if body.CredentialMode != nil {
		mode := service.NormalizeCredentialMode(*body.CredentialMode)
		if mode != service.CredentialModeSession && mode != service.CredentialModeAPIKey {
			return nil, false, errors.New("Invalid credentialMode. Expected session or apikey.")
		}
		requestedMode, hasRequestedMode = mode, true
	}

	updates := map[string]any{}
	pendingExtraConfig := row.Account.ExtraConfig
	mergeExtraConfigUpdate := func(patch map[string]any) {
		merged := service.MergeExtraConfig(pendingExtraConfig, patch)
		if merged != nil {
			pendingExtraConfig = merged
			updates["extraConfig"] = *merged
		}
	}
	if body.Username != nil {
		updates["username"] = *body.Username
	}
	mirrorAPIKeyToken := false
	explicitSessionSwitch := hasRequestedMode && requestedMode == service.CredentialModeSession
	if body.AccessToken != nil {
		nextAccessToken := strings.TrimSpace(*body.AccessToken)
		updates["accessToken"] = nextAccessToken
		// A session JWT is not an API key: skip the apikey mirror when the
		// operator explicitly moved this account to session mode (#1176).
		mirrorAPIKeyToken = !explicitSessionSwitch && shouldMirrorAPIKeyToken(row.Account, body.APIToken)
		if mirrorAPIKeyToken {
			updates["apiToken"] = nextAccessToken
		}
	}
	if body.APIToken != nil && !mirrorAPIKeyToken {
		updates["apiToken"] = normalizeAPITokenUpdate(body.APIToken)
	}
	if body.Status != nil {
		status := normalizeAccountStatus(*body.Status)
		if status == "" {
			return nil, false, errors.New("Invalid account status. Expected active, disabled, or expired.")
		}
		updates["status"] = status
	}
	if body.UnitCost != nil {
		if *body.UnitCost <= 0 {
			return nil, false, errors.New("Invalid unitCost value. Expected positive number.")
		}
		updates["unitCost"] = *body.UnitCost
	}
	if body.ExtraConfig != nil {
		// Merge with existing extraConfig to preserve keys not in the update.
		// For sub2api, strip nested sub2apiAuth here and merge it via managed-auth
		// helpers later so partial patches cannot clobber unrelated auth keys.
		if ecMap, ok := body.ExtraConfig.(map[string]any); ok {
			if service.IsSub2ApiPlatform(row.Site.Platform) {
				stripped := make(map[string]any, len(ecMap))
				for k, v := range ecMap {
					if k == "sub2apiAuth" {
						continue
					}
					stripped[k] = v
				}
				if len(stripped) > 0 {
					mergeExtraConfigUpdate(stripped)
				}
			} else {
				mergeExtraConfigUpdate(ecMap)
			}
		}
	}
	nextAccount := row.Account
	nextAccount.ExtraConfig = pendingExtraConfig
	if accessToken, ok := updates["accessToken"].(string); ok {
		nextAccount.AccessToken = accessToken
	}
	if apiToken, ok := updates["apiToken"].(*string); ok {
		nextAccount.APIToken = apiToken
	} else if apiTokenStr, ok := updates["apiToken"].(string); ok {
		token := apiTokenStr
		nextAccount.APIToken = &token
	}
	if status, ok := updates["status"].(string); ok {
		nextAccount.Status = status
	}
	if service.BuildCapabilitiesForAccount(&nextAccount).CanCheckin {
		if body.CheckinEnabled != nil {
			updates["checkinEnabled"] = *body.CheckinEnabled
		}
	} else if row.Account.CheckinEnabled || body.CheckinEnabled != nil {
		updates["checkinEnabled"] = false
	}
	if body.IsPinned != nil {
		updates["isPinned"] = *body.IsPinned
	}
	if body.SortOrder != nil {
		so := service.NormalizeSortOrder(body.SortOrder)
		if so == nil {
			return nil, false, errors.New("Invalid sortOrder value. Expected non-negative integer.")
		}
		updates["sortOrder"] = int64(*so)
	}
	if body.ProxyURL != nil {
		// Validation and the reauth gate ran before the transaction; here the
		// field only needs the #1009 deletion semantics: an explicitly cleared
		// proxyUrl must persist as a deletion (issue #1009 residual).
		// MergeExtraConfig deletes keys whose patch value is an untyped nil, so
		// build the patch with a nil interface rather than a typed
		// (*string)(nil), which would marshal to "proxyUrl": null.
		var proxyPatch any
		if *body.ProxyURL != "" {
			proxyPatch = *body.ProxyURL
		}
		mergeExtraConfigUpdate(map[string]any{"proxyUrl": proxyPatch})
	}
	// credentialMode lives in extraConfig (same storage as the create path).
	// Merged after body.ExtraConfig so the explicit top-level field wins over a
	// nested copy in the same request, mirroring how proxyUrl is handled.
	if hasRequestedMode {
		mergeExtraConfigUpdate(map[string]any{"credentialMode": string(requestedMode)})
	}
	// PlatformUserID and skipModelFetch live in extraConfig (same storage as
	// the create path); merge instead of dropping the form fields.
	if body.PlatformUserID != nil {
		mergeExtraConfigUpdate(map[string]any{"platformUserId": *body.PlatformUserID})
	}
	if body.SkipModelFetch != nil {
		mergeExtraConfigUpdate(map[string]any{"skipModelFetch": *body.SkipModelFetch})
	}
	// Tags persist to the accounts.tags JSON-array column with the same
	// normalization as the dedicated PUT /api/accounts/{id}/tags endpoint
	// (trim + dedupe + drop empties).
	if body.Tags != nil {
		updates["tags"] = store.EncodeTags(store.NormalizeTags(*body.Tags))
	}
	// Remark is a free-form human-readable note. An empty/whitespace string
	// clears the column (NULL); otherwise the trimmed value is persisted.
	// Do not store credentials here — remark is returned in plaintext on
	// admin list/search responses (unlike accessToken/apiToken which are masked).
	if body.Remark != nil {
		updates["remark"] = service.NormalizeNullable(body.Remark)
	}
	// Sub2API managed auth: merge top-level / nested refreshToken+tokenExpiresAt
	// into extraConfig.sub2apiAuth without clobbering unrelated keys.
	if service.IsSub2ApiPlatform(row.Site.Platform) {
		var extraPatch map[string]any
		if body.ExtraConfig != nil {
			if ecMap, ok := body.ExtraConfig.(map[string]any); ok {
				extraPatch = ecMap
			}
		}
		// Merge against the original stored config so nested extraConfig.sub2apiAuth
		// patches compose with top-level fields instead of overwriting wholesale.
		if mergedAuth := service.BuildMergedSub2ApiAuth(row.Account.ExtraConfig, body.RefreshToken, body.TokenExpiresAt, extraPatch); mergedAuth != nil {
			mergeExtraConfigUpdate(map[string]any{"sub2apiAuth": mergedAuth})
		}
	}
	// Fail loud instead of persisting a mode the stored credential cannot back:
	// such an account can never authenticate, and the edit dialog would keep
	// showing a mode that lies about what is on the row (#1176).
	if hasRequestedMode {
		hasSession := strings.TrimSpace(nextAccount.AccessToken) != ""
		hasAPIKey := nextAccount.APIToken != nil && strings.TrimSpace(*nextAccount.APIToken) != ""
		if requestedMode == service.CredentialModeSession && !hasSession {
			return nil, false, errors.New("credentialMode session requires a session credential: send accessToken in the same request.")
		}
		if requestedMode == service.CredentialModeAPIKey && !hasAPIKey {
			return nil, false, errors.New("credentialMode apikey requires an API key: send apiToken in the same request.")
		}
	}

	// Expired API-key recovery (TS accounts.ts parity):
	// when credentials change on an expired apikey account and status is not forced
	// disabled, refresh models with allowInactive and reactivate only on success.
	recovery := shouldRecoverExpiredAPIKey(row.Account, nextAccount, updates)
	if recovery {
		// preserveExpiredStatus: do not activate until model refresh succeeds.
		if status, ok := updates["status"].(string); ok && status == "active" {
			delete(updates, "status")
		}
	}
	return updates, recovery, nil
}
