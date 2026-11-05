package auth

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/config"
)

// RevalidateQueuedRequest reloads current identity and authorization for work
// already admitted at ingress. Admission counters and budget holds belong to
// that original request and must not be consumed again after FIFO waiting.
func RevalidateQueuedRequest(r *http.Request, previous *ProxyAuthContext) DownstreamTokenAuthResult {
	if previous == nil || previous.KeyID == nil {
		return DownstreamTokenAuthResult{StatusCode: 401, Reason: "invalid", Error: "Invalid queued request identity"}
	}
	result := authorizeDownstreamToken(previous.Token, config.RuntimeSafe(), "", admissionOptions{authorizationOnly: true})
	if !result.OK || result.Key == nil {
		return result
	}
	sameID := func(a, b *int64) bool {
		return a == nil && b == nil || a != nil && b != nil && *a == *b
	}
	if result.Key.ID != *previous.KeyID || !sameID(result.Key.OwnerUserID, previous.OwnerUserID) || !sameID(result.Key.UserGroupID, previous.UserGroupID) {
		return DownstreamTokenAuthResult{StatusCode: 403, Reason: "identity_changed", Error: "Queued request identity changed"}
	}
	if ok, reason := CheckDownstreamKeyIP(result.Key, extractClientIP(r)); !ok {
		return DownstreamTokenAuthResult{StatusCode: 403, Reason: reason, Error: "IP address is not allowed for this API key"}
	}
	return result
}
