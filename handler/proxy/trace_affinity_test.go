package proxyhandler

import (
	"context"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
)

func TestTraceAffinityIdentityIsExplicitAndKeyScoped(t *testing.T) {
	a, b := int64(1), int64(2)
	first, second := &auth.ProxyAuthContext{KeyID: &a}, &auth.ProxyAuthContext{KeyID: &b}
	headers := http.Header{"Ah-Trace-Id": []string{"synthetic-session"}}
	one := requestTraceIdentity(context.Background(), first, headers, "model")
	if one == "" || one == requestTraceIdentity(context.Background(), second, headers, "model") || one == requestTraceIdentity(context.Background(), first, headers, "other") {
		t.Fatal("trace namespace crossed Key/model boundary")
	}
	if requestTraceIdentity(context.Background(), first, http.Header{"X-Request-Id": []string{"synthetic-session"}}, "model") != "" {
		t.Fatal("ordinary request ID became session affinity")
	}
	for _, header := range []string{"Session-Id", "Session_id", "X-Session-Affinity", "X-Opencode-Session"} {
		if requestTraceIdentity(context.Background(), first, http.Header{header: []string{"synthetic-session"}}, "model") == "" {
			t.Fatalf("explicit session header not accepted: %s", header)
		}
	}
}
