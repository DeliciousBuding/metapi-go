package proxyhandler

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sync"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

type queuedChannelRevalidator interface {
	RevalidateQueuedChannel(context.Context, *routing.SelectedChannel, string, routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error)
}

func revalidateQueuedChannel(r *http.Request, ctx *Ctx, cfg *UpstreamConfig, selected *routing.SelectedChannel, policy *routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, int, error) {
	if ctx.Auth != nil && ctx.Auth.KeyID != nil {
		current := auth.RevalidateQueuedRequest(r, ctx.Auth)
		if !current.OK {
			return nil, current.StatusCode, fmt.Errorf("Queued request authorization is no longer valid")
		}
		nextAuth := *ctx.Auth
		nextAuth.Policy = current.Policy
		nextAuth.ProxyURL = current.Key.ProxyURL
		ctx.Auth, ctx.Policy = &nextAuth, current.Policy
		refreshed := RoutingPolicyFromAuth(current.Policy)
		refreshed.RequestedContextTokens = policy.RequestedContextTokens
		refreshed.RequiredUpstreamProtocol = policy.RequiredUpstreamProtocol
		refreshed.AllowUpstreamProtocolConversion = policy.AllowUpstreamProtocolConversion
		refreshed.AssociationRequest = policy.AssociationRequest
		refreshed.ResolveAssociationRequest = policy.ResolveAssociationRequest
		refreshed.RequestStream = policy.RequestStream
		refreshed.AllowStreamAggregation = policy.AllowStreamAggregation
		refreshed.TraceIdentity = policy.TraceIdentity
		*policy = refreshed
	}
	revalidator, ok := cfg.Router.(queuedChannelRevalidator)
	if !ok {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("Queued channel cannot be revalidated")
	}
	fresh, err := revalidator.RevalidateQueuedChannel(r.Context(), selected, ctx.RequestedModel, *policy)
	if err != nil || fresh == nil {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("Queued channel authorization or configuration changed")
	}
	return fresh, 0, nil
}

// Token exchange can outlive a grant/configuration change even without queue
// waiting. Reuse the same eligibility owner without another admission or route
// choice; a changed wire configuration cannot use the old channel permit.
func revalidateDirectCredentialDispatch(r *http.Request, ctx *Ctx, cfg *UpstreamConfig, selected *routing.SelectedChannel, policy *routing.DownstreamRoutingPolicy) (int, error) {
	if selected.Direct.CredentialKind != store.DirectCredentialOAuth && selected.Direct.CredentialKind != store.DirectCredentialGCPServiceAccount {
		return 0, nil
	}
	return revalidateDirectOperationDispatch(r, ctx, cfg, selected, policy)
}

// Continuation I/O checks the same selected authority without another route
// choice or admission. Only OAuth/GCP token rotation may change its material.
func revalidateDirectOperationDispatch(r *http.Request, ctx *Ctx, cfg *UpstreamConfig, selected *routing.SelectedChannel, policy *routing.DownstreamRoutingPolicy) (int, error) {
	if policy == nil {
		return http.StatusServiceUnavailable, fmt.Errorf("Selected channel cannot be revalidated after credential exchange")
	}
	var proxyBefore *string
	if ctx.Auth != nil {
		proxyBefore = ctx.Auth.ProxyURL
	}
	fresh, status, err := revalidateQueuedChannel(r, ctx, cfg, selected, policy)
	if err != nil {
		return status, err
	}
	if fresh.Direct == nil {
		return http.StatusServiceUnavailable, fmt.Errorf("Selected channel changed during credential exchange")
	}
	before, after := *selected.Direct, *fresh.Direct
	// Refresh intentionally changes only the token cache. Presentation labels
	// do not affect the authority or request already prepared for this channel.
	if selected.Direct.CredentialKind == store.DirectCredentialOAuth || selected.Direct.CredentialKind == store.DirectCredentialGCPServiceAccount {
		before.Credential, after.Credential = "", ""
	}
	before.ChannelName, after.ChannelName = "", ""
	before.CredentialName, after.CredentialName = "", ""
	var proxyAfter *string
	if ctx.Auth != nil {
		proxyAfter = ctx.Auth.ProxyURL
	}
	if fresh.ActualModel != selected.ActualModel || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(proxyBefore, proxyAfter) {
		return http.StatusServiceUnavailable, fmt.Errorf("Selected channel changed during credential exchange")
	}
	return 0, nil
}

func isDirectAdmissionReason(reason string) bool {
	return reason == "direct_account_concurrency_saturated" || reason == channellimit.ErrQueueFull.Error() ||
		reason == channellimit.ErrQueueTimeout.Error() || reason == channellimit.ErrRPMExhausted.Error()
}

type channelAdmissionKey struct{}
type channelUsageKey struct{}

type channelAdmission struct {
	manager *channellimit.Manager
	id      int64
	config  channellimit.Config
	denied  error
}

type channelUsage struct {
	once      sync.Once
	admission *channelAdmission
}

func channelLimits(cfg *UpstreamConfig) *channellimit.Manager {
	if cfg != nil && cfg.ChannelLimits != nil {
		return cfg.ChannelLimits
	}
	return channellimit.Default
}

func acquireChannelAdmission(ctx context.Context, cfg *UpstreamConfig, selected *routing.SelectedChannel) (*channellimit.Permit, *channelAdmission, error) {
	if selected.Direct == nil {
		return nil, nil, nil
	}
	direct := selected.Direct
	admission := &channelAdmission{
		manager: channelLimits(cfg), id: direct.ChannelID, config: direct.RateLimit.Config,
	}
	permit, err := admission.manager.Acquire(ctx, admission.id, admission.config)
	return permit, admission, err
}

// prepareChannelRateAttempt runs after body/credential validation and before
// beginning an observed upstream attempt. Rejected requests never get a usage
// record or provider failure. Endpoint fallbacks receive their own RPM slot.
func prepareChannelRateAttempt(ctx context.Context) (context.Context, error) {
	admission, _ := ctx.Value(channelAdmissionKey{}).(*channelAdmission)
	if admission == nil || !admission.config.Enabled() {
		return ctx, nil
	}
	if err := admission.manager.TryRequest(ctx, admission.id, admission.config); err != nil {
		admission.denied = err
		return ctx, err
	}
	return context.WithValue(ctx, channelUsageKey{}, &channelUsage{admission: admission}), nil
}

func recordChannelRateUsage(ctx context.Context, usage ParsedUsage) {
	record, _ := ctx.Value(channelUsageKey{}).(*channelUsage)
	if record == nil {
		return
	}
	record.once.Do(func() {
		record.admission.manager.RecordUsage(record.admission.id, usage.TotalTokens, usage.Found)
	})
}
