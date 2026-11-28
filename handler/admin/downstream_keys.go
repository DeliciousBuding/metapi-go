package admin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterDownstreamKeysRoutes registers all /api/downstream-keys routes.
func RegisterDownstreamKeysRoutes(r chi.Router, db *store.DB) {
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
	db *store.DB
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
	rows, err := h.queryPolicyIDs("SELECT id FROM token_routes WHERE id IN", requested)
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

// queryPolicyIDs binds the non-empty ID list supplied by the policy checks.
func (h *downstreamKeysHandler) queryPolicyIDs(query string, ids []int64) ([]map[string]any, error) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return queryRowsErr(h.db.DB, query+" ("+sqlPlaceholders(len(ids))+")", args...)
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
		rows, qErr := h.queryPolicyIDs("SELECT id FROM token_routes WHERE id IN", allowedRouteIds)
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
		rows, qErr := h.queryPolicyIDs("SELECT id FROM sites WHERE id IN", ids)
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

	// Validate excludedCredentialRefs + allowedCredentialRefs.
	for _, ref := range append(append([]any{}, excludedCredentialRefs...), allowedCredentialRefs...) {
		obj, ok := ref.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := obj["kind"].(string)
		if kind == "direct_grant" {
			grantID := coerceInt64(obj["grantId"])
			var count int
			if err := h.db.Get(&count, `SELECT COUNT(*) FROM upstream_grants WHERE id = ?`, grantID); err != nil {
				return "", err
			}
			if count != 1 {
				return fmt.Sprintf("credentialRefs contains an unknown direct grant: %d", grantID), nil
			}
		} else if kind == "account_token" {
			tokenId := coerceInt64(obj["tokenId"])
			accountId := coerceInt64(obj["accountId"])
			siteId := coerceInt64(obj["siteId"])
			if tokenId <= 0 {
				return fmt.Sprintf("credentialRefs contains an invalid token reference: %v", obj["tokenId"]), nil
			}
			row, err := queryRow(h.db,
				`SELECT at.id as token_id, at.account_id, a.site_id
				 FROM account_tokens at
				 INNER JOIN accounts a ON at.account_id = a.id
				 WHERE at.id = ?`, tokenId)
			if err != nil {
				return "", err
			}
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
			row, err := queryRow(h.db,
				`SELECT id as account_id, site_id, api_token
				 FROM accounts WHERE id = ?`, accountId)
			if err != nil {
				return "", err
			}
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

func normalizeDownstreamAccessPolicy(raw json.RawMessage) (*string, error) {
	p, err := store.ParseDownstreamAccessPolicy(string(raw))
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	b, err := json.Marshal(p)
	s := string(b)
	return &s, err
}
