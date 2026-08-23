package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service/catalogsync"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestConfigureProxyUpstreamWiresPricingCatalogResolver verifies the wiring
// point: with PRICING_CATALOG_ENABLED the TokenRouter receives the catalog
// manager, the manager syncs the registered sources, and a seeded third-party
// relay site is honestly labeled catalog_estimate (never catalog).
func TestConfigureProxyUpstreamWiresPricingCatalogResolver(t *testing.T) {
	_ = store.CloseDatabase()
	cfg := testProxyConfig(t)
	cfg.PricingCatalogEnabled = true
	cfg.PricingCatalogURL = ""
	cfg.PricingCatalogRefreshMin = 0 // no periodic ticker in tests
	t.Cleanup(func() {
		proxyhandler.SetUpstreamConfig(nil)
		scheduler.SetActiveChannelIDsProvider(nil)
		ShutdownPricingCatalog()
		pricingCatalogMu.Lock()
		pricingCatalogManager = nil
		pricingCatalogMu.Unlock()
		_ = store.CloseDatabase()
	})

	const catalogPayload = `{"openai":{"id":"openai","models":{"gpt-real":{"id":"gpt-real","cost":{"input":2.5,"output":10}}}}}`
	catalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(catalogPayload))
	}))
	t.Cleanup(catalogServer.Close)

	config.Set(cfg)
	if err := store.EnsureRuntimeDatabase(cfg); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	db := store.GetDB()
	seedProxyRoute(t, db, "http://relay.test/v1", "gpt-real", "upstream-token")

	// Pre-seed the registry with a single custom source pointing at the test
	// server so EnsureDefaults does not seed the internet presets (and the
	// initial sync stays hermetic).
	if _, err := catalogsync.NewStore(db.DB).CreateSource(context.Background(), catalogsync.SourceInput{
		Name:    "test-catalog",
		URL:     catalogServer.URL,
		Enabled: boolPtr(true),
	}); err != nil {
		t.Fatalf("seed catalog source: %v", err)
	}

	if err := ConfigureProxyUpstream(cfg); err != nil {
		t.Fatalf("ConfigureProxyUpstream: %v", err)
	}

	manager := CatalogManager()
	if manager == nil {
		t.Fatal("catalog manager was not wired into the router")
	}

	// The manager fetches asynchronously at startup; poll for the live snapshot.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := manager.Snapshot().Lookup("gpt-real"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("catalog manager did not publish the test catalog within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}

	var siteID int64
	if err := db.QueryRow(`SELECT id FROM sites`).Scan(&siteID); err != nil {
		t.Fatalf("load seeded site: %v", err)
	}

	cost, source := manager.ResolveCatalogPricing(siteID, 0, "gpt-real")
	if cost == nil || *cost <= 0 {
		t.Fatalf("catalog cost for seeded model = %v, want positive", cost)
	}
	if source != pricingcatalog.SourceRelayEstimate {
		t.Errorf("seeded relay site source = %q, want %q (official list price must never be presented as a real relay price)", source, pricingcatalog.SourceRelayEstimate)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
