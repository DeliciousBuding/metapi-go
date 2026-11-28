package upstream

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

// DiscoveryCredentialResolver resolves one stored credential to a usable
// bearer token through the shared OAuth/service-account lifecycle. The caller
// wires it — the admin handler uses oauth.ResolveDirectCredential; nil means
// only static API keys are usable. Resolving may write the credential's token
// cache (the same lazily-refreshed cache real requests maintain); catalog
// rows are still never written by preview.
type DiscoveryCredentialResolver func(ctx context.Context, credentialID int64, proxyURL *string) (string, error)

var errDiscoveryExchangeFailed = errors.New("discovery credential token exchange failed")

type catalogChannelRow struct {
	ID                int64                     `db:"id"`
	Name              string                    `db:"name"`
	Provider          string                    `db:"provider"`
	Enabled           bool                      `db:"enabled"`
	EndpointConfig    store.DirectEndpoints     `db:"endpoint_config"`
	UseSystemProxy    bool                      `db:"proxy"`
	ChannelProxy      string                    `db:"channel_proxy"`
	CustomHeaders     string                    `db:"custom_header"`
	RequestConfig     store.DirectRequestConfig `db:"request_config"`
	DiscoveryIdentity string                    `db:"-"`
}

func loadCatalogChannel(ctx context.Context, db *sqlx.DB, channelID int64) (*catalogChannelRow, error) {
	var row catalogChannelRow
	err := db.GetContext(ctx, &row, db.Rebind(`SELECT id,name,provider,enabled,endpoint_config,proxy,channel_proxy,custom_header,request_config FROM upstream_channels WHERE id=?`), channelID)
	if err != nil {
		return nil, &Error{Status: 404, Message: "Upstream channel not found"}
	}
	return &row, nil
}

// loadDiscoveryCredential returns a usable bearer token from the channel's
// first usable enabled credential. Static API keys are used as-is. OAuth and
// service accounts always go through the credential owner, even when secret
// contains an old cached token: expiry, replacement and revocation are not
// decisions the catalog can safely infer. Tokens never leave the process.
func loadDiscoveryCredential(ctx context.Context, db *sqlx.DB, channelID int64, proxyURL *string, resolve DiscoveryCredentialResolver) (string, error) {
	credential, err := loadDiscoveryCredentialForKinds(ctx, db, channelID, proxyURL, resolve, nil)
	return credential.Secret, err
}

type catalogCredential struct{ Kind, Secret string }

func loadDiscoveryCredentialForKinds(ctx context.Context, db *sqlx.DB, channelID int64, proxyURL *string, resolve DiscoveryCredentialResolver, kinds []string) (catalogCredential, error) {
	rows := []struct {
		ID     int64  `db:"id"`
		Kind   string `db:"kind"`
		Secret string `db:"secret"`
	}{}
	if err := db.SelectContext(ctx, &rows, db.Rebind(`SELECT id,kind,secret FROM upstream_credentials WHERE channel_id=? AND enabled ORDER BY id`), channelID); err != nil {
		return catalogCredential{}, err
	}
	exchangeFailed := false
	for _, row := range rows {
		if len(kinds) > 0 && !slices.Contains(kinds, row.Kind) {
			continue
		}
		switch row.Kind {
		case store.DirectCredentialAPIKey:
			if row.Secret != "" {
				return catalogCredential{row.Kind, row.Secret}, nil
			}
			continue
		case store.DirectCredentialOAuth, store.DirectCredentialGCPServiceAccount:
			// These kinds require a current credential resolution, never a
			// fallback to their stored token after the resolver rejects them.
		case store.DirectCredentialNone:
			// Only an explicit provider capability may request this identity.
			// The shared resolver verifies the channel's anonymous policy and
			// stored material; an absent token is not proof of public access.
			if !slices.Contains(kinds, store.DirectCredentialNone) || row.Secret != "" {
				continue
			}
		default:
			continue
		}
		if resolve == nil {
			continue
		}
		token, err := resolve(ctx, row.ID, proxyURL)
		anonymous := row.Kind == store.DirectCredentialNone
		if err != nil || token == "" && !anonymous || token != "" && anonymous {
			exchangeFailed = true
			continue
		}
		var current struct {
			Kind   string `db:"kind"`
			Secret string `db:"secret"`
		}
		if db.GetContext(ctx, &current, db.Rebind(`SELECT kind,secret FROM upstream_credentials WHERE id=? AND channel_id=? AND enabled`), row.ID, channelID) != nil || len(kinds) > 0 && !slices.Contains(kinds, current.Kind) || anonymous && (current.Kind != store.DirectCredentialNone || current.Secret != "") {
			exchangeFailed = true
			continue
		}
		return catalogCredential{current.Kind, token}, nil
	}
	if exchangeFailed {
		return catalogCredential{}, errDiscoveryExchangeFailed
	}
	return catalogCredential{}, sql.ErrNoRows
}

// discoverChannelCatalog discovers a stored channel without preset fallbacks.
func discoverChannelCatalog(ctx context.Context, db *sqlx.DB, channel *catalogChannelRow, resolve DiscoveryCredentialResolver) ([]string, Discovery, error) {
	if binding := channel.RequestConfig.Provider; binding != nil && binding.Catalog != nil {
		if err := channel.RequestConfig.ValidateEndpoints(channel.EndpointConfig); err != nil {
			return nil, Discovery{}, invalid("Invalid provider catalog binding")
		}
		manager := channelplugins.New(db)
		identity := providerCatalogChannelIdentity(channel)
		read := func(ctx context.Context, kinds []string) (catalogCredential, error) {
			proxyURL := effectiveCatalogProxy(channel.ChannelProxy, channel.UseSystemProxy)
			var proxyPtr *string
			if proxyURL != "" {
				proxyPtr = &proxyURL
			}
			return loadDiscoveryCredentialForKinds(ctx, db, channel.ID, proxyPtr, resolve, kinds)
		}
		verify := func(ctx context.Context) error {
			current, err := loadCatalogChannel(ctx, db, channel.ID)
			if err != nil || providerCatalogChannelIdentity(current) != identity {
				return conflict("Channel discovery configuration changed; retry discovery")
			}
			return nil
		}
		models, discovery, code, err := discoverProviderCatalog(ctx, manager, *binding, channel.ChannelProxy, channel.CustomHeaders, channel.UseSystemProxy, read, verify)
		if err == nil {
			channel.DiscoveryIdentity = identity + ":" + code
		}
		return models, discovery, err
	}
	credential := func(ctx context.Context) (string, error) {
		// The token exchange uses the same egress path as the catalog call.
		proxyURL := channel.ChannelProxy
		if proxyURL == "" && channel.UseSystemProxy {
			if runtime := config.RuntimeSafe(); runtime != nil {
				proxyURL = runtime.SystemProxyUrl
			}
		}
		var proxyPtr *string
		if proxyURL != "" {
			proxyPtr = &proxyURL
		}
		secret, err := loadDiscoveryCredential(ctx, db, channel.ID, proxyPtr, resolve)
		if err != nil {
			if errors.Is(err, errDiscoveryExchangeFailed) {
				return "", invalid("Channel credential token exchange failed")
			}
			return "", invalid("Channel has no enabled credential for model discovery")
		}
		return secret, nil
	}
	models, discovery, err := discoverUpstreamCatalog(ctx, channel.EndpointConfig, credential, channel.ChannelProxy, channel.CustomHeaders, channel.UseSystemProxy)
	if err != nil {
		switch err {
		case errNoCatalogEndpoint:
			return nil, Discovery{}, invalid("Channel endpoints have no derivable model catalog URL")
		case errCatalogUnserved:
			return nil, Discovery{}, invalid("Upstream does not serve a model catalog at the configured endpoint")
		default:
			return nil, Discovery{}, err
		}
	}
	return models, discovery, nil
}
