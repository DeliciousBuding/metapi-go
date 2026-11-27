package proxyhandler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
)

// observeProxyTerminal records labeled counters, latency histogram, and optional export hook.
// Labels are privacy-safe (endpoint family + outcome); never model/key/body.
func observeProxyTerminal(ctx *Ctx, status string, stream bool, latency time.Duration) {
	if ctx != nil {
		ctx.lifecycle.observe(status, stream)
	}
	endpoint := shared.EndpointOther
	if ctx != nil && ctx.DownstreamPath != "" {
		endpoint = shared.EndpointLabelFromPath(ctx.DownstreamPath)
	}
	shared.ObserveProxyOutcome(shared.ProxyObservation{
		Endpoint: endpoint,
		Status:   status,
		Stream:   stream,
		Latency:  latency,
	})
}

// streamFailureVerdict is the single owner of "how a non-normal stream ending
// is reported". It maps a streamOutcome to the HTTP status recorded on the
// channel failure and the proxy_log row, the reason text, and the terminal
// metric outcome, so channel health, logs and metrics can never disagree about
// the same stream again.
//
// ok=false means the ending is not an upstream fault — clean EOF, or the
// downstream client went away — and must be recorded through the success path.
func streamFailureVerdict(end streamOutcome, idleTimeoutSec int) (status int, reason string, terminal string, ok bool) {
	switch end {
	case streamEndedIdleTimeout:
		return http.StatusRequestTimeout,
			fmt.Sprintf("SSE stream idle timeout: no upstream chunk within %ds", idleTimeoutSec),
			shared.OutcomeTimeout, true
	case streamEndedUpstreamFault:
		return http.StatusBadGateway,
			"SSE stream interrupted: upstream closed the connection before completing the response",
			shared.OutcomeUpstreamError, true
	case streamEndedTruncated:
		return http.StatusBadGateway,
			fmt.Sprintf("SSE stream truncated: response exceeded the configured byte limit (%d bytes)", streamResponseByteLimit()),
			shared.OutcomeUpstreamError, true
	case streamEndedNormally, streamEndedClientDisconnect:
		return 0, "", "", false
	}
	return 0, "", "", false
}

// endpointFailureClass tells the single fallback decision function how the
// failure was observed, so intentional policy differences are explicit
// parameters instead of a second copy of the decision at the call site.
type endpointFailureClass int

const (
	// endpointFailureResponse: the upstream answered with an HTTP status and a
	// body, so both the same-site abort policy and the protocol-hint downgrade
	// list apply.
	endpointFailureResponse endpointFailureClass = iota
	// endpointFailureTransport: another transport failure, such as request
	// construction or body reading. The status passed in is the one the failure
	// is reported with, so the same-site abort policy applies to transport
	// errors exactly as it does to responses.
	endpointFailureTransport
	// A typed dial/DNS failure cannot be repaired by changing the HTTP path.
	// Classification does not depend on localized socket error messages.
	endpointFailureDial
	// endpointFailureFirstByteTimeout: the per-attempt budget expired before
	// headers arrived. Intentionally exempt from the same-site abort policy —
	// a first-byte timeout is a budget verdict about one attempt, not evidence
	// the site is systemically down, so the next protocol candidate is still
	// tried and the channel is not poisoned.
	endpointFailureFirstByteTimeout
)

func classifyEndpointTransportFailure(err error) endpointFailureClass {
	var dialErr *net.OpError
	var dnsErr *net.DNSError
	if (errors.As(err, &dialErr) && dialErr.Op == "dial") || errors.As(err, &dnsErr) {
		return endpointFailureDial
	}
	return endpointFailureTransport
}

// shouldContinueEndpointFallback is the ONE owner of "try the next protocol
// candidate?". Every fallback decision in the dispatch path — HTTP status,
// content failure and transport-level errors alike — goes through it.
func shouldContinueEndpointFallback(status int, rawErrText string, isLastEndpoint bool, disableCrossProtocolFallback bool, class endpointFailureClass) bool {
	if isLastEndpoint || disableCrossProtocolFallback {
		return false
	}
	if class == endpointFailureDial {
		return false
	}
	if class != endpointFailureFirstByteTimeout && proxy.ShouldAbortSameSiteEndpointFallback(status, rawErrText) {
		return false
	}
	if proxy.ShouldDowngradeToNextEndpoint(status, rawErrText) {
		return true
	}
	switch class {
	case endpointFailureTransport, endpointFailureFirstByteTimeout:
		// No HTTP answer means no protocol hint to match: the failure is not
		// attributable to this endpoint shape, so the next candidate is still
		// worth trying (unless the abort policy above said otherwise).
		return true
	case endpointFailureResponse:
	}
	// First-byte style status=0 should already be handled by error path; treat as continue.
	if status == 0 {
		return true
	}
	return false
}

type pendingUpstreamFailure struct {
	resp        *http.Response
	bodyBytes   []byte
	jsonStatus  int
	jsonMessage string
	jsonType    string
}

func bufferedPendingUpstreamFailure(resp *http.Response, bodyBytes []byte) *pendingUpstreamFailure {
	return &pendingUpstreamFailure{resp: resp, bodyBytes: bodyBytes}
}

func jsonPendingUpstreamFailure(status int, message, typ string) *pendingUpstreamFailure {
	return &pendingUpstreamFailure{
		jsonStatus:  status,
		jsonMessage: message,
		jsonType:    typ,
	}
}

func (p *pendingUpstreamFailure) outcomeStatus() string {
	if p == nil {
		return shared.OutcomeUnavailable
	}
	if p.resp != nil {
		return shared.StatusFromHTTP(p.resp.StatusCode)
	}
	return shared.StatusFromHTTP(p.jsonStatus)
}

func (p *pendingUpstreamFailure) reason() string {
	if p == nil {
		return "upstream request failed"
	}
	if p.resp != nil {
		return fmt.Sprintf("upstream status %d", p.resp.StatusCode)
	}
	if p.jsonMessage != "" {
		return p.jsonMessage
	}
	return "upstream request failed"
}

func (p *pendingUpstreamFailure) recordFirstFailure(ctx *Ctx) {
	if p == nil || ctx == nil || ctx.lifecycle == nil {
		return
	}
	status := p.jsonStatus
	if p.resp != nil {
		status = p.resp.StatusCode
	}
	ctx.lifecycle.mu.Lock()
	defer ctx.lifecycle.mu.Unlock()
	if ctx.lifecycle.firstFailure == "" {
		ctx.lifecycle.firstFailure = upstreamFailureReason(status)
	}
}

func (p *pendingUpstreamFailure) write(w http.ResponseWriter, db *store.DB, requestID, model string) {
	if p == nil {
		reportProxyAllFailed(db, model, "no available channels")
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "No available channels", "server_error", requestID)
		return
	}
	if p.resp != nil {
		relayBufferedUpstreamResponse(w, p.resp, p.bodyBytes)
		return
	}
	status := p.jsonStatus
	if status == 0 {
		status = http.StatusBadGateway
	}
	message := p.jsonMessage
	if message == "" {
		message = "Upstream request failed"
	}
	typ := p.jsonType
	if typ == "" {
		typ = "upstream_error"
	}
	writeJSONErrorWithRequest(w, status, message, typ, requestID)
}

func recordUpstreamFailure(ctx context.Context, cfg *UpstreamConfig, selected *routing.SelectedChannel, modelName string, status int, rawErrText string) {
	if cfg == nil || cfg.Router == nil || selected == nil {
		return
	}
	if !proxy.ShouldRecordChannelFailure(status, rawErrText) {
		return
	}
	failureCtx := routing.SiteRuntimeFailureContext{
		ErrorText: &rawErrText,
		ModelName: &modelName,
	}
	if status > 0 {
		failureCtx.Status = &status
	}
	if err := cfg.Router.RecordFailure(ctx, selected.Channel.ID, failureCtx, selectedHealthAccountID(selected)); err != nil {
		slog.Warn("RecordFailure failed", "err", err, "channel_id", selected.Channel.ID, "account_id", selected.Account.ID, "model", modelName)
	}
}

// recordUpstreamSuccess feeds routing success stats. billingCostName is the
// attribution (requested/canonical) model — the same name proxy_logs and
// billing use, so a model redirect (canonical→actual) never skews channel cost
// accumulation; modelName (actual) stays the health-stat label.
func recordUpstreamSuccess(ctx context.Context, cfg *UpstreamConfig, selected *routing.SelectedChannel, billingCostName, modelName string, latencyMs int64, usage ParsedUsage) {
	if cfg == nil || cfg.Router == nil || selected == nil {
		return
	}
	billing := EstimateBillingCostFromUsageWithCatalog(billingCostName, selectedBillingPlatform(selected), usage, currentBillingCatalog(), selectedBillingClass(ctx, selected))
	if usage.videoAccounting != nil {
		billing.EstimatedCost = 0
	}
	if err := cfg.Router.RecordSuccess(ctx, selected.Channel.ID, float64(latencyMs), billing.EstimatedCost, &modelName, selectedHealthAccountID(selected)); err != nil {
		slog.Warn("RecordSuccess failed", "err", err, "channel_id", selected.Channel.ID, "account_id", selected.Account.ID, "model", modelName)
		return
	}
	// Soft-feed first-byte EMA: until header timing is plumbed separately, use
	// total latency as an upper bound so faster channels still score better.
	siteID := selected.Account.SiteID
	if siteID != 0 {
		routing.RecordSiteRuntimeSuccess(siteID, float64(latencyMs), &modelName, float64(latencyMs))
	}
}
