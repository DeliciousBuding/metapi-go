package scheduler

import (
	"fmt"
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

// formatErr is a shorthand for fmt.Errorf.
func formatErr(f string, args ...any) error {
	return fmt.Errorf(f, args...)
}
