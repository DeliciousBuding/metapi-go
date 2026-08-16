package routing

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// TestPricingCatalogProviderImplementsResolver guards the structural contract
// between service/pricingcatalog and routing: the provider must satisfy
// routing.CatalogPricingResolver without importing routing (per design/BACKEND.md).
func TestPricingCatalogProviderImplementsResolver(t *testing.T) {
	provider := pricingcatalog.NewProvider(pricingcatalog.Options{})
	var resolver CatalogPricingResolver = provider
	_ = resolver
}

// TestPricingCatalogSourceConstantsStayInSync guards the honesty labels: the
// provider's provenance literals must match the routing constants so a relay
// estimate can never be re-labeled as an official catalog price.
func TestPricingCatalogSourceConstantsStayInSync(t *testing.T) {
	if pricingcatalog.SourceOfficial != CatalogSourceOfficial {
		t.Errorf("pricingcatalog.SourceOfficial = %q, want %q", pricingcatalog.SourceOfficial, CatalogSourceOfficial)
	}
	if pricingcatalog.SourceRelayEstimate != CatalogSourceRelayEstimate {
		t.Errorf("pricingcatalog.SourceRelayEstimate = %q, want %q", pricingcatalog.SourceRelayEstimate, CatalogSourceRelayEstimate)
	}
}
