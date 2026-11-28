package config

import (
	"net/url"
	"strings"
)

// validateUrl parses a URL string and returns a warning configError when the
// value is malformed. For webhook-style URLs (requireWebScheme=true) it also
// warns when the scheme is not http/https; otherwise it warns when the URL is
// missing a scheme or host. Empty values are treated as unset (no error).
func validateUrl(field, value string, requireWebScheme bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return &configError{
			field:    field,
			value:    value,
			msg:      "malformed URL — " + err.Error(),
			critical: false,
		}
	}
	if requireWebScheme {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return &configError{
				field:    field,
				value:    value,
				msg:      "scheme must be http or https",
				critical: false,
			}
		}
	} else if parsed.Scheme == "" {
		return &configError{
			field:    field,
			value:    value,
			msg:      "URL is missing a scheme",
			critical: false,
		}
	}
	if parsed.Host == "" {
		return &configError{
			field:    field,
			value:    value,
			msg:      "URL is missing a host",
			critical: false,
		}
	}
	return nil
}

func validateAdminCorsOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" || strings.Contains(origin, "*") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Path == "" || parsed.Path == "/"
}
