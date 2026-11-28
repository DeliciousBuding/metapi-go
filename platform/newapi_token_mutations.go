package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

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
		headers := n.cookieHeaders(cookie, cookieUserID)

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
	reason := ""
	headers := n.authHeaders(accessToken, resolvedUserID)
	tokenID, started, resolveErr := n.listedTokenIDForDelete(ctx, baseURL, headers, targetKey, proxy)
	if resolveErr != nil {
		reason = resolveErr.Error()
		if started {
			return fmt.Errorf("list upstream tokens: %w", resolveErr)
		}
	} else {
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
		cookieHeaders := n.cookieHeaders(cookie, resolvedUserID)

		tokenID, started, resolveErr = n.listedTokenIDForDelete(ctx, baseURL, cookieHeaders, targetKey, proxy)
		if resolveErr != nil {
			reason = resolveErr.Error()
			if started {
				return fmt.Errorf("list upstream tokens: %w", resolveErr)
			}
			continue
		}
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

	if reason == "" {
		reason = "no listing answered"
	}
	return fmt.Errorf("list upstream tokens: %s", reason)
}
