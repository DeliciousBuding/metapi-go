package platform

import (
	"context"
	"net/http"
)

type requestRedirectPolicyKey struct{}

// WithoutRequestRedirects narrows one operation's destination permission.
// It reuses the shared clients; it does not mutate their global redirect policy.
func WithoutRequestRedirects(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestRedirectPolicyKey{}, true)
}

func requestRedirectsDisabled(req *http.Request, via []*http.Request) bool {
	if blocked, _ := req.Context().Value(requestRedirectPolicyKey{}).(bool); blocked {
		return true
	}
	if len(via) > 0 {
		blocked, _ := via[0].Context().Value(requestRedirectPolicyKey{}).(bool)
		return blocked
	}
	return false
}
