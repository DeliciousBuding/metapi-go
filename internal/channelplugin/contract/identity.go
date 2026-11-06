// Package contract contains plugin configuration identity rules without
// depending on the executable JS runtime. Storage validates data, not code.
package contract

import "regexp"

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func ValidKey(key string) bool { return keyPattern.MatchString(key) }
