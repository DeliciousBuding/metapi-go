package oauth

import (
	"fmt"
	"strings"
	"time"
)

// 三个 provider 的相对过期时间语义相同；Google 还提供绝对 expiry 作为 fallback。
func parseTokenExpiresAt(value any, expiry string) int64 {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return time.Now().UnixMilli() + int64(v)*1000
		}
	case string:
		parsed, err := parseDecimalInt64(strings.TrimSpace(v))
		if err == nil && parsed > 0 {
			return time.Now().UnixMilli() + parsed*1000
		}
	}
	if expiry != "" {
		if parsed, err := time.Parse(time.RFC3339, expiry); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}

// 保持已有空串、符号和溢出行为；直接换 strconv.ParseInt 会改变这些契约。
func parseDecimalInt64(s string) (int64, error) {
	n := int64(0)
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}
