package backup

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// IsLocalUserResource identifies ownership that a portable backup cannot
// restore. The user namespace is retained in both ordinary owner columns and
// opaque direct-video identity JSON, so losing an optional owner column cannot
// turn the task into a system-Key resource. Conservatively reject any identity
// containing this reserved prefix, including malformed/truncated JSON.
func IsLocalUserResource(table string, row map[string]any) bool {
	if table != "proxy_video_tasks" && table != "proxy_file_objects" {
		return false
	}
	for _, column := range []string{"owner", "direct_identity"} {
		var raw string
		switch value := row[column].(type) {
		case string:
			raw = value
		case []byte:
			raw = string(value)
		}
		if strings.Contains(raw, store.UserKeyResourceOwnerPrefix) {
			return true
		}
	}
	return false
}
