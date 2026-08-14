package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// --- User-ID header helpers ---

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
	headers := map[string]string{"Authorization": "Bearer " + accessToken}
	for k, v := range n.userIDHeaders(userID) {
		headers[k] = v
	}
	return headers
}

// --- User-ID discovery ---

// Package-level patterns: Go RE2 does not support PCRE lookaheads like (?!\d).
// Match digit runs of length ≥4, then reject ids longer than 8 in Go (parity with former {4,8}).
var (
	underscoreUserIDRE = regexp.MustCompile(`_(\d{4,})`)
	namedUserIDRE      = regexp.MustCompile(`(?i)(?:user(?:name)?|uid|id)[^\d]{0,16}(\d{4,})`)
)

func (n *NewApiAdapter) tryDecodeUserID(token string) *int {
	t := strings.TrimSpace(token)
	t = strings.TrimPrefix(t, "Bearer ")
	t = strings.TrimSpace(t)

	parts := strings.Split(t, ".")
	if len(parts) != 3 {
		return nil
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}

	if id, ok := claims["id"].(float64); ok {
		result := int(id)
		return &result
	}
	if sub, ok := claims["sub"]; ok {
		switch v := sub.(type) {
		case float64:
			result := int(v)
			return &result
		case string:
			if id, err := strconv.Atoi(v); err == nil {
				return &id
			}
		}
	}
	return nil
}

func (n *NewApiAdapter) buildUserIDProbeCandidates(token string) []int {
	var candidates []int
	seen := make(map[int]bool)

	push := func(id int) {
		if id <= 0 || seen[id] {
			return
		}
		seen[id] = true
		candidates = append(candidates, id)
	}

	if id := n.tryDecodeUserID(token); id != nil {
		push(*id)
	}

	for _, id := range n.extractLikelyUserIDs(token) {
		push(id)
	}

	// Hardcoded probe list (common NewApi deployment defaults)
	for _, id := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 50, 100, 8899, 11494} {
		push(id)
	}

	return candidates
}

func (n *NewApiAdapter) extractLikelyUserIDs(token string) []int {
	var ids []int
	seen := make(map[int]bool)
	push := func(id int) {
		if id <= 0 || id > 10_000_000 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}

	t := strings.TrimSpace(token)

	// Extract session values from cookie candidates
	sessionValues := make(map[string]bool)
	for _, c := range buildCookieCandidates(t) {
		re := regexp.MustCompile(`(?:^|;\s*)session=([^;]+)`)
		if match := re.FindStringSubmatch(c); len(match) > 1 {
			sessionValues[strings.TrimSpace(match[1])] = true
		}
	}
	if t != "" && !strings.Contains(t, "=") {
		sessionValues[stripBearerPrefix(t)] = true
	}

	for sv := range sessionValues {
		// Try base64 decode
		decoded, err := base64.RawStdEncoding.DecodeString(sv)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(sv)
		}
		if err != nil {
			continue
		}

		payloads := []string{string(decoded)}
		parts := strings.Split(string(decoded), "|")
		if len(parts) >= 2 {
			if midDecoded, err := base64.RawStdEncoding.DecodeString(parts[1]); err == nil {
				payloads = append(payloads, string(midDecoded))
			} else if midDecoded, err := base64.StdEncoding.DecodeString(parts[1]); err == nil {
				payloads = append(payloads, string(midDecoded))
			}
		}

		for _, payload := range payloads {
			// Pattern: _12345678 (RE2-safe; length capped in Go)
			for _, match := range underscoreUserIDRE.FindAllStringSubmatch(payload, -1) {
				if len(match[1]) > 8 {
					continue
				}
				if id, err := strconv.Atoi(match[1]); err == nil {
					push(id)
				}
			}
			// Pattern: user/id/uid near a number (RE2-safe; length capped in Go)
			for _, match := range namedUserIDRE.FindAllStringSubmatch(payload, -1) {
				if len(match[1]) > 8 {
					continue
				}
				if id, err := strconv.Atoi(match[1]); err == nil {
					push(id)
				}
			}
		}

		// Gob binary extraction for 'id' field
		for _, id := range extractGobFieldInts(decoded, "id") {
			push(id)
		}
	}

	return ids
}

// --- Gob decoding ---

func extractGobFieldInts(payload []byte, fieldName string) []int {
	var ids []int
	seen := make(map[int]bool)
	push := func(id int) {
		if id <= 0 || id > 10_000_000 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}

	// Build marker: fieldName + 0x03 + "int" + 0x04
	marker := append([]byte(fieldName), 0x03)
	marker = append(marker, []byte("int")...)
	marker = append(marker, 0x04)

	start := 0
	for start < len(payload) {
		pos := indexOf(payload, marker, start)
		if pos < 0 {
			break
		}

		markerEnd := pos + len(marker)
		if markerEnd+1 >= len(payload) {
			start = pos + len(marker)
			continue
		}

		encodedLength := payload[markerEnd]
		delimiter := payload[markerEnd+1]
		if delimiter != 0x00 {
			start = pos + len(marker)
			continue
		}

		byteLength := int(encodedLength) - 1
		if byteLength <= 0 || markerEnd+2+byteLength > len(payload) {
			start = pos + len(marker)
			continue
		}

		valueBytes := payload[markerEnd+2 : markerEnd+2+byteLength]
		if id := decodeGobSignedInt(valueBytes); id > 0 {
			push(id)
		}

		start = pos + len(marker)
	}

	return ids
}

func indexOf(data, sub []byte, start int) int {
	for i := start; i <= len(data)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if data[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func decodeGobSignedInt(encoded []byte) int {
	if len(encoded) == 0 {
		return 0
	}

	var unsigned uint64
	if encoded[0] < 0x80 {
		unsigned = uint64(encoded[0])
	} else {
		width := 0x100 - int(encoded[0])
		if width <= 0 || len(encoded) != width+1 {
			return 0
		}
		for i := 1; i < len(encoded); i++ {
			unsigned = (unsigned << 8) | uint64(encoded[i])
		}
	}

	// zigzag decode
	var signed int64
	if unsigned&1 == 0 {
		signed = int64(unsigned >> 1)
	} else {
		signed = -int64((unsigned >> 1) + 1)
	}

	if signed <= 0 || signed > 10_000_000 {
		return 0
	}
	return int(signed)
}

// --- Cookie-based fetch helpers ---

func (n *NewApiAdapter) fetchUserSelfByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) (map[string]interface{}, error) {
	for _, cookie := range buildCookieCandidates(token) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(userID) {
			headers[k] = v
		}

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
			headers := map[string]string{"Cookie": cookie}
			for k, v := range n.userIDHeaders(&idCopy) {
				headers[k] = v
			}

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
