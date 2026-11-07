package contract

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const imageTaskPlaceholder = "{taskId}"

func ValidImageTaskProtocol(protocol string) bool {
	switch protocol {
	case "imageGeneration", "imageEdit", "imageVariation", "modelscopeImageGeneration":
		return true
	default:
		return false
	}
}

// The task id replaces exactly one full path segment. This is an operation
// template, not a wildcard URL grant or a plugin-controlled destination.
func ValidImageQueryPath(path string) bool {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || len(path) > 8192 || strings.ContainsAny(path, "?#\\") || strings.ContainsFunc(path, unicode.IsControl) || strings.Count(path, imageTaskPlaceholder) != 1 {
		return false
	}
	decoded, err := url.PathUnescape(path)
	if err != nil || strings.ContainsAny(decoded, "?#\\") || strings.ContainsFunc(decoded, unicode.IsControl) {
		return false
	}
	placeholder := false
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return false
		}
		if strings.ContainsAny(segment, "{}") {
			if segment != imageTaskPlaceholder || placeholder {
				return false
			}
			placeholder = true
		}
	}
	return placeholder
}

// Preserve escaped vendor task ids such as "task/id ?#%" as data. The caller
// separately proves the saved template belongs to its selected endpoint.
func ImageQueryURL(template, taskID string) (string, error) {
	u, err := url.Parse(template)
	if err != nil || len(template) > 9216 || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "http" && u.Scheme != "https" || !ValidImageQueryPath(u.Path) || taskID == "" || taskID == "." || taskID == ".." || len(taskID) > 512 || !utf8.ValidString(taskID) || strings.ContainsFunc(taskID, unicode.IsControl) {
		return "", errors.New("invalid_image_query_template")
	}
	escaped := strings.Replace(u.EscapedPath(), url.PathEscape(imageTaskPlaceholder), url.PathEscape(taskID), 1)
	u.Path = strings.Replace(u.Path, imageTaskPlaceholder, taskID, 1)
	u.RawPath = escaped
	return u.String(), nil
}
