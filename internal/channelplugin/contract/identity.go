// Package contract contains plugin configuration identity rules without
// depending on the executable JS runtime. Storage validates data, not code.
package contract

import "regexp"

const MaxProtocols = 6

// Protocols name endpoint slots, not interchangeable wire formats. In
// particular an HTTP Responses binding grants neither compact nor WS hooks.
func ValidProtocol(protocol string) bool {
	switch protocol {
	case "chat", "responses", "messages", "gemini", "responsesCompact", "responsesWebsocket":
		return true
	default:
		return false
	}
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func ValidKey(key string) bool { return keyPattern.MatchString(key) }
