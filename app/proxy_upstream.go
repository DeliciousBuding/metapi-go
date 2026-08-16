package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"github.com/deliciousbuding/metapi-go/store"
)

// ConfigureProxyUpstream wires the /v1 data plane to the runtime database.
// Without this, proxy handlers can parse requests but cannot select real channels.
// Also publishes the TokenRouter / RouteDecisionService for admin decision APIs.
func ConfigureProxyUpstream(cfg *config.Config) error {
	db := store.GetDB()
	if db == nil {
		proxyhandler.SetUpstreamConfig(nil)
		setTokenRouteDecisionRuntime(nil, nil)
		// Clear channel-recovery active-ID hook on failed/cleared reconfigure.
		scheduler.SetActiveChannelIDsProvider(nil)
		return fmt.Errorf("proxy upstream: database is not initialized")
	}
	coord := proxy.NewProxyChannelCoordinator(cfg)
	// Overall HTTP client timeout is a safety ceiling for full request lifetime.
	// Observed first-byte timeout is separate: PROXY_FIRST_BYTE_TIMEOUT_SEC
	// (seconds) is converted to milliseconds via proxy.FirstByteTimeoutMs and
	// applied per attempt in handler/proxy sendUpstreamRequest.
	// Keep client timeout at least as large as the first-byte window so the
	// client does not pre-empt first-byte observation.
	requestTimeout := 90 * time.Second
	if firstByteMs := proxy.FirstByteTimeoutMs(cfg.ProxyFirstByteTimeoutSec); firstByteMs > 0 {
		fb := time.Duration(firstByteMs) * time.Millisecond
		// Ceiling: max(90s, first-byte*2) so multi-endpoint fallback can still complete.
		if doubled := fb * 2; doubled > requestTimeout {
			requestTimeout = doubled
		}
	}
	auth.ConfigureSharedAdmissionFromRedisURL(cfg.RedisURL)

	routingStore := service.NewProxyRoutingStore(db)
	router := routing.NewTokenRouter(routingStore, cfg, pricingCatalogResolver(cfg, db), proxyLoadProvider{coord: coord})
	decisionService := routing.NewRouteDecisionService(router, routingStore)
	setTokenRouteDecisionRuntime(router, decisionService)
	proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{
		Router:      router,
		Coordinator: coord,
		Executor:    proxy.NewRuntimeExecutor(requestTimeout),
		// Persist successful/failed proxy attempts (token usage when available).
		// EnqueueProxyLog routes through the async batch writer when
		// PROXY_LOG_ASYNC is enabled (default), falling back to a synchronous
		// InsertProxyLog in sync mode and under channel backpressure. The
		// writer resolves store.GetDB() at flush time so runtime overrides in
		// tests are honored.
		LogProxy: func(ctx context.Context, entry proxy.ProxyLogEntry) error {
			return proxyhandler.EnqueueProxyLog(ctx, entry)
		},
	})
	// Configure the proxy_log batch writer from env. Idempotent: a reconfigure
	// first drains the previous writer so no goroutine or in-flight entry leaks.
	proxyhandler.ConfigureProxyLogWriter(
		cfg.ProxyLogAsync,
		cfg.ProxyLogBatchSize,
		cfg.ProxyLogFlushIntervalMs,
	)
	// Channel recovery active candidates follow coordinator leases.
	// Normalize nil→empty so an empty lease set does not look "unset".
	scheduler.SetActiveChannelIDsProvider(func() []int64 {
		ids := coord.GetActiveChannelIDs()
		if ids == nil {
			return []int64{}
		}
		return ids
	})
	// Remember router for ModelProbeScheduler health recording.
	// Wire global scheduler if already started (reconfigure / test paths).
	rememberProbeRouter(cfg, router)
	WireGlobalModelProbeScheduler()
	return nil
}

var (
	tokenRouteDecisionMu      sync.RWMutex
	tokenRouteDecisionRouter  *routing.TokenRouter
	tokenRouteDecisionService *routing.RouteDecisionService
)

func setTokenRouteDecisionRuntime(router *routing.TokenRouter, decisions *routing.RouteDecisionService) {
	tokenRouteDecisionMu.Lock()
	defer tokenRouteDecisionMu.Unlock()
	tokenRouteDecisionRouter = router
	tokenRouteDecisionService = decisions
}

// TokenRouteDecisionRuntime returns the TokenRouter and RouteDecisionService
// published by ConfigureProxyUpstream. Both may be nil when upstream is not wired.
// Used by router/admin registration without creating an app↔admin import cycle.
func TokenRouteDecisionRuntime() (*routing.TokenRouter, *routing.RouteDecisionService) {
	tokenRouteDecisionMu.RLock()
	defer tokenRouteDecisionMu.RUnlock()
	return tokenRouteDecisionRouter, tokenRouteDecisionService
}

type proxyLoadProvider struct {
	coord *proxy.ProxyChannelCoordinator
}

func (p proxyLoadProvider) GetChannelLoadSnapshot(params routing.ChannelLoadParams) routing.ChannelLoadSnapshot {
	if p.coord == nil {
		return routing.ChannelLoadSnapshot{}
	}
	snap := p.coord.GetChannelLoadSnapshot(params.ChannelID, params.AccountExtraConfig, params.AccountOAuthProvider)
	return routing.ChannelLoadSnapshot{
		SessionScoped:    snap.SessionScoped,
		ConcurrencyLimit: snap.ConcurrencyLimit,
		ActiveLeaseCount: snap.ActiveLeaseCount,
		WaitingCount:     snap.WaitingCount,
		Saturated:        snap.Saturated,
	}
}

// ShutdownProxyLogBatchWriter drains and stops the global proxy_log batch
// writer. It is intended to be registered as an app OnClose hook during server
// startup so the writer flushes its final batch before app.cleanup() calls
// store.CloseDatabase(). Exposed here (rather than having cmd/server import
// handler/proxy directly) to keep the cmd → app package boundary intact.
// No-op when the writer was never started (sync mode / PROXY_LOG_ASYNC=false).
func ShutdownProxyLogBatchWriter(ctx context.Context) error {
	return proxyhandler.ShutdownProxyLogBatchWriter(ctx)
}

var (
	pricingCatalogMu       sync.Mutex
	pricingCatalogProvider *pricingcatalog.Provider
)

// pricingCatalogResolver returns the process-global models.dev catalog pricing
// provider wired into the TokenRouter as the cold-start cost signal, or nil
// when PRICING_CATALOG_ENABLED=false. Reconfigure calls reuse the existing
// provider (and its refresh loop) instead of stacking duplicate tickers.
func pricingCatalogResolver(cfg *config.Config, db *store.DB) routing.CatalogPricingResolver {
	if cfg == nil || !cfg.PricingCatalogEnabled {
		return nil
	}
	pricingCatalogMu.Lock()
	defer pricingCatalogMu.Unlock()
	if pricingCatalogProvider != nil {
		return pricingCatalogProvider
	}
	provider := pricingcatalog.NewProvider(pricingcatalog.Options{
		RefreshInterval:    time.Duration(cfg.PricingCatalogRefreshMin) * time.Minute,
		FetchURL:           cfg.PricingCatalogURL,
		SiteSnapshotSource: &siteSnapshotSource{db: db},
	})
	provider.Start()
	pricingCatalogProvider = provider
	return provider
}

// siteSnapshotSource resolves site platform/URL from the runtime DB for
// vendor classification (official vendor host vs third-party relay).
type siteSnapshotSource struct {
	db *store.DB
}

func (s *siteSnapshotSource) GetSiteSnapshot(ctx context.Context, siteID int64) (pricingcatalog.SiteSnapshot, bool, error) {
	if s == nil || s.db == nil {
		return pricingcatalog.SiteSnapshot{}, false, nil
	}
	var row struct {
		Platform string `db:"platform"`
		URL      string `db:"url"`
	}
	err := s.db.GetContext(ctx, &row, s.db.Rebind("SELECT platform, url FROM sites WHERE id = ?"), siteID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return pricingcatalog.SiteSnapshot{}, false, nil
		}
		return pricingcatalog.SiteSnapshot{}, false, err
	}
	return pricingcatalog.SiteSnapshot{Platform: row.Platform, URL: row.URL}, true, nil
}

// ShutdownPricingCatalog stops the catalog pricing refresh loop. Registered as
// an app OnClose hook during server startup; no-op when the catalog was never
// enabled.
func ShutdownPricingCatalog() {
	pricingCatalogMu.Lock()
	provider := pricingCatalogProvider
	pricingCatalogMu.Unlock()
	if provider != nil {
		provider.Stop()
	}
}
