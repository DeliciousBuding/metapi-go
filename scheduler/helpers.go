package scheduler

import (
	"strings"
)

// stringsTrimLower trims whitespace and lowercases. Returns "active" for empty.
func stringsTrimLower(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return "active"
	}
	return strings.ToLower(t)
}
