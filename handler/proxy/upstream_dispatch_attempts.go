package proxyhandler

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
)

func dispatchChannelAttempts(
	w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig,
	upstreamPath string, maxRetries int, requestID string, startedAt time.Time,
) {
	policy := ctx.dispatchPolicy
	excludeChannelIDs := make([]int64, 0)
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
				DownstreamPolicy:  *policy,
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
				r.Context(), cfg.Router, ctx.RequestedModel, excludeChannelIDs, *policy)
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
			selected, status, err = revalidateQueuedChannel(r, ctx, cfg, selected, policy)
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
