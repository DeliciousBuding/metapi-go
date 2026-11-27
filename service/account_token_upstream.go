package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/jmoiron/sqlx"
)

// UpstreamAPIToken is a token payload returned by platform adapters during sync.
type UpstreamAPIToken struct {
	Name       string
	Key        string
	Enabled    bool
	TokenGroup string
}

// UpstreamTokenListing carries the tokens plus the facts convergence needs:
// whether the adapter proved the listing complete, and whether a legacy
// fallback made even a short listing unsafe for default-switch decisions.
type UpstreamTokenListing struct {
	Tokens   []UpstreamAPIToken
	Complete bool
	// DefaultSwitchUnsafe is set for a legacy flat listing that answered without
	// a total. Its tokens remain syncable, but absence cannot prove revocation.
	DefaultSwitchUnsafe bool
}

// PlatformAPITokensToUpstream converts platform.ApiTokenInfo values to the
// local UpstreamAPIToken shape used by SyncTokensFromUpstream.
func PlatformAPITokensToUpstream(tokens []platform.ApiTokenInfo) []UpstreamAPIToken {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]UpstreamAPIToken, 0, len(tokens))
	for _, token := range tokens {
		key := strings.TrimSpace(token.Key)
		if key == "" {
			continue
		}
		out = append(out, UpstreamAPIToken{
			Name:       strings.TrimSpace(token.Name),
			Key:        key,
			Enabled:    token.Enabled,
			TokenGroup: strings.TrimSpace(token.TokenGroup),
		})
	}
	return out
}

type completeAPITokenLister interface {
	GetAPITokensComplete(ctx context.Context, baseURL, accessToken string, platformUserID *int, proxy *platform.ProxyConfig) ([]platform.ApiTokenInfo, bool, error)
}

// FetchUpstreamAPITokenListing loads tokens and carries whether the adapter
// proved the listing complete. Only adapters implementing the explicit
// complete-lister contract may bypass the convergence truncation guard.
func FetchUpstreamAPITokenListing(
	ctx context.Context,
	adapter platform.PlatformAdapter,
	baseURL, accessToken string,
	platformUserID *int,
	proxy *platform.ProxyConfig,
) (UpstreamTokenListing, error) {
	if adapter == nil {
		return UpstreamTokenListing{}, fmt.Errorf("platform adapter is nil")
	}

	var tokens []platform.ApiTokenInfo
	complete := false
	defaultSwitchUnsafe := false
	if lister, ok := adapter.(completeAPITokenLister); ok {
		var err error
		tokens, complete, err = lister.GetAPITokensComplete(ctx, baseURL, accessToken, platformUserID, proxy)
		if errors.Is(err, platform.ErrNewAPITokenListingCompletenessUnproven) {
			defaultSwitchUnsafe = true
			err = nil
		}
		if err != nil {
			return UpstreamTokenListing{}, err
		}
	} else {
		var err error
		tokens, err = adapter.GetAPITokens(ctx, baseURL, accessToken, platformUserID, proxy)
		if err != nil {
			return UpstreamTokenListing{}, err
		}
	}

	if len(tokens) == 0 && !complete && !defaultSwitchUnsafe {
		single, singleErr := adapter.GetAPIToken(ctx, baseURL, accessToken, platformUserID, proxy)
		if singleErr != nil {
			return UpstreamTokenListing{}, singleErr
		}
		if single != nil {
			key := strings.TrimSpace(*single)
			if key != "" {
				tokens = []platform.ApiTokenInfo{{
					Name:    "default",
					Key:     key,
					Enabled: true,
				}}
			}
		}
	}
	return UpstreamTokenListing{
		Tokens:              PlatformAPITokensToUpstream(tokens),
		Complete:            complete,
		DefaultSwitchUnsafe: defaultSwitchUnsafe,
	}, nil
}

// FetchUpstreamAPITokens preserves the legacy slice-only API for callers that
// do not carry listing completeness.
func FetchUpstreamAPITokens(
	ctx context.Context,
	adapter platform.PlatformAdapter,
	baseURL, accessToken string,
	platformUserID *int,
	proxy *platform.ProxyConfig,
) ([]UpstreamAPIToken, error) {
	listing, err := FetchUpstreamAPITokenListing(ctx, adapter, baseURL, accessToken, platformUserID, proxy)
	if err != nil {
		return nil, err
	}
	return listing.Tokens, nil
}

// NormalizeTokenGroups trims, de-duplicates, and ensures a non-empty result.
// When every input is blank, returns ["default"].
func NormalizeTokenGroups(groups []string) []string {
	seen := make(map[string]struct{}, len(groups))
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		trimmed := strings.TrimSpace(group)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{"default"}
	}
	return out
}

// GetLocalTokenGroups returns distinct token groups stored for the account.
func GetLocalTokenGroups(db *sqlx.DB, accountID int64) ([]string, error) {
	var groups []string
	err := db.Select(&groups,
		db.Rebind("SELECT DISTINCT COALESCE(token_group, 'default') FROM account_tokens WHERE account_id = ?"),
		accountID,
	)
	if err != nil {
		return nil, err
	}
	return NormalizeTokenGroups(groups), nil
}

// GetTokenGroups prefers platform.GetUserGroups for session accounts when an
// adapter and access token are available. Falls back to local DISTINCT
// token_group on adapter nil, missing access token, upstream error, or empty
// upstream result. Upstream errors are swallowed so local groups remain usable.
func GetTokenGroups(
	ctx context.Context,
	db *sqlx.DB,
	accountID int64,
	adapter platform.PlatformAdapter,
	baseURL, accessToken string,
	platformUserID *int,
	proxy *platform.ProxyConfig,
) ([]string, error) {
	accessToken = strings.TrimSpace(accessToken)
	if adapter != nil && accessToken != "" {
		if ctx == nil {
			ctx = context.Background()
		}
		upstream, err := adapter.GetUserGroups(ctx, baseURL, accessToken, platformUserID, proxy)
		if err == nil {
			hasValue := false
			for _, group := range upstream {
				if strings.TrimSpace(group) != "" {
					hasValue = true
					break
				}
			}
			if hasValue {
				return NormalizeTokenGroups(upstream), nil
			}
		}
		// nil adapter path is handled below; upstream error/empty → local fallback
	}
	return GetLocalTokenGroups(db, accountID)
}
