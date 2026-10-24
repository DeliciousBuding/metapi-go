package backup

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAxonHubRateLimitContract(t *testing.T) {
	for _, tc := range []struct {
		name, rateLimit string
		blocked         bool
	}{
		{"null", `null`, false},
		{"empty", `{}`, false},
		{"null limits", `{"rpm":null,"tpm":null,"maxConcurrent":null}`, false},
		{"zero limits", `{"rpm":0,"tpm":0,"maxConcurrent":0}`, false},
		{"signed 64-bit inactive limits", `{"rpm":-9223372036854775808,"tpm":null,"maxConcurrent":0}`, false},
		{"negative limits", `{"rpm":-1,"tpm":-2,"maxConcurrent":-3}`, false},
		{"queue without concurrency", `{"queueSize":10,"queueTimeoutMs":1000}`, false},
		{"queue with disabled concurrency", `{"maxConcurrent":0,"queueSize":10,"queueTimeoutMs":1000}`, false},
		{"positive rpm", `{"rpm":1}`, true},
		{"positive tpm", `{"tpm":1}`, true},
		{"positive concurrency", `{"maxConcurrent":1}`, true},
		{"positive concurrency with zero queue", `{"maxConcurrent":1,"queueSize":0}`, true},
		{"positive rpm overrides disabled concurrency", `{"rpm":1,"maxConcurrent":0,"queueSize":0}`, true},
		{"unknown zero field", `{"futureLimit":0}`, true},
		{"unknown null field", `{"futureLimit":null}`, true},
		{"unknown private field", `{"futureLimit":{"token":"fixture-private-marker"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := axonHubSettingsPayload(`{"rateLimit":` + tc.rateLimit + `}`)
			preview, err := PreviewAxonHubV14(nil, raw, "rate-limit-fixture")
			if err != nil {
				t.Fatal(err)
			}
			if tc.blocked {
				if len(preview.SkippedChannels) != 1 || !residualContains(preview.SkippedChannels[0].Reasons, "channel_rate_limit_unsupported") || preview.Routable["routes"] != 0 {
					t.Fatalf("active or unknown rate limit imported: %+v", preview)
				}
			} else if preview.Routable["channels"] != 1 || preview.Routable["routes"] != 1 || len(preview.SkippedChannels) != 0 {
				t.Fatalf("inactive rate limit dropped channel: %+v", preview)
			}
			encoded, err := json.Marshal(preview)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "fixture-private-marker") || strings.Contains(string(encoded), "fixture-key") {
				t.Fatal("private rate-limit payload or credential leaked in preview")
			}
		})
	}
}

func TestAxonHubRateLimitRejectsMalformedInput(t *testing.T) {
	for _, raw := range []string{`[]`, `true`, `"unlimited"`, `{"rpm":"0"}`, `{"tpm":1.5}`, `{"maxConcurrent":9223372036854775808}`, `{"queueSize":false}`, `{"queueTimeoutMs":{}}`, `{"rpm":0,"rpm":1}`} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseAxonHubSource(axonHubSettingsPayload(`{"rateLimit":` + raw + `}`))
			if err == nil {
				t.Fatal("malformed rate limit accepted")
			}
			if strings.Contains(err.Error(), raw) {
				t.Fatal("rate-limit error echoed private source input")
			}
		})
	}
}

func TestAxonHubRateLimitImportPreservesGraph(t *testing.T) {
	db := openAxonHubTestDB(t)
	inactive := axonHubSettingsPayload(`{"rateLimit":{"rpm":0,"maxConcurrent":null,"queueSize":10}}`)
	for i := 0; i < 2; i++ {
		counts, err := ImportAxonHubV14(db, inactive, "rate-limit-fixture", false)
		if err != nil {
			t.Fatal(err)
		}
		if counts["channels"] != 1 || counts["routes"] != 1 || counts["grants"] != 1 {
			t.Fatalf("inactive rate limit lost graph: %v", counts)
		}
	}
	for _, table := range []string{"upstream_channels", "upstream_credentials", "upstream_models", "upstream_grants", "token_routes"} {
		if countRows(t, db, "SELECT COUNT(*) FROM "+table) != 1 {
			t.Fatalf("reimport changed %s cardinality", table)
		}
	}
	// A failed second origin must neither claim nor remove the first origin's
	// route, even though both snapshots name the same source model.
	if _, err := ImportAxonHubV14(db, inactive, "another-origin", false); err == nil {
		t.Fatal("another origin claimed an existing route")
	}
	for _, settings := range []string{`{"rateLimit":{"rpm":1}}`, `{"rateLimit":{"futureLimit":0}}`, `{"rateLimit":{"rpm":"private-invalid-value"}}`} {
		if _, err := ImportAxonHubV14(db, axonHubSettingsPayload(settings), "rate-limit-fixture", false); err == nil {
			t.Fatal("unsupported rate limit replaced existing graph")
		}
		if countRows(t, db, "SELECT COUNT(*) FROM token_routes") != 1 || countRows(t, db, "SELECT COUNT(*) FROM upstream_grants") != 1 {
			t.Fatal("failed import changed existing graph")
		}
	}
}
