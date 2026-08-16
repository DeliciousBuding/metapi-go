package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestConfigureProxyUpstreamWiresPricingCatalogResolver verifies the wiring
// point: with PRICING_CATALOG_ENABLED the TokenRouter receives the models.dev
// provider, the provider fetches the configured catalog URL, and a seeded
// third-party relay site is honestly labeled catalog_estimate (never catalog).
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
		pricingCatalogProvider = nil
		pricingCatalogMu.Unlock()
		_ = store.CloseDatabase()
	})

	const catalogPayload = `{"openai":{"id":"openai","models":{"gpt-real":{"id":"gpt-real","cost":{"input":2.5,"output":10}}}}}`
	catalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(catalogPayload))
	}))
	t.Cleanup(catalogServer.Close)
	cfg.PricingCatalogURL = catalogServer.URL

	config.Set(cfg)
	if err := store.EnsureRuntimeDatabase(cfg); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	db := store.GetDB()
	seedProxyRoute(t, db, "http://relay.test/v1", "gpt-real", "upstream-token")

	if err := ConfigureProxyUpstream(cfg); err != nil {
		t.Fatalf("ConfigureProxyUpstream: %v", err)
	}

	pricingCatalogMu.Lock()
	provider := pricingCatalogProvider
	pricingCatalogMu.Unlock()
	if provider == nil {
		t.Fatal("catalog provider was not wired into the router")
	}

	// The provider fetches asynchronously at startup; poll for the live snapshot.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := provider.Snapshot().Lookup("gpt-real"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("catalog provider did not publish the test catalog within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}

	var siteID int64
	if err := db.QueryRow(`SELECT id FROM sites`).Scan(&siteID); err != nil {
		t.Fatalf("load seeded site: %v", err)
	}

	cost, source := provider.ResolveCatalogPricing(siteID, 0, "gpt-real")
	if cost == nil || *cost <= 0 {
		t.Fatalf("catalog cost for seeded model = %v, want positive", cost)
	}
	if source != pricingcatalog.SourceRelayEstimate {
		t.Errorf("seeded relay site source = %q, want %q (official list price must never be presented as a real relay price)", source, pricingcatalog.SourceRelayEstimate)
	}
}
