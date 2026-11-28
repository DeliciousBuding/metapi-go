package backup

import (
	"fmt"
	"sort"

	"github.com/deliciousbuding/metapi-go/store"
)

// axonHubPlan is the single compiled result shared by the preview and the
// transaction executor. Both paths render exactly what this produced, so a
// preview can never claim a channel the import will not write.
type axonHubPlan struct {
	keys        []axonHubPlanAPIKey
	channels    []axonHubPlanChannel
	credentials []axonHubPlanCredential
	models      []axonHubPlanModel
	grants      []axonHubPlanGrant
	routes      []axonHubPlanRoute

	notImported map[string]int
	skipped     []AxonHubSkippedChannel
	residuals   []string
	// blocking holds reasons this backup cannot be imported at all. Skip
	// reasons stay per channel so a partial import is still reviewable.
	blocking []string
}

type axonHubPlanCredential struct {
	ChannelSourceID int
	Name            string
	Secret          string
	Kind            string
	OAuthState      store.DirectOAuthState
	// GCPMaterial is the service-account JSON for gcp_service_account
	// credentials; Secret stays empty until the first token exchange.
	GCPMaterial string
	Enabled     bool
}

type axonHubPlanModel struct {
	ChannelSourceID int
	Name            string
	Enabled         bool
}

type axonHubPlanGrant struct {
	ChannelSourceID int
	ModelName       string
	CredentialName  string
	Protocols       int
	Enabled         bool
}

// CompileAxonHubPlan turns a parsed backup into the exact channel graph the
// executor will persist. It never guesses: a feature it cannot reproduce
// excludes the channel that carries it and says why.
func CompileAxonHubPlan(src *AxonHubSource) (*axonHubPlan, error) {
	if src == nil {
		return nil, axonHubErr("AxonHub source is missing")
	}
	plan := &axonHubPlan{
		notImported: map[string]int{},
	}
	channels := make([]*axonHubPlanChannel, 0, len(src.Channels))
	bySourceID := make(map[int]*axonHubPlanChannel, len(src.Channels))
	usedChannelNames := map[string]int{}

	for _, channel := range src.Channels {
		if channel.DeletedAt != 0 {
			plan.notImported["deletedChannels"]++
			continue
		}
		compiled, reasons, protocolResiduals := compileAxonHubChannel(channel)
		plan.residuals = append(plan.residuals, protocolResiduals...)
		if len(reasons) > 0 {
			plan.skipped = append(plan.skipped, AxonHubSkippedChannel{
				SourceID: channel.ID,
				Type:     sanitizeIdentifier(channel.Type),
				Reasons:  reasons,
			})
			continue
		}
		compiled.Name = uniqueChannelName(compiled.Name, channel.ID, usedChannelNames)
		channels = append(channels, compiled)
		bySourceID[channel.ID] = compiled
	}
	if len(channels) == 0 {
		plan.blocking = append(plan.blocking, "no_servable_channels")
	}

	plan.channels = derefChannels(channels)
	for _, channel := range plan.channels {
		if channel.GCPMaterial != "" {
			plan.credentials = append(plan.credentials, axonHubPlanCredential{
				ChannelSourceID: channel.SourceID,
				Name:            credentialName(0, 1),
				Kind:            store.DirectCredentialGCPServiceAccount,
				GCPMaterial:     channel.GCPMaterial,
				Enabled:         true,
			})
			continue
		}
		for i, secret := range channel.CredentialKeys {
			kind := store.DirectCredentialAPIKey
			if secret == "" {
				kind = store.DirectCredentialNone
			}
			state := store.DirectOAuthState{}
			if channel.OAuthState != nil {
				kind = store.DirectCredentialOAuth
				state = *channel.OAuthState
			}
			plan.credentials = append(plan.credentials, axonHubPlanCredential{
				ChannelSourceID: channel.SourceID,
				Name:            credentialName(i, len(channel.CredentialKeys)),
				Secret:          secret,
				Kind:            kind,
				OAuthState:      state,
				Enabled:         !channel.DisabledKeys[secret],
			})
		}
	}

	routes, residuals := compileAxonHubRoutes(src, channels, bySourceID)
	plan.routes = routes
	plan.residuals = append(plan.residuals, residuals...)
	sort.Strings(plan.residuals)

	usedModels := map[string]bool{}
	// A grant is one (channel, model, credential) triple: upstream_grants is
	// unique on (model_id, credential_id), so two models on one channel each
	// need their own row even when they share a key.
	usedGrants := map[string]bool{}
	for _, route := range plan.routes {
		if !route.Enabled {
			plan.notImported["disabledModelRoutes"]++
		}
		for _, item := range route.Items {
			modelKey := fmt.Sprintf("%d\x00%s", item.ChannelSourceID, item.ModelName)
			if !usedModels[modelKey] {
				usedModels[modelKey] = true
				channel := bySourceID[item.ChannelSourceID]
				plan.models = append(plan.models, axonHubPlanModel{
					ChannelSourceID: item.ChannelSourceID,
					Name:            item.ModelName,
					Enabled:         channel.Enabled,
				})
			}
			grantKey := fmt.Sprintf("%d\x00%s\x00%s", item.ChannelSourceID, item.ModelName, item.CredentialName)
			if usedGrants[grantKey] {
				continue
			}
			usedGrants[grantKey] = true
			channel := bySourceID[item.ChannelSourceID]
			plan.grants = append(plan.grants, axonHubPlanGrant{
				ChannelSourceID: item.ChannelSourceID,
				ModelName:       item.ModelName,
				CredentialName:  item.CredentialName,
				Protocols:       channel.Protocols,
				Enabled:         true,
			})
		}
	}

	if err := compileAxonHubAccess(src, plan); err != nil {
		return nil, err
	}
	plan.notImported["modelPrices"] = len(src.ChannelModelPrices)
	plan.notImported["systemConfigs"] = len(src.SystemConfigs)
	plan.notImported["usageRequests"] = src.UsageRequests
	plan.notImported["usageLogs"] = src.UsageLogs
	for section, count := range src.UnknownSections {
		plan.notImported["section:"+section] += count
	}
	dropZeroCounts(plan.notImported)
	return plan, nil
}

func derefChannels(channels []*axonHubPlanChannel) []axonHubPlanChannel {
	out := make([]axonHubPlanChannel, 0, len(channels))
	for _, channel := range channels {
		out = append(out, *channel)
	}
	return out
}

func credentialName(index, total int) string {
	if total <= 1 {
		return "default"
	}
	return fmt.Sprintf("key-%d", index+1)
}
