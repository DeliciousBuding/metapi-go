package proxyhandler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"unicode"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
)

type traceIdentityContextKey struct{}

func requestTraceIdentity(ctx context.Context, pac *auth.ProxyAuthContext, headers http.Header, requestedModel string) string {
	if identity, _ := ctx.Value(traceIdentityContextKey{}).(string); identity != "" {
		return identity
	}
	namespace := downstreamSessionNamespace(pac)
	if namespace == "" {
		return ""
	}
	// Only explicit session metadata. Ordinary prompt/metadata text and
	// request IDs never become cross-request affinity identities.
	for _, header := range []string{"AH-Trace-Id", "Session-Id", "Session_id", "Conversation-Id", "Conversation_id", "X-Opencode-Session", "X-Session-Id", "X-Session-Affinity"} {
		value := strings.TrimSpace(headers.Get(header))
		if value == "" {
			continue
		}
		if len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return ""
		}
		sum := sha256.Sum256([]byte(namespace + "\x00" + header + "\x00" + value + "\x00" + requestedModel))
		return hex.EncodeToString(sum[:])
	}
	return ""
}

type successfulSelectionRecorder interface {
	RecordSuccessfulSelection(*routing.SelectedChannel)
}

func rememberSuccessfulRouting(cfg *UpstreamConfig, selected *routing.SelectedChannel) {
	if recorder, ok := cfg.Router.(successfulSelectionRecorder); ok {
		recorder.RecordSuccessfulSelection(selected)
	}
}

func forgetFailedRouting(cfg *UpstreamConfig, selected *routing.SelectedChannel) {
	if recorder, ok := cfg.Router.(interface {
		RecordFailedSelection(*routing.SelectedChannel)
	}); ok {
		recorder.RecordFailedSelection(selected)
	}
}
