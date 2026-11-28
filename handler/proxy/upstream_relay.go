package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
)

// relay decides whether an upstream reply is terminal or permits another attempt.
func (reply endpointReply) relay(
	w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig,
	selected *routing.SelectedChannel, upstreamModel, upstreamPath string,
	retry, maxRetries int, isLastEndpoint, disableCrossProtocolFallback bool, requestID string,
) (finished bool, nextPending *pendingUpstreamFailure, cont bool) {
	resp, startedAt := reply.response, reply.startedAt
	firstByteLatencyMs, aggregatedUsage := reply.firstByteLatencyMs, reply.aggregatedUsage
	stream := reply.effectiveStream
	rejected := resp.StatusCode < 200 || resp.StatusCode >= 300
	if stream && !rejected {
		reply.relayStream(w, r, ctx, cfg, selected, upstreamModel, upstreamPath, retry, requestID)
		return true, nil, false
	}

	respBody, readErr := proxy.ReadBufferedResponseBody(resp.Body)
	resp.Body.Close()
	latencyMs := time.Since(startedAt).Milliseconds()
	if readErr != nil {
		if !stream && r.Context().Err() != nil {
			writeFailureProxyLog(context.WithoutCancel(r.Context()), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 499, false, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, "Client canceled request")
			w.WriteHeader(499)
			observeProxyTerminal(ctx, shared.OutcomeClientError, false, time.Duration(latencyMs)*time.Millisecond)
			return true, nil, false
		}
		if stream {
			slog.Warn("failed to read upstream stream error response",
				"err", readErr, "latency_ms", latencyMs, "status", resp.StatusCode,
				"request_id", requestID, "retry", retry)
		} else {
			slog.Warn("failed to read upstream response",
				"err", readErr, "latency_ms", latencyMs, "channel_id", selected.Channel.ID,
				"request_id", requestID, "retry", retry)
		}
		errText := readErr.Error()
		if rejected && shouldContinueEndpointFallback(http.StatusBadGateway, errText, isLastEndpoint, disableCrossProtocolFallback, classifyEndpointTransportFailure(readErr)) {
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, stream, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, truncateErrText(errText))
			return false, nil, true
		}
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, http.StatusBadGateway, errText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, stream, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, errText)
		if rejected && retry < maxRetries {
			return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Failed to read upstream response", "upstream_error"), false
		}
		writeJSONErrorWithRequest(w, 502, "Failed to read upstream response", "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, stream, time.Duration(latencyMs)*time.Millisecond)
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
		stream:    stream,
	})
	respBody = body.bytes
	if !stream {
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
	}
	if rejected {
		rawErrText := string(respBody)
		failUsage := body.parseUsage()
		executed := responseHasExecutionEvidence(respBody, failUsage, resp.StatusCode)
		if !executed && shouldContinueEndpointResponseFallback(selected, resp.StatusCode, rawErrText, isLastEndpoint, disableCrossProtocolFallback, body.readable) {
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, resp.StatusCode, stream, failUsage, retry, requestID, truncateErrText(rawErrText))
			return false, nil, true
		}
		// Retain any usage object in the error body
		// (measurable under-count after disconnect partial).
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, resp.StatusCode, rawErrText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, resp.StatusCode, stream, failUsage, retry, requestID, truncateErrText(rawErrText))
		if !executed && retry < maxRetries && proxy.ShouldRetryProxyRequest(resp.StatusCode, rawErrText) {
			return false, bufferedPendingUpstreamFailure(resp, respBody), false
		}
		if stream {
			relayBufferedUpstreamErrorResponse(w, resp, respBody)
		} else {
			relayBufferedUpstreamResponse(w, resp, respBody)
		}
		observeProxyTerminal(ctx, shared.StatusFromHTTP(resp.StatusCode), stream, time.Duration(latencyMs)*time.Millisecond)
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
