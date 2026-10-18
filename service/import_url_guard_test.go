package service

import "testing"

func TestSanitizeImportedSiteRows_DropsForbiddenTargets(t *testing.T) {
	rows := []map[string]any{
		{"id": int64(1), "url": "https://a.example.com", "name": "clean"},
		{"id": int64(2), "url": "http://169.254.169.254/latest/meta-data/", "name": "metadata"},
		{"id": int64(3), "url": "https://b.example.com", "proxy_url": "http://[fd00::1]:8080", "name": "ula-proxy-allowed"},
		{"id": int64(4), "url": "https://c.example.com", "external_checkin_url": "http://100.100.100.200/latest/meta-data", "name": "metadata-checkin"},
		{"id": int64(5), "url": "http://127.0.0.1:11434/v1", "name": "localhost-allowed"},
		{"id": int64(6), "url": nil, "name": "null-url-allowed"},
	}
	kept, dropped := SanitizeImportedSiteRows("sites", rows)
	if dropped != 2 {
		t.Fatalf("dropped = %d, want 2 (kept=%v)", dropped, kept)
	}
	if len(kept) != 4 {
		t.Fatalf("kept = %d rows, want 4", len(kept))
	}
	// ULA proxies and localhost stay allowed (operator-hosted upstreams);
	// metadata targets (169.254.169.254, 100.100.100.200) are dropped.
	wantKept := map[int64]bool{1: true, 3: true, 5: true, 6: true}
	for _, row := range kept {
		id, _ := row["id"].(int64)
		if !wantKept[id] {
			t.Fatalf("unexpected kept row id=%d: %v", id, row)
		}
	}
}

func TestSanitizeImportedSiteRows_EndpointURLs(t *testing.T) {
	rows := []map[string]any{
		{"id": int64(1), "site_id": int64(1), "url": "https://ep.example.com"},
		{"id": int64(2), "site_id": int64(1), "url": "http://169.254.169.254/"},
	}
	kept, dropped := SanitizeImportedSiteRows("site_api_endpoints", rows)
	if dropped != 1 || len(kept) != 1 {
		t.Fatalf("kept=%d dropped=%d, want 1/1", len(kept), dropped)
	}
}

func TestSanitizeImportedSiteRows_UnguardedTablesUntouched(t *testing.T) {
	rows := []map[string]any{{"key": "anything", "value": "http://169.254.169.254/"}}
	kept, dropped := SanitizeImportedSiteRows("settings", rows)
	if dropped != 0 || len(kept) != 1 {
		t.Fatalf("unguarded table must pass through: kept=%d dropped=%d", len(kept), dropped)
	}
}

// TestSanitizeImportedSiteRows_UpstreamChannelsBaseURL covers the M9 gap: a
// crafted backup could plant a GCP IPv6 metadata base URL in
// upstream_channels, then the proxy data plane would dial it with the caller's
// custom headers.
func TestSanitizeImportedSiteRows_UpstreamChannelsBaseURL(t *testing.T) {
	rows := []map[string]any{
		{"id": int64(1), "name": "public", "base_url": "https://provider.example/v1"},
		{"id": int64(2), "name": "gcp-metadata", "base_url": "http://[fd20:ce::254]"},
		{"id": int64(3), "name": "link-local", "base_url": "http://169.254.169.254"},
		{"id": int64(4), "name": "private-allowed", "base_url": "http://10.0.0.5:8080"},
		{"id": int64(5), "name": "null-base", "base_url": nil},
	}
	kept, dropped := SanitizeImportedSiteRows("upstream_channels", rows)
	if dropped != 2 {
		t.Fatalf("dropped = %d, want 2 (kept=%v)", dropped, kept)
	}
	if len(kept) != 3 {
		t.Fatalf("kept = %d rows, want 3", len(kept))
	}
	for _, row := range kept {
		name := row["name"]
		if name == "gcp-metadata" || name == "link-local" {
			t.Fatalf("forbidden upstream channel row survived: %v", row)
		}
	}
}
