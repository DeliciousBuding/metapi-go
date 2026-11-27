package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/scheduler"
)

type proxyChannelHealthProbeAdapter struct {
	exec *proxy.ChannelHealthProbeExecutor
}

func (a proxyChannelHealthProbeAdapter) ProbeChannel(ctx context.Context, target scheduler.ProbeTarget) (scheduler.ProbeOutcome, error) {
	out, err := a.exec.ProbeChannel(ctx, proxy.ChannelProbeTarget{
		ChannelID: target.ChannelID,
		AccountID: target.AccountID,
		SiteID:    target.SiteID,
		ModelName: target.ModelName,
	})
	if err != nil {
		return scheduler.ProbeOutcome{}, err
	}
	return scheduler.ProbeOutcome{
		AccountID:  out.AccountID,
		SiteID:     out.SiteID,
		Status:     out.Status,
		LatencyMs:  out.LatencyMs,
		HTTPStatus: out.HTTPStatus,
		ErrorText:  out.ErrorText,
	}, nil
}

// activeProbeRouter is the TokenRouter built by ConfigureProxyUpstream.
// ModelProbeScheduler health recording feeds this router at boot.
var (
	probeWireMu       sync.RWMutex
	activeProbeRouter *routing.TokenRouter
	activeProbeCfg    *config.Config
)

// tokenRouterHealthRecorder adapts routing.TokenRouter to scheduler.ChannelHealthRecorder.
// Probe failures never mark credentials expired (router.RecordProbeFailure contract).
type tokenRouterHealthRecorder struct {
	router *routing.TokenRouter
}

func (r *tokenRouterHealthRecorder) RecordProbeSuccess(
	ctx context.Context,
	channelID int64,
	latencyMs float64,
	modelName *string,
	actualAccountID *int64,
) error {
	if r == nil || r.router == nil {
		return nil
	}
	return r.router.RecordProbeSuccess(ctx, channelID, latencyMs, modelName, actualAccountID)
}

func (r *tokenRouterHealthRecorder) RecordProbeFailure(
	ctx context.Context,
	channelID int64,
	httpStatus *int,
	errorText *string,
	modelName *string,
	actualAccountID *int64,
) error {
	if r == nil || r.router == nil {
		return nil
	}
	return r.router.RecordProbeFailure(ctx, channelID, routing.SiteRuntimeFailureContext{
		Status:    httpStatus,
		ErrorText: errorText,
		ModelName: modelName,
	}, actualAccountID)
}

// rememberProbeRouter stores the live TokenRouter for model-probe wiring.
func rememberProbeRouter(cfg *config.Config, router *routing.TokenRouter) {
	probeWireMu.Lock()
	defer probeWireMu.Unlock()
	activeProbeCfg = cfg
	activeProbeRouter = router
}

// WireModelProbeScheduler injects ChannelHealthProbe + ChannelHealthRecorder
// into a ModelProbeScheduler using the router from ConfigureProxyUpstream.
// Safe no-op when the proxy router has not been configured yet.
func WireModelProbeScheduler(s *scheduler.ModelProbeScheduler) {
	if s == nil {
		return
	}
	probeWireMu.RLock()
	router := activeProbeRouter
	cfg := activeProbeCfg
	probeWireMu.RUnlock()

	if router == nil {
		slog.Debug("model-probe wire: TokenRouter not ready; probe executor deferred")
		return
	}
	if cfg == nil {
		cfg = config.Get()
	}

	probe := proxy.NewChannelHealthProbeExecutor(cfg)
	s.SetProbeExecutor(proxyChannelHealthProbeAdapter{exec: probe})
	s.SetHealthRecorder(&tokenRouterHealthRecorder{router: router})
	slog.Info("model-probe: probe executor and health recorder wired")
}

// WireGlobalModelProbeScheduler injects deps into the process-global scheduler
// if one is already registered (admin / recovery paths).
func WireGlobalModelProbeScheduler() {
	WireModelProbeScheduler(scheduler.GetGlobalModelProbeScheduler())
}
