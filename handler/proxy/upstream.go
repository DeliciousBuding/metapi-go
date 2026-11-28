package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/alert"
	"github.com/deliciousbuding/metapi-go/store"
)

// dispatchUpstream forwards a proxy request to the selected upstream channel.
// Implements the spec's 10-step Handler pattern.
func dispatchUpstream(w http.ResponseWriter, r *http.Request, ctx *Ctx) {
	if ctx.lifecycle != nil {
		ctx.lifecycle.bind(ctx.Auth, ctx.DownstreamPath, ctx.RequestedModel)
		ctx.lifecycle.mu.Lock()
		ctx.lifecycle.dispatched = true
		ctx.lifecycle.mu.Unlock()
	}
	r = withDownstreamResponseModel(r, ctx)
	shared.RecordProxyRequest()
	startedAt := time.Now()
	// Parent request/trace id is stable across channel retries and endpoint fallbacks.
	reqCtx, requestID := proxy.EnsureRequestID(r.Context(), "")
	if requestID != "" && r.Context() != reqCtx {
		r = r.WithContext(reqCtx)
	}
	cfg := getUpstreamConfig()
	if cfg == nil {
		if isProxyStubEnabled() {
			writeStubResponse(w, ctx)
			return
		}
		unconfiguredUpstreamLogOnce.Do(func() {
			slog.Error("proxy upstream dependencies are not configured", "request_id", requestID)
		})
		if ctx.lifecycle != nil {
			ctx.lifecycle.mu.Lock()
			ctx.lifecycle.gatewayFailure = "upstream_not_configured"
			ctx.lifecycle.mu.Unlock()
		}
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "Proxy upstream is not configured", "server_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUnavailable, ctx != nil && ctx.IsStream, time.Since(startedAt))
		return
	}

	excludeChannelIDs := make([]int64, 0)
	ctx.retryBudget = &upstreamRetryBudget{}
	maxRetries := ctx.MaxRetries
	downstreamPolicy := RoutingPolicyFromAuth(ctx.Policy)
	requestStream := ctx.IsStream
	downstreamPolicy.RequestStream = &requestStream
	downstreamPolicy.TraceIdentity = requestTraceIdentity(r.Context(), ctx.Auth, r.Header, ctx.RequestedModel)
	// multi-tier: best-effort request context estimate for route tier pick.
	// ctx is constructed non-nil by PrepareCtx (which returns a SurfResult on
	// failure instead of a nil Ctx), so it is safe to dereference unguarded.
	downstreamPolicy.RequestedContextTokens = routing.EstimateRequestContextTokens(ctx.Body)
	upstreamPath := ctx.DownstreamPath
	if upstreamPath == "" {
		upstreamPath = r.URL.Path
	}
	downstreamPolicy.RequiredUpstreamProtocol, downstreamPolicy.AllowUpstreamProtocolConversion = proxy.RoutingProtocolForPath(upstreamPath)
	downstreamPolicy.AllowStreamAggregation = directStreamAggregateProtocol(upstreamPath) != "" && !strings.HasSuffix(upstreamPath, "/count_tokens")
	downstreamPolicy.ResolveAssociationRequest = associationRequestFromContext(r.Context())
	if downstreamPolicy.ResolveAssociationRequest == nil {
		downstreamPolicy.ResolveAssociationRequest = lazyAssociationRequest(upstreamPath, ctx.RawBody, r.Header, ctx.IsStream, time.Now())
	}
	ctx.dispatchPolicy = &downstreamPolicy
	// Never forward a downstream path that can escape the selected site's API
	// prefix. Go's http.Client preserves ".." on the wire, and the upstream host
	// would normalize it outside the prefix — letting any authenticated
	// downstream key holder reach arbitrary upstream paths. chi hands back the
	// raw segment when RawPath is set, so the guard also decodes encoded
	// separators/dots and re-checks the files/videos resource prefixes after
	// path.Clean. Traversal-free paths pass through byte-for-byte.
	if !validUpstreamDispatchPath(upstreamPath) {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, "Invalid request path", "invalid_request_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeClientError, ctx != nil && ctx.IsStream, time.Since(startedAt))
		return
	}
	if endpoint, ok := proxy.EndpointFromPath(upstreamPath); ok && endpoint == proxy.EndpointMessages {
		channelID, required, err := messagesBridgePreferredChannel(ctx)
		ctx.messagesBridgeReplayRequired = required
		if err != nil || (required && (channelID == nil || *channelID == 0)) {
			writeMessagesReplayFailure(w, ctx, requestID)
			return
		}
		if required {
			if ctx.ForcedChannelID != nil && *ctx.ForcedChannelID != 0 && *ctx.ForcedChannelID != *channelID {
				writeMessagesReplayFailure(w, ctx, requestID)
				return
			}
			// This is not an authorization bypass: the normal preferred-channel
			// selector still applies current downstream policy and availability.
			ctx.ForcedChannelID = channelID
			maxRetries = 0
		}
	}
	var pendingFailure *pendingUpstreamFailure
	// The selection round and the upstream attempt budget are deliberately
	// separate. A saturated direct credential only skips candidates, so its
	// grants (models) must not consume real retries while they are skipped.
	// Regular site saturation keeps its historical budget behavior: it still
	// advances the attempt counter, exactly as the old retry loop did.
	attempt := 0
	selectionRound := 0
	lastSaturationReason := ""
	rejectedDirectChannels := make(map[int64]bool)
	directLimit := int64(directAccountConcurrencyLimit())

	for attempt <= maxRetries {
		if attempt > 0 && ctx.retryBudgetExpired() {
			break
		}
		// Step 6: Channel selection
		selected, err := proxy.SelectProxyChannelForAttempt(
			r.Context(),
			cfg.Router,
			proxy.ChannelSelectionInput{
				RequestedModel:    ctx.RequestedModel,
				DownstreamPolicy:  downstreamPolicy,
				ExcludeChannelIDs: excludeChannelIDs,
				RetryCount:        selectionRound,
				ForcedChannelID:   ctx.ForcedChannelID,
			},
		)
		if err != nil || selected == nil {
			// Ask why. Selection reports "nothing selectable" as (nil, nil), so
			// without this the log line reads err=<nil> and the client gets a
			// bare "No available channels" — no way to tell an unmatched route
			// from unbound tokens, cooldown or a downstream key policy, which
			// have three different fixes (#1179).
			noChannelReason := proxy.ExplainNoChannel(
				r.Context(), cfg.Router, ctx.RequestedModel, excludeChannelIDs, downstreamPolicy)
			slog.Warn("channel selection failed",
				"err", err,
				"model", ctx.RequestedModel,
				"reason", noChannelReason,
				"retry", attempt,
				"request_id", requestID,
			)
			if pendingFailure != nil {
				reportProxyAllFailedTerminal(ctx, cfg, pendingFailure.reason())
				pendingFailure.write(w, resolveUpstreamDB(ctx, cfg), requestID, ctx.RequestedModel)
				observeProxyTerminal(ctx, pendingFailure.outcomeStatus(), ctx != nil && ctx.IsStream, time.Since(startedAt))
				return
			}
			eventReason := "no available channels"
			if isDirectAdmissionReason(lastSaturationReason) {
				eventReason = lastSaturationReason
			}
			message := "No available channels"
			if noChannelReason != "" {
				eventReason += ": " + noChannelReason
				message += ": " + noChannelReason
			}
			reportProxyAllFailedTerminal(ctx, cfg, eventReason)
			writeJSONErrorWithRequest(w, 503, message, "server_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeUnavailable, ctx != nil && ctx.IsStream, time.Since(startedAt))
			return
		}
		// A selector that returns an already-excluded channel breaks its
		// exclusion contract. Only the direct verdict below uses this flag:
		// direct saturation does not spend the attempt budget, so a repeated
		// channel needs an explicit stop instead of an arbitrary skip cap.
		repeatedSelection := slices.Contains(excludeChannelIDs, selected.Channel.ID)
		selectionRound++
		excludeChannelIDs = append(excludeChannelIDs, selected.Channel.ID)
		if selected.Direct != nil && rejectedDirectChannels[selected.Direct.ChannelID] {
			if repeatedSelection {
				break
			}
			continue
		}

		// Wait for the channel before taking the independent credential slot.
		// A queued channel must not starve unrelated channels sharing an account.
		channelPermit, admission, admissionErr := acquireChannelAdmission(r.Context(), cfg, selected)
		if admissionErr != nil {
			if r.Context().Err() != nil {
				w.WriteHeader(499)
				observeProxyTerminal(ctx, shared.OutcomeClientError, ctx.IsStream, time.Since(startedAt))
				return
			}
			lastSaturationReason = admissionErr.Error()
			if selected.Direct != nil {
				rejectedDirectChannels[selected.Direct.ChannelID] = true
			}
			if ctx.lifecycle != nil {
				ctx.lifecycle.mu.Lock()
				ctx.lifecycle.noRouteReason = lastSaturationReason
				ctx.lifecycle.mu.Unlock()
			}
			if repeatedSelection {
				break
			}
			continue
		}
		if channelPermit != nil && channelPermit.Waited {
			var status int
			selected, status, err = revalidateQueuedChannel(r, ctx, cfg, selected, &downstreamPolicy)
			if err != nil {
				channelPermit.Release()
				writeJSONErrorWithRequest(w, status, err.Error(), "permission_error", requestID)
				observeProxyTerminal(ctx, shared.OutcomeUnavailable, ctx.IsStream, time.Since(startedAt))
				return
			}
			if ctx.Auth != nil {
				r = r.WithContext(auth.WithProxyAuth(r.Context(), ctx.Auth))
			}
		}

		// Concurrency admission (orthogonal to channel leases).
		// Regular channels take a site-scoped slot (sites.max_concurrency);
		// direct channels have no site row, so they take an account-scoped
		// slot keyed by upstream credential ID. On saturation: skip to the
		// next channel — do NOT record failure or start a cooldown cascade.
		siteLimiter := cfg.SiteLimiter
		if siteLimiter == nil {
			siteLimiter = proxy.DefaultSiteConcurrencyLimiter
		}
		var attemptSlot *proxy.SiteSlot
		var acquired bool
		saturationReason := "site_concurrency_saturated"
		credentialID := int64(0)
		if selected.Direct != nil {
			credentialID = selected.Direct.CredentialID
			directLimiter := cfg.DirectAccountLimiter
			if directLimiter == nil {
				directLimiter = proxy.DefaultDirectAccountConcurrencyLimiter
			}
			attemptSlot, acquired = directLimiter.TryAcquire(credentialID, directLimit)
			if !acquired {
				saturationReason = "direct_account_concurrency_saturated"
			}
		} else {
			attemptSlot, acquired = siteLimiter.TryAcquire(selected.Site.ID, selected.Site.MaxConcurrency)
		}
		if !acquired {
			channelPermit.Release()
			lastSaturationReason = saturationReason
			if ctx.lifecycle != nil {
				ctx.lifecycle.mu.Lock()
				ctx.lifecycle.noRouteReason = saturationReason
				ctx.lifecycle.mu.Unlock()
			}
			maxConcurrency := selected.Site.MaxConcurrency
			if selected.Direct != nil {
				maxConcurrency = directLimit
			}
			slog.Debug("concurrency saturated; skipping channel without failure cascade",
				"reason", saturationReason,
				"site_id", selected.Site.ID,
				"credential_id", credentialID,
				"channel_id", selected.Channel.ID,
				"max_concurrency", maxConcurrency,
				"model", ctx.RequestedModel,
				"retry", attempt,
				"request_id", requestID,
			)
			if selected.Direct != nil {
				if repeatedSelection {
					slog.Warn("router returned an already-excluded channel; stopping saturation skip scan",
						"channel_id", selected.Channel.ID,
						"credential_id", credentialID,
						"request_id", requestID,
					)
					break
				}
				// Saturated direct account: move the selection cursor, not the
				// upstream retry budget.
				continue
			}
			// Historical site saturation behavior: the skip consumed one retry.
			attempt++
			continue
		}

		// Hold the slot for the full attempt, including the streamed body;
		// always release, including on the panic path.
		var finished bool
		var nextPending *pendingUpstreamFailure
		func() {
			defer channelPermit.Release()
			if attemptSlot != nil {
				defer attemptSlot.Release()
			}
			attemptRequest := r
			if admission != nil {
				attemptRequest = r.WithContext(context.WithValue(r.Context(), channelAdmissionKey{}, admission))
			}
			finished, nextPending = dispatchSelectedUpstream(w, attemptRequest, ctx, cfg, selected, upstreamPath, attempt, maxRetries, requestID)
		}()
		if finished {
			return
		}
		if nextPending != nil {
			nextPending.recordFirstFailure(ctx)
			pendingFailure = nextPending
		}
		if selected.AffinityPreferred && selected.Direct != nil {
			rejectedDirectChannels[selected.Direct.ChannelID] = true
		}
		if admission != nil && admission.denied != nil {
			if r.Context().Err() != nil {
				w.WriteHeader(499)
				observeProxyTerminal(ctx, shared.OutcomeClientError, ctx.IsStream, time.Since(startedAt))
				return
			}
			lastSaturationReason = admission.denied.Error()
			rejectedDirectChannels[admission.id] = true
			if ctx.lifecycle != nil {
				ctx.lifecycle.mu.Lock()
				ctx.lifecycle.noRouteReason = lastSaturationReason
				ctx.lifecycle.mu.Unlock()
			}
			if repeatedSelection {
				break
			}
			continue
		}
		if r.Context().Err() != nil {
			w.WriteHeader(499)
			observeProxyTerminal(ctx, shared.OutcomeClientError, ctx.IsStream, time.Since(startedAt))
			return
		}
		attempt++
	}

	reason := "all channels exhausted"
	if pendingFailure != nil {
		reason = pendingFailure.reason()
	} else if isDirectAdmissionReason(lastSaturationReason) {
		reason = lastSaturationReason
	}
	if pendingFailure != nil {
		pendingFailure.recordFirstFailure(ctx)
		reportProxyAllFailedTerminal(ctx, cfg, reason)
		pendingFailure.write(w, resolveUpstreamDB(ctx, cfg), requestID, ctx.RequestedModel)
		observeProxyTerminal(ctx, pendingFailure.outcomeStatus(), ctx.IsStream, time.Since(startedAt))
		return
	}
	reportProxyAllFailedTerminal(ctx, cfg, reason)
	writeJSONErrorWithRequest(w, 503, "All channels exhausted", "server_error", requestID)
	observeProxyTerminal(ctx, shared.OutcomeUnavailable, ctx != nil && ctx.IsStream, time.Since(startedAt))
}

func resolveUpstreamDB(ctx *Ctx, cfg *UpstreamConfig) *store.DB {
	if ctx != nil && ctx.db != nil {
		return ctx.db
	}
	if cfg != nil {
		return cfg.DB
	}
	return nil
}

// reportProxyAllFailedTerminal reports a terminal all-channels-failed outcome
// so operators get an event + notification instead of a silent 5xx. Dedup is
// handled inside alert.ReportProxyAllFailed (per-model cooldown) so a dead
// upstream cannot spam on every request. No-op when the DB is not wired.
func reportProxyAllFailedTerminal(ctx *Ctx, cfg *UpstreamConfig, reason string) {
	model := ""
	if ctx != nil {
		model = ctx.RequestedModel
	}
	reportProxyAllFailed(resolveUpstreamDB(ctx, cfg), model, reason)
}

func reportProxyAllFailed(db *store.DB, model, reason string) {
	if db == nil {
		return
	}
	alert.ReportProxyAllFailed(config.RuntimeSafe(), db.DB, alert.ProxyAllFailedParams{Model: model, Reason: reason})
}

// relay decides whether an upstream reply is terminal or permits another attempt.
func (reply endpointReply) relay(
	w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig,
	selected *routing.SelectedChannel, upstreamModel, upstreamPath string,
	retry, maxRetries int, isLastEndpoint, disableCrossProtocolFallback bool, requestID string,
) (finished bool, nextPending *pendingUpstreamFailure, cont bool) {
	resp, startedAt := reply.response, reply.startedAt
	firstByteLatencyMs, aggregatedUsage := reply.firstByteLatencyMs, reply.aggregatedUsage
	if reply.effectiveStream {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, readErr := proxy.ReadBufferedResponseBody(resp.Body)
			resp.Body.Close()
			latencyMs := time.Since(startedAt).Milliseconds()
			if readErr != nil {
				slog.Warn("failed to read upstream stream error response",
					"err", readErr, "latency_ms", latencyMs, "status", resp.StatusCode,
					"request_id", requestID, "retry", retry)
				errText := readErr.Error()
				if shouldContinueEndpointFallback(http.StatusBadGateway, errText, isLastEndpoint, disableCrossProtocolFallback, classifyEndpointTransportFailure(readErr)) {
					writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, true, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, truncateErrText(errText))
					return false, nil, true
				}
				recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, http.StatusBadGateway, errText)
				writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, true, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, errText)
				if retry < maxRetries {
					return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Failed to read upstream response", "upstream_error"), false
				}
				writeJSONErrorWithRequest(w, 502, "Failed to read upstream response", "upstream_error", requestID)
				observeProxyTerminal(ctx, shared.OutcomeUpstreamError, true, time.Duration(latencyMs)*time.Millisecond)
				return true, nil, false
			}
			errBody := normalizeBufferedUpstreamBody(resp, respBody, upstreamBodyIdent{
				siteID:    selected.Site.ID,
				channelID: selected.Channel.ID,
				requestID: requestID,
				stream:    true,
			})
			respBody = errBody.bytes
			rawErrText := string(respBody)
			failUsage := errBody.parseUsage()
			executed := responseHasExecutionEvidence(respBody, failUsage, resp.StatusCode)
			if !executed && shouldContinueEndpointResponseFallback(selected, resp.StatusCode, rawErrText, isLastEndpoint, disableCrossProtocolFallback, errBody.readable) {
				writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, resp.StatusCode, true, failUsage, retry, requestID, truncateErrText(rawErrText))
				return false, nil, true
			}
			// Best-effort usage from error JSON bodies (some gateways still
			// include usage). Only from bytes we could actually read: an
			// undecodable body yields the explicit unknown source.
			recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, resp.StatusCode, rawErrText)
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, resp.StatusCode, true, failUsage, retry, requestID, truncateErrText(rawErrText))
			if !executed && retry < maxRetries && proxy.ShouldRetryProxyRequest(resp.StatusCode, rawErrText) {
				return false, bufferedPendingUpstreamFailure(resp, respBody), false
			}
			relayBufferedUpstreamErrorResponse(w, resp, respBody)
			observeProxyTerminal(ctx, shared.StatusFromHTTP(resp.StatusCode), true, time.Duration(latencyMs)*time.Millisecond)
			return true, nil, false
		}
		reply.relayStream(w, r, ctx, cfg, selected, upstreamModel, upstreamPath, retry, requestID)
		return true, nil, false
	}

	respBody, readErr := proxy.ReadBufferedResponseBody(resp.Body)
	resp.Body.Close()
	latencyMs := time.Since(startedAt).Milliseconds()
	if readErr != nil {
		if r.Context().Err() != nil {
			writeFailureProxyLog(context.WithoutCancel(r.Context()), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 499, false, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, "Client canceled request")
			w.WriteHeader(499)
			observeProxyTerminal(ctx, shared.OutcomeClientError, false, time.Duration(latencyMs)*time.Millisecond)
			return true, nil, false
		}
		slog.Warn("failed to read upstream response",
			"err", readErr, "latency_ms", latencyMs, "channel_id", selected.Channel.ID,
			"request_id", requestID, "retry", retry)
		errText := readErr.Error()
		rejected := resp.StatusCode < 200 || resp.StatusCode >= 300
		if rejected && shouldContinueEndpointFallback(http.StatusBadGateway, errText, isLastEndpoint, disableCrossProtocolFallback, classifyEndpointTransportFailure(readErr)) {
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, false, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, truncateErrText(errText))
			return false, nil, true
		}
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, http.StatusBadGateway, errText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, false, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, errText)
		if rejected && retry < maxRetries {
			return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Failed to read upstream response", "upstream_error"), false
		}
		writeJSONErrorWithRequest(w, 502, "Failed to read upstream response", "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Duration(latencyMs)*time.Millisecond)
		return true, nil, false
	}
	// Single encoding decision for this body: it fixes resp.Header (a decoded
	// body loses its Content-Encoding) and tells us whether the bytes may be
	// parsed at all. Everything below — usage, the content judge and the relay —
	// consumes that one answer.
	body := normalizeBufferedUpstreamBody(resp, respBody, upstreamBodyIdent{
		siteID:    selected.Site.ID,
		channelID: selected.Channel.ID,
		requestID: requestID,
	})
	respBody = body.bytes
	if body.readable {
		normalizeUpstreamJSONContentType(resp.Header, respBody, r.URL.Path)
	}
	if wire := directProviderWireFromContext(r.Context()); wire != nil && wire.Driver != nil && body.readable && aggregatedUsage == nil {
		observed, _, observationErr := observeDirectDriverBody(wire, respBody, false)
		body.observedUsage = &observed
		if observationErr == nil {
			if err := captureDirectMediaTaskUsage(r.Context(), ctx, selected, wire, observed); err != nil {
				writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 503, false, observed, retry, requestID, err.Error())
				writeJSONErrorWithRequest(w, 503, err.Error(), "upstream_error", requestID)
				return true, nil, false
			}
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var decoded []byte
			var decodeErr error
			if observationErr == nil {
				decoded, _, decodeErr = decodeDirectDriverPayload(r.Context(), wire, respBody, false, resp.StatusCode)
			}
			if observationErr != nil || decodeErr != nil {
				if observationErr == nil {
					if err := retainFailedDirectMediaSubmission(r.Context(), ctx, selected, wire, observed); err != nil {
						writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 503, false, observed, retry, requestID, err.Error())
						writeJSONErrorWithRequest(w, 503, err.Error(), "upstream_error", requestID)
						return true, nil, false
					}
				}
				reason := "Cannot decode selected provider response"
				writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, false, observed, retry, requestID, reason)
				writeJSONErrorWithRequest(w, http.StatusBadGateway, reason, "upstream_error", requestID)
				observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Since(startedAt))
				return true, nil, false
			}
			if !bytes.Equal(decoded, respBody) {
				respBody, body.bytes = decoded, decoded
				resp.Header.Del("Content-Length")
				resp.Header.Del("ETag")
				resp.Header.Del("Content-MD5")
				resp.Header.Del("Digest")
				resp.Header.Set("Content-Type", "application/json")
			}
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawErrText := string(respBody)
		failUsage := body.parseUsage()
		executed := responseHasExecutionEvidence(respBody, failUsage, resp.StatusCode)
		if !executed && shouldContinueEndpointResponseFallback(selected, resp.StatusCode, rawErrText, isLastEndpoint, disableCrossProtocolFallback, body.readable) {
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, resp.StatusCode, false, failUsage, retry, requestID, truncateErrText(rawErrText))
			return false, nil, true
		}
		// Non-stream HTTP errors: retain any usage object in the error body
		// (measurable under-count after disconnect partial).
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, resp.StatusCode, rawErrText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, resp.StatusCode, false, failUsage, retry, requestID, truncateErrText(rawErrText))
		if !executed && retry < maxRetries && proxy.ShouldRetryProxyRequest(resp.StatusCode, rawErrText) {
			return false, bufferedPendingUpstreamFailure(resp, respBody), false
		}
		relayBufferedUpstreamResponse(w, resp, respBody)
		observeProxyTerminal(ctx, shared.StatusFromHTTP(resp.StatusCode), false, time.Duration(latencyMs)*time.Millisecond)
		return true, nil, false
	}
	nativeHasOutput, convertErr := normalizeEndpointJSONResponse(resp, &body, upstreamPath, directProviderWireFromContext(r.Context()))
	if convertErr != nil {
		reason := "Cannot decode upstream native JSON response"
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, http.StatusBadGateway, reason)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, false, body.parseUsage(), retry, requestID, reason)
		writeJSONErrorWithRequest(w, http.StatusBadGateway, reason, "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Since(startedAt))
		return true, nil, false
	}
	respBody = body.bytes
	usage := body.parseUsage()
	if aggregatedUsage != nil {
		usage = *aggregatedUsage
	}
	if wire := directProviderWireFromContext(r.Context()); wire != nil && (len(wire.Plugins) > 0 || wire.Driver != nil) && body.readable {
		var pluginErr error
		respBody, pluginErr = applyDirectChannelResponsePlugins(r.Context(), wire, respBody, "response")
		if pluginErr != nil {
			reason := "Cannot execute selected channel response plugin"
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 502, false, usage, retry, requestID, reason)
			writeJSONErrorWithRequest(w, 502, reason, "upstream_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Since(startedAt))
			return true, nil, false
		}
		resp.Header.Del("Content-Length")
		resp.Header.Del("ETag")
		resp.Header.Del("Content-MD5")
		resp.Header.Del("Digest")
	}
	videoBody, videoErr := respBody, error(nil)
	if selected.Direct != nil {
		videoBody, videoErr = processVideoTaskResponse(ctx, selected, r.Method, upstreamPath, respBody)
	}
	if videoErr != nil {
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 502, false, usage, retry, requestID, videoErr.Error())
		writeJSONErrorWithRequest(w, 502, videoErr.Error(), "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Since(startedAt))
		return true, nil, false
	}
	if !bytes.Equal(videoBody, respBody) {
		resp.Header.Del("Content-Length")
		resp.Header.Del("ETag")
		resp.Header.Del("Content-MD5")
		resp.Header.Del("Digest")
		respBody = videoBody
	}
	usage, videoErr = accountDirectVideoUsage(r.Context(), ctx, selected, usage)
	if videoErr != nil {
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 503, false, usage, retry, requestID, videoErr.Error())
		writeJSONErrorWithRequest(w, 503, videoErr.Error(), "upstream_error", requestID)
		return true, nil, false
	}
	if r.Method == http.MethodDelete && ctx.videoTask != nil && ctx.videoUsageVerified {
		db := resolveUpstreamDB(ctx, cfg)
		if db == nil {
			writeJSONErrorWithRequest(w, 503, "video task deletion could not be recorded", "upstream_error", requestID)
			return true, nil, false
		}
		if _, err := db.ExecContext(context.WithoutCancel(r.Context()), `DELETE FROM proxy_video_tasks WHERE public_id=?`, ctx.videoTask.PublicID); err != nil {
			writeJSONErrorWithRequest(w, 503, "video task deletion could not be recorded", "upstream_error", requestID)
			return true, nil, false
		}
	}
	// Same single judge the streaming path calls (see judgeStreamContent), fed
	// with the buffered facts this path already has. When the body could not be
	// decoded the facts say so explicitly and the judge declines to rule.
	facts := body.judgeFacts(resp.StatusCode, usage)
	facts.HasOutput = nativeHasOutput
	verdict := proxy.JudgeUpstreamContent(facts)
	if verdict.Failed && !ctx.videoUsageVerified {
		slog.Warn("content-based failure detected",
			"reason", verdict.Reason,
			"status", verdict.Status,
			"model", upstreamModel,
			"channel_id", selected.Channel.ID,
			"latency_ms", latencyMs,
			"request_id", requestID,
			"retry", retry,
		)
		executed := responseHasExecutionEvidence(respBody, usage, resp.StatusCode) || aggregatedUsage != nil
		if !executed && shouldContinueEndpointFallback(verdict.Status, verdict.Reason, isLastEndpoint, disableCrossProtocolFallback, endpointFailureResponse) {
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, verdict.Status, false, usage, retry, requestID, truncateErrText(verdict.Reason))
			return false, nil, true
		}
		// Content failures often still carry real usage (keyword match / empty-
		// content edge cases with non-zero tokens). Persist failed row + tokens.
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, verdict.Status, verdict.Reason)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, verdict.Status, false, usage, retry, requestID, verdict.Reason)
		if !executed && retry < maxRetries && proxy.ShouldRetryProxyRequest(verdict.Status, verdict.Reason) {
			return false, jsonPendingUpstreamFailure(verdict.Status, "Upstream returned an error response", "upstream_error"), false
		}
		writeJSONErrorWithRequest(w, verdict.Status, "Upstream returned an error response", "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.StatusFromHTTP(verdict.Status), false, time.Duration(latencyMs)*time.Millisecond)
		return true, nil, false
	}
	respBody, convertErr = projectEndpointResponse(r, resp, respBody, body.readable, selected.Direct != nil, ctx.DownstreamPath, upstreamPath, upstreamModel, reply.bridgeOptions)
	if convertErr != nil {
		// The upstream did run: retain its measured usage, but never record an
		// unusable downstream representation as a successful tool response.
		errText := "Cannot convert upstream response to the client protocol"
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, http.StatusBadGateway, errText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, false, usage, retry, requestID, errText)
		writeJSONErrorWithRequest(w, http.StatusBadGateway, errText, "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Since(startedAt))
		return true, nil, false
	}
	if selected.Direct == nil {
		managedBody, managedErr := processManagedProxyResponse(ctx, selected, r.Method, upstreamPath, respBody)
		if managedErr != nil {
			// The upstream call succeeded but its body cannot be relayed without
			// leaking or misattributing another key's objects. Relay the failure
			// instead of the unsafe body.
			slog.Warn("cannot process shared-upstream response safely",
				"err", managedErr, "path", upstreamPath,
				"channel_id", selected.Channel.ID, "request_id", requestID)
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, false, usage, retry, requestID, truncateErrText(managedErr.Error()))
			writeJSONErrorWithRequest(w, http.StatusBadGateway, managedErr.Error(), "upstream_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Since(startedAt))
			return true, nil, false
		}
		if !bytes.Equal(managedBody, respBody) {
			for _, key := range []string{"Content-Length", "ETag", "Content-MD5", "Digest"} {
				resp.Header.Del(key)
			}
			respBody = managedBody
		}
	}
	recordUpstreamSuccess(r.Context(), cfg, selected, ctx.RequestedModel, upstreamModel, latencyMs, usage)
	writeSuccessProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, resp.StatusCode, false, usage, retry, requestID)
	if body.readable {
		respBody = normalizeNativeTerminalResponse(resp, respBody, r.URL.Path)
		respBody = restoreBufferedDownstreamResponseModel(resp, respBody, downstreamResponseModel(r))
	}
	relayBufferedUpstreamResponse(w, resp, respBody)
	observeProxyTerminal(ctx, shared.OutcomeSuccess, false, time.Duration(latencyMs)*time.Millisecond)
	return true, nil, false
}

// Retryable status codes and protocol hints must not replay observed work.
// Only failed responses take this path; normal successes need no second parse.
func responseHasExecutionEvidence(body []byte, usage ParsedUsage, status int) bool {
	retryableError := status == http.StatusTooManyRequests || status >= 500 && status < 600
	if usage.TotalTokens > 0 || usage.invalidForUserBudget && !retryableError {
		return true
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	return decoder.Decode(&payload) == nil && directResponseHasActivity(payload, retryableError, true)
}

func selectedHealthAccountID(selected *routing.SelectedChannel) *int64 {
	if selected.Direct != nil || selected.Account.ID <= 0 {
		return nil
	}
	return &selected.Account.ID
}
