package pricingcatalog

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
)

// maxCatalogPayloadBytes bounds the catalog payload read (the datasets are
// ~2 MB; the bound protects against an unexpectedly large response body).
const maxCatalogPayloadBytes = 32 << 20

// newCatalogHTTPClient builds the default source-fetch client. Catalog
// sources are an admin-configurable outbound URL, so the transport uses the
// public-only dial guard and pins the validated DNS answer: a source cannot
// point the server at loopback/RFC1918/ULA/metadata, and a hostname cannot
// pass validation and then resolve privately on the dial (audit L10).
func newCatalogHTTPClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dial := ssrf.NewPublicOnlyDialContext(net.DefaultResolver, dialer.DialContext)
	if allowPrivate {
		// Operators may host an internal catalog mirror. Private targets are
		// still dialed by the pinned IP, so DNS rebinding stays closed.
		dial = ssrf.NewSiteDialContext(net.DefaultResolver, dialer.DialContext)
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dial,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       30 * time.Second,
		},
	}
}

// fetchSource fetches + parses one source and updates its runtime state.
// Returns nil on failure; the runtime keeps the previous last-good snapshot.
func (p *Provider) fetchSource(ctx context.Context, spec SourceSpec) *CatalogSnapshot {
	attemptedAt := time.Now()
	data, err := p.fetch(ctx, spec.URL)
	var snapshot *CatalogSnapshot
	if err == nil {
		snapshot, err = parseSourceData(spec.Kind, data)
	}
	if err == nil && snapshot != nil {
		for key, entry := range snapshot.models {
			entry.PriceSource = spec.Name
			entry.PriceFetchedAt = attemptedAt
			snapshot.models[key] = entry
		}
		snapshot.claudeAliases = make(map[string]CatalogEntry)
		for _, entry := range snapshot.models {
			snapshot.addAlias(entry)
		}
	}

	p.sourcesMu.Lock()
	found := false
	for i := range p.sources {
		if sourceKey(p.sources[i].spec) == sourceKey(spec) {
			p.sources[i].attemptedAt = attemptedAt
			if err != nil {
				p.sources[i].lastError = err.Error()
				p.logger.Warn("pricingcatalog: source fetch failed", "source", spec.Name, "url", spec.URL, "error", err)
			} else {
				p.sources[i].lastGood = snapshot
				now := time.Now()
				p.sources[i].lastSuccess = &now
				p.sources[i].lastError = ""
			}
			found = true
			break
		}
	}
	p.sourcesMu.Unlock()
	// Anonymous single-source mode (legacy FetchURL) has no runtime slot:
	// treat a failed fetch as nil.
	if !found {
		if err != nil {
			p.logger.Warn("pricingcatalog: source fetch failed", "source", spec.Name, "url", spec.URL, "error", err)
		}
		return snapshot
	}
	if err != nil {
		return nil
	}
	return snapshot
}

// parseSourceData parses payload bytes with the source-kind-selected parser.
// SourceKindAuto tries llm-metadata (superset shape) first, then the newapi
// ratio payloads, and falls back to the models.dev parser — so official
// datasets, ratio endpoints and custom mirrors of either shape work without
// URL sniffing.
func parseSourceData(kind SourceKind, data []byte) (*CatalogSnapshot, error) {
	switch kind {
	case SourceKindLLMMetadata:
		return ParseLLMMetadata(data)
	case SourceKindModelsDev:
		return ParseCatalog(data)
	case SourceKindNewAPIRatios:
		return ParseNewAPIRatios(data)
	default:
		if isNewAPIPayload(data) {
			return ParseNewAPIRatios(data)
		}
		snapshot, err := ParseLLMMetadata(data)
		if err == nil && snapshot.Len() > 0 {
			return snapshot, nil
		}
		return ParseCatalog(data)
	}
}

func (p *Provider) fetch(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: build request: %w", err)
	}
	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: fetch %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricingcatalog: fetch %s: unexpected status %d", url, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogPayloadBytes))
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: read %s: %w", url, err)
	}
	return data, nil
}
