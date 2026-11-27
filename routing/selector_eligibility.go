package routing

import (
	"github.com/deliciousbuding/metapi-go/store"
)

func (s *ChannelSelector) eligibleChannels(match *RouteMatch, requestedModel string, byDisplayName bool, excludeChannelIDs []int64, nowISO string, policy DownstreamRoutingPolicy) []RouteChannelCandidate {
	var available []RouteChannelCandidate
	for _, candidate := range match.Channels {
		candidate, blocker := resolveAssociationCandidate(candidate, policy)
		if blocker.Code != "" {
			continue
		}
		if len(s.getCandidateBlockers(candidate, requestedModel, byDisplayName, excludeChannelIDs, nowISO, policy)) == 0 {
			available = append(available, candidate)
		}
	}
	return filterStreamPolicyCandidates(available, policy)
}

// The caller resolves the request-specific association before checking the grant.
func (s *ChannelSelector) getCandidateBlockers(
	candidate RouteChannelCandidate,
	requestedModel string,
	bypassSourceModelCheck bool,
	excludeChannelIDs []int64,
	nowISO string,
	policy DownstreamRoutingPolicy,
) []CandidateBlocker {
	reasons := make([]CandidateBlocker, 0)
	if blocker := streamPolicyBlocker(candidate, policy); blocker.Code != "" {
		reasons = append(reasons, blocker)
	}
	if direct := candidate.Direct; direct != nil {
		if IsCooldownActive(candidate.Channel.CooldownUntil, nowISO) {
			reasons = append(reasons, CandidateBlocker{"cooldown", "direct upstream grant cooling down"})
		}
		if !candidate.Channel.Enabled || !direct.ChannelEnabled || !direct.ModelEnabled || !direct.CredentialEnabled || !direct.GrantEnabled {
			reasons = append(reasons, CandidateBlocker{"disabled", "direct upstream grant disabled"})
		}
		protocols := direct.Protocols
		if direct.CredentialKind == store.DirectCredentialNone {
			protocols &= direct.Endpoints.AnonymousProtocolMask()
			if !store.DirectProviderAllowsAnonymous(direct.Provider) {
				protocols = 0
			}
		}
		if len(direct.ProtocolOrder) > 0 {
			allowed := 0
			for _, protocol := range direct.ProtocolOrder {
				allowed |= protocol
			}
			protocols &= allowed
		}
		required := policy.RequiredUpstreamProtocol
		if required&store.DirectGenerationProtocols != 0 && policy.AllowUpstreamProtocolConversion && direct.Endpoints.IsConfigured() {
			required = store.DirectGenerationProtocols
		}
		if direct.Endpoints.IsConfigured() {
			protocols &= direct.Endpoints.ProtocolMask()
		}
		if required != 0 && required&store.DirectGenerationProtocols == 0 && direct.Endpoints.ProtocolMask()&required == 0 {
			protocols = 0
		}
		if required == 0 || protocols&required == 0 {
			code := "protocol_not_authorized"
			if required == 0 {
				code = "protocol_context_missing"
			}
			reasons = append(reasons, CandidateBlocker{code, "direct upstream protocol not authorized"})
		}
		if len(policy.AllowedSiteIDs) > 0 {
			reasons = append(reasons, CandidateBlocker{"site_not_allowed", "direct upstream is not in the downstream key site allow-list"})
		}
		if policy.UserRouteIDs == nil && len(policy.AllowedRouteIDs) > 0 && !matchesSupportedModelPolicy(requestedModel, policy.SupportedModels) {
			allowed := false
			for _, routeID := range policy.AllowedRouteIDs {
				if routeID == direct.RouteID {
					allowed = true
					break
				}
			}
			if !allowed {
				reasons = append(reasons, CandidateBlocker{"route_not_allowed", "direct upstream route is not allowed by downstream key"})
			}
		}
		if reason := s.resolveDownstreamExclusionBlocker(candidate, policy); reason.Code != "" {
			reasons = append(reasons, reason)
		}
		for _, id := range excludeChannelIDs {
			if id == candidate.Channel.ID {
				reasons = append(reasons, CandidateBlocker{"already_tried", "direct upstream grant already tried"})
				break
			}
		}
		return reasons
	}

	// Shared accounts and OAuth pools do not have a typed Decisions endpoint or
	// grant. The -1 model-list sentinel remains a discovery query, not a dial.
	if policy.RequiredUpstreamProtocol == store.DirectProtocolDecisions {
		reasons = append(reasons, CandidateBlocker{"protocol_not_authorized", "Decisions requires an explicit direct endpoint and grant"})
	}

	if !bypassSourceModelCheck && !ChannelSupportsRequestedModelWithRedirects(candidate.Channel.SourceModel, requestedModel, candidate.Account.ID) {
		srcModel := ""
		if candidate.Channel.SourceModel != nil {
			srcModel = *candidate.Channel.SourceModel
		}
		reasons = append(reasons, CandidateBlocker{"source_model_mismatch", "来源模型不匹配=" + srcModel})
	}

	if !candidate.Channel.Enabled {
		reasons = append(reasons, CandidateBlocker{"disabled", "通道禁用"})
	}

	// OAuth route unit: check member availability
	if IsOAuthRouteUnitCandidate(candidate) {
		candidate = s.filterRouteUnitMembersByPolicy(candidate, policy)
		eligibleMembers := getEligibleRouteUnitMembers(candidate, requestedModel, nowISO)
		if len(eligibleMembers) == 0 {
			name := "round_robin"
			if candidate.RouteUnit != nil {
				name = candidate.RouteUnit.Name
			}
			reasons = append(reasons, CandidateBlocker{"oauth_members_unavailable", "路由池成员不可用（" + name + "）"})
		}
		return reasons
	}

	// Account status
	if IsExplicitTokenChannel(candidate) {
		if candidate.Account.Status == "disabled" {
			reasons = append(reasons, CandidateBlocker{"account_inactive", "账号状态=" + candidate.Account.Status})
		}
	} else if candidate.Account.Status != "active" {
		reasons = append(reasons, CandidateBlocker{"account_inactive", "账号状态=" + candidate.Account.Status})
	}

	// Site status
	if candidate.Site.Status == "disabled" {
		reasons = append(reasons, CandidateBlocker{"site_disabled", "站点状态=" + candidate.Site.Status})
	}

	// Downstream exclusion
	if reason := s.resolveDownstreamExclusionBlocker(candidate, policy); reason.Code != "" {
		reasons = append(reasons, reason)
	}

	// Already tried
	for _, id := range excludeChannelIDs {
		if id == candidate.Channel.ID {
			reasons = append(reasons, CandidateBlocker{"already_tried", "当前请求已尝试"})
			break
		}
	}

	// Token available
	tokenValue := s.resolveChannelTokenValue(candidate)
	if tokenValue == "" {
		reasons = append(reasons, CandidateBlocker{"token_unavailable", "令牌不可用"})
	}

	// Cooldown — parse timestamps (millis-aware); do not lex-compare ISO strings.
	if IsCooldownActive(candidate.Channel.CooldownUntil, nowISO) {
		reasons = append(reasons, CandidateBlocker{"cooldown", "冷却中"})
	}

	return reasons
}

func matchesSupportedModelPolicy(model string, patterns []string) bool {
	for _, pattern := range patterns {
		if MatchesModelPattern(model, pattern) {
			return true
		}
	}
	return false
}

func (s *ChannelSelector) resolveChannelTokenValue(candidate RouteChannelCandidate) string {
	if IsOAuthRouteUnitCandidate(candidate) {
		return ResolveRouteUnitMemberTokenValue(candidate.Account)
	}
	if candidate.Direct != nil {
		if !candidate.Direct.CredentialEnabled {
			return ""
		}
		return candidate.Direct.Credential
	}
	if candidate.Channel.TokenID != nil && *candidate.Channel.TokenID > 0 {
		if candidate.Token == nil {
			return ""
		}
		// Check usable
		if candidate.Token.Token == "" || !candidate.Token.Enabled {
			return ""
		}
		return candidate.Token.Token
	}

	// OAuth account: use accessToken
	if candidate.Account.OAuthProvider != nil && *candidate.Account.OAuthProvider != "" {
		if candidate.Account.AccessToken != "" {
			return candidate.Account.AccessToken
		}
		return ""
	}

	// Fallback: apiToken
	if candidate.Account.APIToken != nil && *candidate.Account.APIToken != "" {
		return *candidate.Account.APIToken
	}

	return ""
}

func (s *ChannelSelector) resolveDownstreamExclusionReason(candidate RouteChannelCandidate, policy DownstreamRoutingPolicy) string {
	return s.resolveDownstreamExclusionBlocker(candidate, policy).Message
}

func (s *ChannelSelector) resolveDownstreamExclusionBlocker(candidate RouteChannelCandidate, policy DownstreamRoutingPolicy) CandidateBlocker {
	channelID := int64(0)
	if candidate.Direct != nil {
		channelID = candidate.Direct.ChannelID
	}
	if !policy.AccessPolicy.AllowsUpstreamChannel(channelID) {
		return CandidateBlocker{"channel_not_allowed", "upstream channel is not allowed by downstream access policy"}
	}

	if candidate.Direct != nil {
		if len(policy.AllowedSiteIDs) > 0 {
			return CandidateBlocker{"site_not_allowed", "direct upstream is not in the downstream key site allow-list"}
		}
		if len(policy.ExcludedCredentialRefs) > 0 {
			for _, ref := range policy.ExcludedCredentialRefs {
				if credentialRefMatchesOne(candidate, ref, s) {
					return CandidateBlocker{"credential_excluded", "direct upstream credential excluded by downstream key"}
				}
			}
		}
		if len(policy.AllowedCredentialRefs) > 0 && !credentialRefMatchesCandidate(candidate, policy.AllowedCredentialRefs, s) {
			return CandidateBlocker{"credential_not_allowed", "direct upstream credential is not in downstream key allow-list"}
		}
		return CandidateBlocker{}
	}
	// allow-list: when AllowedSiteIDs is non-empty, only listed sites are eligible.
	if len(policy.AllowedSiteIDs) > 0 {
		allowed := false
		for _, siteID := range policy.AllowedSiteIDs {
			if siteID == candidate.Site.ID {
				allowed = true
				break
			}
		}
		if !allowed {
			return CandidateBlocker{"site_not_allowed", "站点不在下游密钥允许列表中"}
		}
	}

	for _, siteID := range policy.ExcludedSiteIDs {
		if siteID == candidate.Site.ID {
			return CandidateBlocker{"site_excluded", "站点已被下游密钥排除"}
		}
	}

	// allow-list: when AllowedCredentialRefs is non-empty, candidate must match one.
	if len(policy.AllowedCredentialRefs) > 0 {
		if !credentialRefMatchesCandidate(candidate, policy.AllowedCredentialRefs, s) {
			return CandidateBlocker{"credential_not_allowed", "API Key/令牌不在下游密钥允许列表中"}
		}
	}

	if len(policy.ExcludedCredentialRefs) == 0 {
		return CandidateBlocker{}
	}

	for _, ref := range policy.ExcludedCredentialRefs {
		if credentialRefMatchesOne(candidate, ref, s) {
			return CandidateBlocker{"credential_excluded", "API Key/令牌已被下游密钥排除"}
		}
	}
	return CandidateBlocker{}
}

func credentialRefMatchesCandidate(candidate RouteChannelCandidate, refs []CredentialRef, s *ChannelSelector) bool {
	for _, ref := range refs {
		if credentialRefMatchesOne(candidate, ref, s) {
			return true
		}
	}
	return false
}

func credentialRefMatchesOne(candidate RouteChannelCandidate, ref CredentialRef, s *ChannelSelector) bool {
	if candidate.Direct != nil {
		return ref.Kind == "direct_grant" && ref.GrantID == candidate.Direct.GrantID
	}
	if ref.Kind == "account_token" {
		return candidate.Channel.TokenID != nil && *candidate.Channel.TokenID == ref.TokenID &&
			candidate.Token != nil && candidate.Token.ID == ref.TokenID &&
			candidate.Account.ID == ref.AccountID &&
			candidate.Site.ID == ref.SiteID
	}
	// default_api_key (or empty kind legacy): channel without explicit token, using account apiToken.
	if candidate.Channel.TokenID == nil &&
		candidate.Account.ID == ref.AccountID &&
		candidate.Site.ID == ref.SiteID {
		tokenValue := s.resolveChannelTokenValue(candidate)
		apiToken := ""
		if candidate.Account.APIToken != nil {
			apiToken = *candidate.Account.APIToken
		}
		return tokenValue != "" && apiToken != "" && tokenValue == apiToken
	}
	return false
}
