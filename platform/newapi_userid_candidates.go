package platform

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

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
