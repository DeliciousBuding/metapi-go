package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	errNewAPITokenListingUnavailable = errors.New("new-api token listing unavailable")

	// ErrNewAPITokenListingCompletenessUnproven means the legacy listing
	// answered but did not expose a total that can prove the snapshot complete.
	// Callers may still use the tokens for synchronization, but must not use
	// their absence to converge the default relay credential.
	ErrNewAPITokenListingCompletenessUnproven = errors.New("new-api token listing completeness unproven")
)

const (
	// Bound the complete-list fetch independently of the remote total. The
	// limits cover the normal 1000-token account without letting a hostile or
	// broken upstream keep returning new IDs forever.
	newAPITokenListMaxItems        = 1000
	newAPITokenListMaxPages        = 10
	newAPITokenListMaxDecodedBytes = 8 << 20
)

// --- Token CRUD ---

// apiTokenListPath is the legacy one-api token listing path. Keep its p=0/size
// spelling: one-api's older query contract is not the New API p/page_size
// contract below.
func apiTokenListPath() string {
	return fmt.Sprintf("/api/token/?p=0&size=%d", UpstreamTokenListPageLimit)
}

// newAPITokenListPath uses New API's current one-based page and page_size
// contract. page_size is capped at 100 upstream, so this is also the ownership
// batch size used by /api/token/batch/keys.
func newAPITokenListPath(page int) string {
	if page < 1 {
		page = 1
	}
	return fmt.Sprintf("/api/token/?p=%d&page_size=%d", page, UpstreamTokenListPageLimit)
}

func (n *NewApiAdapter) GetAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := n.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		return nil, err
	}
	return findFirstEnabledToken(tokens), nil
}

func (n *NewApiAdapter) GetAPITokens(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	tokens, _, err := n.getAPITokensWithUserComplete(ctx, baseURL, accessToken, platformUserId, proxy)
	if errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		return tokens, nil
	}
	return tokens, err
}

// GetAPITokensComplete is the explicit completeness contract consumed by the
// token-sync convergence path. A legacy flat response without a total is
// returned with ErrNewAPITokenListingCompletenessUnproven so callers can sync
// the tokens but must not converge the default from their absence.
func (n *NewApiAdapter) GetAPITokensComplete(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, bool, error) {
	return n.getAPITokensWithUserComplete(ctx, baseURL, accessToken, platformUserId, proxy)
}

func (n *NewApiAdapter) getAPITokenWithUser(ctx context.Context, baseURL, accessToken string, userID *int, proxy *ProxyConfig) (*string, error) {
	tokens, _, err := n.getAPITokensWithUserComplete(ctx, baseURL, accessToken, userID, proxy)
	if errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		err = nil
	}
	if err != nil {
		if errors.Is(err, errNewAPITokenListingUnavailable) {
			return nil, nil
		}
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	return findFirstEnabledToken(tokens), nil
}

func (n *NewApiAdapter) getAPITokensWithUserComplete(ctx context.Context, baseURL, accessToken string, userID *int, proxy *ProxyConfig) ([]ApiTokenInfo, bool, error) {
	// Try Bearer auth. A successful first page is authoritative: later-page or
	// ownership-hydration failures must surface, not fall through to another
	// credential shape and return a partial list.
	headers := n.authHeaders(accessToken, userID)
	tokens, started, complete, err := n.fetchNewAPITokens(ctx, baseURL, headers, proxy)
	if err == nil || errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		return tokens, complete, err
	}
	if started {
		return nil, false, err
	}

	// Cookie fallback is a transport-shape compatibility path for the same
	// stored credential. Do not probe a different user identity after failure.
	cookieTokens, cookieStarted, cookieComplete, cookieErr := n.getAPITokensByCookie(ctx, baseURL, accessToken, userID, proxy)
	if cookieErr == nil || errors.Is(cookieErr, ErrNewAPITokenListingCompletenessUnproven) {
		return cookieTokens, cookieComplete, cookieErr
	}
	if cookieStarted {
		return nil, false, cookieErr
	}

	if cookieErr != nil {
		err = cookieErr
	}
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", errNewAPITokenListingUnavailable, err)
	}
	return []ApiTokenInfo{}, false, nil
}

func (n *NewApiAdapter) getAPITokensByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) ([]ApiTokenInfo, bool, bool, error) {
	var lastErr error
	for _, cookie := range buildCookieCandidates(token) {
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(userID) {
			headers[k] = v
		}

		tokens, started, complete, err := n.fetchNewAPITokens(ctx, baseURL, headers, proxy)
		if err == nil || errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
			return tokens, started, complete, err
		}
		if started {
			return nil, true, false, err
		}
		lastErr = err
	}
	return nil, false, false, lastErr
}

func (n *NewApiAdapter) fetchNewAPITokens(ctx context.Context, baseURL string, headers map[string]string, proxy *ProxyConfig) ([]ApiTokenInfo, bool, bool, error) {
	items, started, complete, err := n.fetchNewAPITokenItems(ctx, baseURL, headers, proxy)
	if err != nil && !errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		return nil, started, false, err
	}
	normalized, err := n.normalizeListedTokens(ctx, baseURL, items, headers, proxy)
	if err != nil {
		return nil, true, false, err
	}
	if complete {
		return normalized, true, true, nil
	}
	return normalized, true, false, ErrNewAPITokenListingCompletenessUnproven
}

func (n *NewApiAdapter) fetchNewAPITokenItems(ctx context.Context, baseURL string, headers map[string]string, proxy *ProxyConfig) ([]map[string]interface{}, bool, bool, error) {
	all := make([]map[string]interface{}, 0)
	seenIDs := make(map[int]struct{})
	total := -1
	started := false
	decodedBytes := 0

	for page := 1; ; page++ {
		if page > newAPITokenListMaxPages {
			return nil, started, false, fmt.Errorf("list upstream tokens: page limit %d exceeded", newAPITokenListMaxPages)
		}
		resp, err := fetchJSON(ctx, strings.TrimRight(baseURL, "/")+newAPITokenListPath(page), http.MethodGet, nil, headers, proxy)
		if err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: %w", page, err)
		}
		started = true

		if success, present := getBool(resp, "success"); present && !success {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: upstream refused the listing", page)
		}

		items, flat, present, err := newAPITokenPageItems(resp)
		if err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: %w", page, err)
		}
		if !present {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: response has no supported token collection", page)
		}

		pageTotal, hasTotal, err := newAPITokenPageTotal(resp)
		if err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: %w", page, err)
		}
		// A flat response to the current p/page_size query has no page marker
		// that proves it is the first page. Retry the known legacy query before
		// using any of it; never treat this current-query payload as complete.
		if page == 1 && !hasTotal && flat {
			return n.fetchLegacyNewAPITokenItems(ctx, baseURL, headers, proxy)
		}
		if page == 1 {
			if hasTotal {
				total = pageTotal
				if total > newAPITokenListMaxItems {
					return nil, started, false, fmt.Errorf("list upstream tokens: total %d exceeds limit %d", total, newAPITokenListMaxItems)
				}
			} else if !flat {
				return nil, started, false, fmt.Errorf("list upstream tokens page 1: response has no total")
			}
		} else if !hasTotal || pageTotal != total {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: total changed or missing", page)
		}

		if len(items) > newAPITokenListMaxItems-len(all) {
			return nil, started, false, fmt.Errorf("list upstream tokens: item count exceeds limit %d", newAPITokenListMaxItems)
		}
		pageBytes, err := json.Marshal(items)
		if err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: measure decoded items: %w", page, err)
		}
		if len(pageBytes) > newAPITokenListMaxDecodedBytes-decodedBytes {
			return nil, started, false, fmt.Errorf("list upstream tokens: decoded items exceed limit %d bytes", newAPITokenListMaxDecodedBytes)
		}
		decodedBytes += len(pageBytes)
		if err := appendNewAPITokenItems(&all, seenIDs, items); err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: %w", page, err)
		}

		if total >= 0 {
			if len(all) > total {
				return nil, started, false, fmt.Errorf("list upstream tokens: returned %d tokens beyond total %d", len(all), total)
			}
			if len(all) == total {
				return all, started, true, nil
			}
			if len(items) < UpstreamTokenListPageLimit {
				return nil, started, false, fmt.Errorf("list upstream tokens page %d: short page before total %d", page, total)
			}
			continue
		}

		return nil, started, false, fmt.Errorf("legacy token listing has no total")
	}
}

// fetchLegacyNewAPITokenItems retries the recognized One API/New API legacy
// first-page contract after a flat response to the current query. Without a
// total, the returned tokens are still usable for synchronization but are not
// proof that the listing is complete.
func (n *NewApiAdapter) fetchLegacyNewAPITokenItems(ctx context.Context, baseURL string, headers map[string]string, proxy *ProxyConfig) ([]map[string]interface{}, bool, bool, error) {
	resp, err := fetchJSON(ctx, strings.TrimRight(baseURL, "/")+apiTokenListPath(), http.MethodGet, nil, headers, proxy)
	if err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: %w", err)
	}
	if success, present := getBool(resp, "success"); present && !success {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: upstream refused the listing")
	}
	items, _, present, err := newAPITokenPageItems(resp)
	if err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: %w", err)
	}
	if !present {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: response has no supported token collection")
	}
	if len(items) > newAPITokenListMaxItems {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: item count exceeds limit %d", newAPITokenListMaxItems)
	}
	pageBytes, err := json.Marshal(items)
	if err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: measure decoded items: %w", err)
	}
	if len(pageBytes) > newAPITokenListMaxDecodedBytes {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: decoded items exceed limit %d bytes", newAPITokenListMaxDecodedBytes)
	}

	all := make([]map[string]interface{}, 0, len(items))
	if err := appendNewAPITokenItems(&all, make(map[int]struct{}), items); err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: %w", err)
	}

	total, hasTotal, err := newAPITokenPageTotal(resp)
	if err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: %w", err)
	}
	if hasTotal {
		if total > newAPITokenListMaxItems {
			return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: total %d exceeds limit %d", total, newAPITokenListMaxItems)
		}
		if len(items) > total {
			return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: returned %d tokens beyond total %d", len(items), total)
		}
		if len(items) == total {
			return all, true, true, nil
		}
	}
	return all, true, false, ErrNewAPITokenListingCompletenessUnproven
}

func newAPITokenPageItems(resp map[string]interface{}) ([]map[string]interface{}, bool, bool, error) {
	if data, ok := resp["data"].([]interface{}); ok {
		items, err := tokenMapsFromList(data)
		return items, true, true, err
	}

	var candidates []interface{}
	if data, ok := getMap(resp, "data"); ok {
		candidates = append(candidates, data["items"], data["data"], data["list"])
	}
	candidates = append(candidates, resp["items"], resp["list"])
	for _, raw := range candidates {
		list, ok := raw.([]interface{})
		if !ok {
			continue
		}
		items, err := tokenMapsFromList(list)
		return items, false, true, err
	}
	return nil, false, false, nil
}

func tokenMapsFromList(list []interface{}) ([]map[string]interface{}, error) {
	items := make([]map[string]interface{}, 0, len(list))
	for i, raw := range list {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("token item %d is not an object", i+1)
		}
		items = append(items, item)
	}
	return items, nil
}

func newAPITokenPageTotal(resp map[string]interface{}) (int, bool, error) {
	total, ok := getFloat(resp, "total")
	if !ok {
		data, hasData := getMap(resp, "data")
		if hasData {
			total, ok = getFloat(data, "total")
		}
	}
	if !ok {
		return 0, false, nil
	}
	if total < 0 || total != float64(int(total)) {
		return 0, false, fmt.Errorf("invalid total %v", total)
	}
	return int(total), true, nil
}

func appendNewAPITokenItems(all *[]map[string]interface{}, seenIDs map[int]struct{}, items []map[string]interface{}) error {
	for i, item := range items {
		key, ok := getString(item, "key")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("token item %d has no key", i+1)
		}

		if rawID, exists := item["id"]; exists {
			id, ok := rawID.(float64)
			if !ok || id <= 0 || id != float64(int(id)) {
				return fmt.Errorf("token item %d has an invalid id", i+1)
			}
			if _, exists := seenIDs[int(id)]; exists {
				return fmt.Errorf("duplicate token id %d", int(id))
			}
			seenIDs[int(id)] = struct{}{}
		}
		*all = append(*all, item)
	}
	return nil
}

// validateNewAPITokenKeys runs after masked display values have been hydrated.
// IDs are the identity while listing; real keys are the identity once the
// ownership-checked hydration has replaced every mask.
func validateNewAPITokenKeys(items []map[string]interface{}) error {
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		key, ok := getString(item, "key")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("token item %d has no key", i+1)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate token entry")
		}
		seen[key] = struct{}{}
	}
	return nil
}

// normalizeListedTokens resolves New API v1's masked display keys before
// returning routing credentials. The same hydration owns deletion identity.
func (n *NewApiAdapter) normalizeListedTokens(
	ctx context.Context,
	baseURL string,
	items []map[string]interface{},
	headers map[string]string,
	proxy *ProxyConfig,
) ([]ApiTokenInfo, error) {
	if err := n.hydrateListedTokenKeys(ctx, baseURL, items, headers, proxy); err != nil {
		return nil, err
	}
	if err := validateNewAPITokenKeys(items); err != nil {
		return nil, err
	}
	return normalizeTokenItems(items), nil
}

// hydrateListedTokenKeys replaces masked keys in the owned list using the
// ownership-checked batch endpoint. An unresolved key is not proof of absence.
func (n *NewApiAdapter) hydrateListedTokenKeys(
	ctx context.Context,
	baseURL string,
	items []map[string]interface{},
	headers map[string]string,
	proxy *ProxyConfig,
) error {
	maskedIDs := make([]int, 0)
	seen := make(map[int]struct{})
	for _, item := range items {
		key, _ := getString(item, "key")
		if !strings.Contains(key, "*") {
			continue
		}
		id, ok := getFloat(item, "id")
		if !ok || id <= 0 || id != float64(int(id)) {
			return fmt.Errorf("New API returned a masked token key without a usable token id")
		}
		if _, exists := seen[int(id)]; exists {
			return fmt.Errorf("New API returned a duplicate masked token id %d", int(id))
		}
		seen[int(id)] = struct{}{}
		maskedIDs = append(maskedIDs, int(id))
	}
	if len(maskedIDs) == 0 {
		return nil
	}

	for start := 0; start < len(maskedIDs); start += UpstreamTokenListPageLimit {
		end := start + UpstreamTokenListPageLimit
		if end > len(maskedIDs) {
			end = len(maskedIDs)
		}
		batchIDs := maskedIDs[start:end]
		resp, err := fetchJSON(
			ctx,
			strings.TrimRight(baseURL, "/")+"/api/token/batch/keys",
			"POST",
			map[string]interface{}{"ids": batchIDs},
			headers,
			proxy,
		)
		if err != nil {
			return fmt.Errorf("fetch full New API token keys: %w", err)
		}
		if success, present := getBool(resp, "success"); present && !success {
			return fmt.Errorf("fetch full New API token keys: upstream refused the lookup")
		}
		data, ok := getMap(resp, "data")
		if !ok {
			return fmt.Errorf("fetch full New API token keys: response has no data")
		}
		keys, ok := getMap(data, "keys")
		if !ok {
			return fmt.Errorf("fetch full New API token keys: response has no keys")
		}
		for _, id := range batchIDs {
			fullKey, _ := getString(keys, fmt.Sprintf("%d", id))
			fullKey = strings.TrimSpace(fullKey)
			if fullKey == "" || strings.Contains(fullKey, "*") {
				return fmt.Errorf("fetch full New API token keys: token %d is missing", id)
			}
			for _, item := range items {
				itemID, _ := getFloat(item, "id")
				if int(itemID) == id {
					item["key"] = fullKey
					break
				}
			}
		}
	}
	return nil
}

func (n *NewApiAdapter) listedTokenIDForDelete(ctx context.Context, baseURL string, headers map[string]string, targetKey string, proxy *ProxyConfig) (*int, bool, error) {
	items, started, complete, err := n.fetchNewAPITokenItems(ctx, baseURL, headers, proxy)
	if err != nil && !errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		return nil, started, err
	}
	if !started {
		return nil, false, fmt.Errorf("upstream token listing did not answer")
	}
	if err := n.hydrateListedTokenKeys(ctx, baseURL, items, headers, proxy); err != nil {
		return nil, true, err
	}
	if err := validateNewAPITokenKeys(items); err != nil {
		return nil, true, err
	}
	// New API stores bare keys but presents them with an optional sk- prefix.
	// Compare the credential identity, not that display prefix.
	targetKey = strings.TrimPrefix(targetKey, "sk-")
	for _, item := range items {
		key, ok := getString(item, "key")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, true, fmt.Errorf("upstream token listing contains an unresolved key")
		}
		if strings.TrimPrefix(normalizeTokenKeyForCompare(key), "sk-") == targetKey {
			id := getIntPtr(item, "id")
			if id == nil || *id <= 0 {
				return nil, true, fmt.Errorf("matching upstream token has no usable id")
			}
			return id, true, nil
		}
	}
	if !complete {
		return nil, true, ErrNewAPITokenListingCompletenessUnproven
	}
	return nil, true, nil
}

func (n *NewApiAdapter) CreateAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, options *CreateAPITokenOptions, proxy *ProxyConfig) (bool, error) {
	payload := buildDefaultTokenPayload(options)
	bodyBytes, _ := json.Marshal(payload)

	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	// `answered` separates "the upstream refused" from "no attempt reached the
	// upstream". Both end as a 502 in the caller, but only the second one is a
	// failure to ask, and reporting it as a refusal left the operator with no
	// reason and no WARN log.
	answered := false
	reason := ""

	// Try Bearer auth
	resp, err := fetchJSON(ctx, baseURL+"/api/token/", "POST", json.RawMessage(bodyBytes), n.authHeaders(accessToken, resolvedUserID), proxy)
	if err != nil {
		reason = err.Error()
	} else {
		if success, _ := getBool(resp, "success"); success {
			return true, nil
		}
		answered = true
		reason = newApiRefusalReason(resp)
	}

	// Cookie fallback
	cookieUserID := resolvedUserID
	if cookieUserID == nil {
		cookieUserID = n.probeUserIDByCookie(ctx, baseURL, accessToken, proxy)
	}
	for _, cookie := range buildCookieCandidates(accessToken) {
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(cookieUserID) {
			headers[k] = v
		}

		resp, err := fetchJSON(ctx, baseURL+"/api/token/", "POST", json.RawMessage(bodyBytes), headers, proxy)
		if err != nil {
			reason = err.Error()
			continue
		}
		if success, _ := getBool(resp, "success"); success {
			return true, nil
		}
		answered = true
		reason = newApiRefusalReason(resp)
	}

	if answered {
		return false, nil
	}
	return false, fmt.Errorf("create upstream token: %s", reason)
}

// newApiRefusalReason is the upstream's own verdict on a write it answered.
func newApiRefusalReason(resp map[string]interface{}) string {
	msg, _ := getString(resp, "message")
	if strings.TrimSpace(msg) == "" {
		return "upstream reported failure"
	}
	return msg
}

func (n *NewApiAdapter) DeleteAPIToken(ctx context.Context, baseURL, accessToken, tokenKey string, platformUserId *int, proxy *ProxyConfig) error {
	targetKey := normalizeTokenKeyForCompare(tokenKey)
	if targetKey == "" {
		return nil
	}

	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	// A listing authorizes DELETE only when it identifies the target by the
	// same owner's key/ID. A missing target is a no-op only when the listing is
	// complete; an unproven legacy listing must fail closed. Never replay
	// DELETE through a cookie or alternate user after a failed write.
	listed := false
	reason := ""
	headers := n.authHeaders(accessToken, resolvedUserID)
	tokenID, started, resolveErr := n.listedTokenIDForDelete(ctx, baseURL, headers, targetKey, proxy)
	if resolveErr != nil {
		reason = resolveErr.Error()
		if started {
			return fmt.Errorf("list upstream tokens: %w", resolveErr)
		}
	} else {
		listed = true
		if tokenID == nil {
			return nil
		}
		delResp, err := fetchJSON(ctx, fmt.Sprintf("%s/api/token/%d", baseURL, *tokenID), http.MethodDelete, nil, headers, proxy)
		if err != nil {
			return fmt.Errorf("delete upstream token %d: %s", *tokenID, err)
		}
		if success, _ := getBool(delResp, "success"); success {
			return nil
		}
		return fmt.Errorf("delete upstream token %d: %s", *tokenID, newApiRefusalReason(delResp))
	}

	// The bearer shape never answered. Retry listing with cookie transport for
	// the same stored credential; no user-identity probe is performed.
	for _, cookie := range buildCookieCandidates(accessToken) {
		cookieHeaders := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(resolvedUserID) {
			cookieHeaders[k] = v
		}

		tokenID, started, resolveErr = n.listedTokenIDForDelete(ctx, baseURL, cookieHeaders, targetKey, proxy)
		if resolveErr != nil {
			reason = resolveErr.Error()
			if started {
				return fmt.Errorf("list upstream tokens: %w", resolveErr)
			}
			continue
		}
		listed = true
		if tokenID == nil {
			return nil
		}

		delResp, err := fetchJSON(ctx, fmt.Sprintf("%s/api/token/%d", baseURL, *tokenID), http.MethodDelete, nil, cookieHeaders, proxy)
		if err != nil {
			return fmt.Errorf("delete upstream token %d: %s", *tokenID, err)
		}
		if success, _ := getBool(delResp, "success"); success {
			return nil
		}
		return fmt.Errorf("delete upstream token %d: %s", *tokenID, newApiRefusalReason(delResp))
	}

	if listed {
		return nil
	}
	if reason == "" {
		reason = "no listing answered"
	}
	return fmt.Errorf("list upstream tokens: %s", reason)
}
