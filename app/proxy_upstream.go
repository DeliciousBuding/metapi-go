package app

import (
	"context"
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
	router := routing.NewTokenRouter(routingStore, cfg, nil, proxyLoadProvider{coord: coord})
	decisionService := routing.NewRouteDecisionService(router, routingStore)
	setTokenRouteDecisionRuntime(router, decisionService)
	proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{
		Router:      router,
		Coordinator: coord,
		Executor:    proxy.NewRuntimeExecutor(requestTimeout),
		// Persist successful/failed proxy attempts (token usage when available).
		// Writer uses store.GetDB() so it follows runtime DB overrides in tests.
		LogProxy: func(ctx context.Context, entry proxy.ProxyLogEntry) error {
			return proxyhandler.InsertProxyLog(ctx, store.GetDB(), entry)
		},
	})
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
