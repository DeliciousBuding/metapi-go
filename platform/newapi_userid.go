package platform

import (
	"context"
	"fmt"
)

func (n *NewApiAdapter) userIDHeaders(userID *int) map[string]string {
	headers := make(map[string]string)
	if userID != nil {
		val := fmt.Sprintf("%d", *userID)
		headers["New-API-User"] = val
		headers["Veloera-User"] = val
		headers["voapi-user"] = val
		headers["User-id"] = val
		headers["X-User-Id"] = val
		headers["Rix-Api-User"] = val
		headers["neo-api-user"] = val
	}
	return headers
}

func (n *NewApiAdapter) authHeaders(accessToken string, userID *int) map[string]string {
	headers := n.userIDHeaders(userID)
	headers["Authorization"] = "Bearer " + accessToken
	return headers
}

func (n *NewApiAdapter) cookieHeaders(cookie string, userID *int) map[string]string {
	headers := n.userIDHeaders(userID)
	headers["Cookie"] = cookie
	return headers
}

func (n *NewApiAdapter) fetchUserSelfByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) (map[string]interface{}, error) {
	for _, cookie := range buildCookieCandidates(token) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		headers := n.cookieHeaders(cookie, userID)

		resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, headers, proxy)
		if err != nil {
			continue
		}
		if success, _ := getBool(resp, "success"); success {
			if _, ok := getMap(resp, "data"); ok {
				return resp, nil
			}
		}
	}
	return nil, fmt.Errorf("cookie fetch failed")
}

func (n *NewApiAdapter) probeUserIDByCookie(ctx context.Context, baseURL, token string, proxy *ProxyConfig) *int {
	candidates := n.buildUserIDProbeCandidates(token)
	for _, cookie := range buildCookieCandidates(token) {
		for _, id := range candidates {
			if err := ctx.Err(); err != nil {
				return nil
			}
			idCopy := id
			headers := n.cookieHeaders(cookie, &idCopy)

			resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, headers, proxy)
			if err != nil {
				continue
			}
			if success, _ := getBool(resp, "success"); success {
				if _, ok := getMap(resp, "data"); ok {
					result := id
					return &result
				}
			}
		}
	}
	return nil
}

func (n *NewApiAdapter) probeAlternateUserIDByCookie(ctx context.Context, baseURL, token string, currentUserID *int, proxy *ProxyConfig) *int {
	probed := n.probeUserIDByCookie(ctx, baseURL, token, proxy)
	if probed == nil {
		return nil
	}
	if currentUserID != nil && *currentUserID > 0 && *probed == *currentUserID {
		return nil
	}
	return probed
}

// discoverUserID tries JWT, Bearer direct, cookie direct, then cookie probe.
func (n *NewApiAdapter) discoverUserID(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) *int {
	if err := ctx.Err(); err != nil {
		return nil
	}
	// 1. JWT decode
	if jwtID := n.tryDecodeUserID(accessToken); jwtID != nil {
		idCopy := *jwtID
		resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, n.authHeaders(accessToken, &idCopy), proxy)
		if err == nil {
			if success, _ := getBool(resp, "success"); success {
				if _, ok := getMap(resp, "data"); ok {
					return &idCopy
				}
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return nil
	}
	// 2. Bearer direct (no userID)
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, authBearerHeaders(accessToken), proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			if data, ok := getMap(resp, "data"); ok {
				if id := getIntPtr(data, "id"); id != nil {
					return id
				}
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return nil
	}
	// 3. Cookie direct
	cookieResp, err := n.fetchUserSelfByCookie(ctx, baseURL, accessToken, nil, proxy)
	if err == nil && cookieResp != nil {
		if data, ok := getMap(cookieResp, "data"); ok {
			if id := getIntPtr(data, "id"); id != nil {
				return id
			}
		}
	}

	// 4. Cookie probe
	return n.probeUserIDByCookie(ctx, baseURL, accessToken, proxy)
}

func (n *NewApiAdapter) probeUserID(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) *int {
	if err := ctx.Err(); err != nil {
		return nil
	}
	if jwtID := n.tryDecodeUserID(accessToken); jwtID != nil {
		idCopy := *jwtID
		if n.testUserID(ctx, baseURL, accessToken, idCopy, proxy) {
			return &idCopy
		}
	}

	for _, id := range n.buildUserIDProbeCandidates(accessToken) {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if n.testUserID(ctx, baseURL, accessToken, id, proxy) {
			result := id
			return &result
		}
	}
	return nil
}

func (n *NewApiAdapter) testUserID(ctx context.Context, baseURL, accessToken string, userID int, proxy *ProxyConfig) bool {
	idCopy := userID
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, n.authHeaders(accessToken, &idCopy), proxy)
	if err != nil {
		return false
	}
	success, _ := getBool(resp, "success")
	return success
}

func (n *NewApiAdapter) userSelfDataByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) (map[string]interface{}, *int) {
	resp, err := n.fetchUserSelfByCookie(ctx, baseURL, token, userID, proxy)
	if err == nil {
		if data, ok := getMap(resp, "data"); ok {
			return data, getIntPtr(data, "id")
		}
	}
	altID := n.probeAlternateUserIDByCookie(ctx, baseURL, token, userID, proxy)
	if altID != nil {
		resp, err = n.fetchUserSelfByCookie(ctx, baseURL, token, altID, proxy)
		if err == nil {
			if data, ok := getMap(resp, "data"); ok {
				return data, altID
			}
		}
	}
	return nil, nil
}
