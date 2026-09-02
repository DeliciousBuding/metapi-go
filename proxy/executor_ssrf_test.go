package proxy

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// The executor dials operator-configured site URLs, so its transports carry the
// shared site dial guard. A hostname/IP that resolves into the cloud metadata
// range must be refused before any connection attempt, even though loopback and
// RFC1918 upstreams stay reachable for self-hosted deployments.
func TestRuntimeExecutor_RefusesMetadataTarget(t *testing.T) {
	executor := NewRuntimeExecutor(2 * time.Second)
	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := executor.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected the dial guard to refuse the metadata address")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("expected a forbidden-target error, got %v", err)
	}
}

func TestNewStreamTransport_RefusesMetadataTarget(t *testing.T) {
	client := &http.Client{Transport: NewStreamTransport(), Timeout: 2 * time.Second}
	resp, err := client.Get("http://metadata.google.internal/computeMetadata/v1/")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected the dial guard to refuse the metadata hostname")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("expected a forbidden-target error, got %v", err)
	}
}
