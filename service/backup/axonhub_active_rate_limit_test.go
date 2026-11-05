package backup

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubActiveRateLimitCompiles(t *testing.T) {
	raw := axonHubSettingsPayload(`{"rateLimit":{"rpm":3,"tpm":200,"maxConcurrent":2,"queueSize":4,"queueTimeoutMs":200}}`)
	preview, err := PreviewAxonHubV14(nil, raw, "active-limit-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.SkippedChannels) != 0 || preview.Routable["routes"] != 1 {
		t.Fatalf("active rate limits did not compile: skipped=%+v routable=%v", preview.SkippedChannels, preview.Routable)
	}
}

func TestAxonHubActiveRateLimitPersistenceAndReimport(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := axonHubSettingsPayload(`{"rateLimit":{"rpm":3,"tpm":200,"maxConcurrent":2,"queueSize":4,"queueTimeoutMs":200}}`)
	for range 2 {
		if _, err := ImportAxonHubV14(db, raw, "active-limit-fixture", false); err != nil {
			t.Fatal(err)
		}
		var actual store.DirectRateLimit
		if err := db.Get(&actual, `SELECT rate_limit FROM upstream_channels`); err != nil || actual.RPM != 3 || actual.TPM != 200 || actual.MaxConcurrent != 2 || actual.QueueSize != 4 || actual.QueueTimeoutMs != 200 {
			t.Fatalf("rate policy persistence=%+v err=%v", actual, err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials`) != 1 || countRows(t, db, `SELECT COUNT(*) FROM upstream_grants`) != 1 {
			t.Fatal("reimport duplicated the limited channel graph")
		}
	}
	if _, err := ImportAxonHubV14(db, axonHubSettingsPayload(`{}`), "active-limit-fixture", false); err != nil {
		t.Fatal(err)
	}
	var cleared store.DirectRateLimit
	if err := db.Get(&cleared, `SELECT rate_limit FROM upstream_channels`); err != nil || cleared.Enabled() {
		t.Fatalf("reimport retained removed source policy: %+v %v", cleared, err)
	}
}
