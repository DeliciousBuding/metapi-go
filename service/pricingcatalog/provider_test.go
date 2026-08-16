package pricingcatalog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeSiteSource struct {
	sites map[int64]SiteSnapshot
	err   error
}

func (f fakeSiteSource) GetSiteSnapshot(ctx context.Context, siteID int64) (SiteSnapshot, bool, error) {
	if f.err != nil {
		return SiteSnapshot{}, false, f.err
	}
	site, ok := f.sites[siteID]
	return site, ok, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestProvider_ResolveCatalogPricing_OfficialAndRelay(t *testing.T) {
	sites := fakeSiteSource{sites: map[int64]SiteSnapshot{
		1: {Platform: "openai", URL: "https://api.openai.com/v1"},
		2: {Platform: "openai", URL: "https://relay.example.com/v1"},
		3: {Platform: "anyrouter", URL: "https://some-relay.example.com"},
	}}
	provider := NewProvider(Options{
		SiteSnapshotSource: sites,
		Logger:             testLogger(),
	})
	provider.snapshot.Store(mustParse(t, sampleCatalogJSON))

	// Official vendor site → "catalog".
	cost, source := provider.ResolveCatalogPricing(1, 10, "gpt-4o")
	if cost == nil || *cost != 0.0125 {
		t.Fatalf("official site cost = %v, want 0.0125", cost)
	}
	if source != SourceOfficial {
		t.Errorf("official site source = %q, want %q", source, SourceOfficial)
	}

	// Relay site → "catalog_estimate" (never a real payment price).
	cost, source = provider.ResolveCatalogPricing(2, 11, "gpt-4o")
	if cost == nil {
		t.Fatal("relay site must still receive an estimate for cold-start routing")
	}
	if source != SourceRelayEstimate {
		t.Errorf("relay site source = %q, want %q", source, SourceRelayEstimate)
	}

	// Non-vendor platform → relay estimate.
	_, source = provider.ResolveCatalogPricing(3, 12, "gpt-4o")
	if source != SourceRelayEstimate {
		t.Errorf("non-vendor platform source = %q, want %q", source, SourceRelayEstimate)
	}
}

func TestProvider_ResolveCatalogPricing_UnknownModelAndPresets(t *testing.T) {
	provider := NewProvider(Options{Logger: testLogger()})

	// Preset snapshot serves before any fetch.
	cost, _ := provider.ResolveCatalogPricing(1, 10, "gpt-4o")
	if cost == nil || *cost <= 0 {
		t.Fatalf("preset gpt-4o cost = %v, want positive", cost)
	}

	if cost, _ := provider.ResolveCatalogPricing(1, 10, "no-such-model-xyz"); cost != nil {
		t.Fatalf("unknown model cost = %v, want nil", *cost)
	}
}

func TestProvider_CatalogUnitCostFuncShape(t *testing.T) {
	provider := NewProvider(Options{Logger: testLogger()})
	provider.snapshot.Store(mustParse(t, sampleCatalogJSON))

	var query func(siteID, accountID int64, modelName string) *float64 = provider.CatalogUnitCost
	cost := query(1, 10, "gpt-4o")
	if cost == nil || *cost != 0.0125 {
		t.Fatalf("CatalogUnitCost = %v, want 0.0125", cost)
	}
	if cost := query(1, 10, "nope"); cost != nil {
		t.Fatalf("CatalogUnitCost unknown model = %v, want nil", *cost)
	}
}

func TestProvider_RefreshPublishesAndKeepsSnapshotOnFailure(t *testing.T) {
	payloads := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case payload := <-payloads:
			_, _ = w.Write([]byte(payload))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	provider := NewProvider(Options{
		FetchURL: server.URL,
		Logger:   testLogger(),
	})
	if provider.Snapshot().Source != PresetSnapshotSource {
		t.Fatalf("initial snapshot source = %q, want presets", provider.Snapshot().Source)
	}

	// First refresh fails → presets retained, error returned.
	if err := provider.Refresh(context.Background()); err == nil {
		t.Fatal("refresh against failing endpoint must error")
	}
	if provider.Snapshot().Source != PresetSnapshotSource {
		t.Fatalf("snapshot after failed refresh = %q, want presets retained", provider.Snapshot().Source)
	}

	// Second refresh succeeds → live snapshot published.
	payloads <- sampleCatalogJSON
	if err := provider.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if provider.Snapshot().Source != "models.dev api.json" {
		t.Fatalf("snapshot source = %q, want live catalog", provider.Snapshot().Source)
	}
	if _, ok := provider.Snapshot().Lookup("gpt-4o"); !ok {
		t.Fatal("refreshed snapshot must contain gpt-4o")
	}

	// Endpoint fails again → live snapshot retained (never downgraded).
	if err := provider.Refresh(context.Background()); err == nil {
		t.Fatal("refresh against failing endpoint must error")
	}
	if provider.Snapshot().Source != "models.dev api.json" {
		t.Fatalf("snapshot after later failure = %q, want live snapshot retained", provider.Snapshot().Source)
	}
}

func TestProvider_ClassifySiteErrorTreatsAsRelay(t *testing.T) {
	provider := NewProvider(Options{
		SiteSnapshotSource: fakeSiteSource{err: errors.New("db down")},
		Logger:             testLogger(),
	})
	provider.snapshot.Store(mustParse(t, sampleCatalogJSON))

	_, source := provider.ResolveCatalogPricing(1, 10, "gpt-4o")
	if source != SourceRelayEstimate {
		t.Errorf("classification error source = %q, want relay estimate (honest default)", source)
	}
}

func TestProvider_StartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleCatalogJSON))
	}))
	defer server.Close()

	provider := NewProvider(Options{
		FetchURL:        server.URL,
		RefreshInterval: 0, // no ticker
		Logger:          testLogger(),
	})
	provider.Start()
	defer provider.Stop()

	// Start is non-blocking; poll for the initial background fetch.
	deadline := time.Now().Add(5 * time.Second)
	for provider.Snapshot().Source != "models.dev api.json" {
		if time.Now().After(deadline) {
			t.Fatal("initial fetch did not publish within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func mustParse(t *testing.T, payload string) *CatalogSnapshot {
	t.Helper()
	snapshot, err := ParseCatalog([]byte(payload))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}
	return snapshot
}
