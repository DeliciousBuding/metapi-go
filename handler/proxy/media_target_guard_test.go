package proxyhandler

import (
	"context"
	"testing"
)

func TestAllowedPublicMediaTargetRejectsPrivateTargets(t *testing.T) {
	previous := mediaTargetAllowPrivate
	mediaTargetAllowPrivate = false
	t.Cleanup(func() { mediaTargetAllowPrivate = previous })

	ctx := context.Background()
	for _, raw := range []string{
		"http://127.0.0.1:18093/marker",
		"http://localhost/marker",
		"http://[::1]/marker",
		"http://10.0.0.1/marker",
		"http://192.168.1.1/marker",
		"http://169.254.169.254/latest/meta-data",
		"http://0.0.0.0/marker",
		"http://0.1.2.3/marker",
		"http://[fd20:ce::254]/marker",
		"http://user:pass@example.com/marker",
		"file:///etc/passwd",
		"not a url",
	} {
		if allowedPublicMediaTarget(ctx, raw) {
			t.Errorf("%q must not be fetched by the gateway", raw)
		}
	}

	if !allowedPublicMediaTarget(ctx, "https://93.184.216.34/image.png") {
		t.Error("public literal address must be allowed")
	}
}

func TestAllowedPublicMediaTargetTestEscapeHatch(t *testing.T) {
	previous := mediaTargetAllowPrivate
	mediaTargetAllowPrivate = true
	t.Cleanup(func() { mediaTargetAllowPrivate = previous })

	if !allowedPublicMediaTarget(context.Background(), "http://127.0.0.1:18093/marker") {
		t.Error("test escape hatch must allow the loopback fixture server")
	}
}
