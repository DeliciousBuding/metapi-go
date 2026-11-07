package upstream

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

// Catalog refresh with discovery diff (route-decision E batch).
//
// The operator question: "the upstream added/removed models since I connected
// — what would change if I re-discover now?" Preview computes the diff without
// writing; apply re-discovers and requires the preview's basis token, so a
// stale preview (catalog edited or upstream changed in between) is refused
// with 409 instead of silently applying an outdated plan.
//
// Source attribution: rows created by discovery (connect or refresh-apply)
// carry origin discovery:<channelID>; operator-created rows keep native:local
// and are NEVER touched by refresh (manual preservation). Legacy rows from
// before attribution existed stay native and are reported, not reattributed.

// DiscoveryOriginPrefix prefixes per-channel discovery origins.
const DiscoveryOriginPrefix = "discovery:"

// DiscoveryOriginForChannel returns the discovery origin key for one channel.
func DiscoveryOriginForChannel(channelID int64) string {
	return DiscoveryOriginPrefix + strconv.FormatInt(channelID, 10)
}

// CatalogStaleModel is one discovery-origin row whose name vanished upstream.
type CatalogStaleModel struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Enabled    bool    `json:"enabled"`
	GrantCount int     `json:"grantCount"`
	RouteIDs   []int64 `json:"routeIds"`
}

// CatalogRefreshPreview is the read-only diff between the live upstream
// catalog and the stored one.
type CatalogRefreshPreview struct {
	ChannelID     int64               `json:"channelId"`
	BasisToken    string              `json:"basisToken"`
	Discovery     Discovery           `json:"discovery"`
	UpstreamCount int                 `json:"upstreamCount"`
	Added         []string            `json:"added"`
	Stale         []CatalogStaleModel `json:"stale"`
	Kept          []string            `json:"kept"`
	// Offline: discovery-origin rows already disabled and still absent upstream.
	// Informational history; apply does nothing for them.
	Offline           []string `json:"offline"`
	ManualOnly        []string `json:"manualOnly"`
	ManualKept        []string `json:"manualKept"`
	LegacyNativeCount int      `json:"legacyNativeCount"`
}

// CatalogRefreshApplyResult reports what one apply actually changed.
type CatalogRefreshApplyResult struct {
	AddedCount       int     `json:"addedCount"`
	StaleCount       int     `json:"staleCount"`
	AffectedRouteIDs []int64 `json:"affectedRouteIds"`
}

// DiscoveryCredentialResolver resolves one stored credential to a usable
// bearer token through the shared OAuth/service-account lifecycle. The caller
// wires it — the admin handler uses oauth.ResolveDirectCredential; nil means
// only static API keys are usable. Resolving may write the credential's token
// cache (the same lazily-refreshed cache real requests maintain); catalog
// rows are still never written by preview.
type DiscoveryCredentialResolver func(ctx context.Context, credentialID int64, proxyURL *string) (string, error)

var errDiscoveryExchangeFailed = errors.New("discovery credential token exchange failed")

type catalogModelRow struct {
	ID      int64  `db:"id"`
	Name    string `db:"name"`
	Enabled bool   `db:"enabled"`
	Origin  string `db:"origin_key"`
}

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
		default:
			continue
		}
		if resolve == nil {
			continue
		}
		token, err := resolve(ctx, row.ID, proxyURL)
		if err != nil || token == "" {
			exchangeFailed = true
			continue
		}
		var current struct {
			Kind string `db:"kind"`
		}
		if db.GetContext(ctx, &current, db.Rebind(`SELECT kind FROM upstream_credentials WHERE id=? AND channel_id=? AND enabled`), row.ID, channelID) != nil || len(kinds) > 0 && !slices.Contains(kinds, current.Kind) {
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

func loadCatalogModels(ctx context.Context, db *sqlx.DB, channelID int64) ([]catalogModelRow, error) {
	rows := []catalogModelRow{}
	if err := db.SelectContext(ctx, &rows, db.Rebind(`SELECT id,name,enabled,origin_key FROM upstream_models WHERE channel_id=? ORDER BY id`), channelID); err != nil {
		return nil, err
	}
	return rows, nil
}

// catalogBasisToken binds the catalog rows and the discovered set: applying a
// preview is only valid while both are unchanged.
func catalogBasisToken(rows []catalogModelRow, discovered []string, identity ...string) string {
	h := sha256.New()
	if len(identity) > 0 && identity[0] != "" {
		_, _ = h.Write([]byte("provider:" + identity[0] + "\x00"))
	}
	for _, row := range rows {
		fmt.Fprintf(h, "%d|%s|%t|%s\n", row.ID, row.Name, row.Enabled, row.Origin)
	}
	h.Write([]byte{0})
	sorted := slices.Clone(discovered)
	slices.Sort(sorted)
	for _, name := range sorted {
		h.Write([]byte(name + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// staleImpact counts the grants and the routes referencing one model, so the
// operator sees what disabling it affects before confirming.
func staleImpact(ctx context.Context, db *sqlx.DB, modelID int64) (int, []int64, error) {
	var grantCount int
	if err := db.GetContext(ctx, &grantCount, db.Rebind(`SELECT COUNT(*) FROM upstream_grants WHERE model_id=?`), modelID); err != nil {
		return 0, nil, err
	}
	routeIDs := []int64{}
	if err := db.SelectContext(ctx, &routeIDs, db.Rebind(`SELECT DISTINCT rg.route_id FROM upstream_route_groups rg JOIN upstream_group_items i ON i.group_id=rg.group_id JOIN upstream_grants g ON g.id=i.grant_id WHERE g.model_id=? ORDER BY rg.route_id`), modelID); err != nil {
		return 0, nil, err
	}
	return grantCount, routeIDs, nil
}

func computeCatalogDiff(rows []catalogModelRow, discovered []string, discoveryOrigin string) (added, kept, manualOnly, manualKept, offline []string, stale []catalogModelRow) {
	catalogNames := map[string]bool{}
	upstreamSet := map[string]bool{}
	for _, name := range discovered {
		upstreamSet[name] = true
	}
	added, kept, manualOnly, manualKept, offline = []string{}, []string{}, []string{}, []string{}, []string{}
	stale = []catalogModelRow{}
	for _, row := range rows {
		catalogNames[row.Name] = true
		switch row.Origin {
		case discoveryOrigin:
			if upstreamSet[row.Name] {
				kept = append(kept, row.Name)
			} else if row.Enabled {
				stale = append(stale, row)
			} else {
				offline = append(offline, row.Name)
			}
		default:
			// Native rows are operator-owned: never refreshed away, reported
			// for visibility only.
			if upstreamSet[row.Name] {
				manualKept = append(manualKept, row.Name)
			} else {
				manualOnly = append(manualOnly, row.Name)
			}
		}
	}
	for _, name := range discovered {
		if !catalogNames[name] {
			added = append(added, name)
		}
	}
	return added, kept, manualOnly, manualKept, offline, stale
}

// PreviewCatalogRefresh computes the diff for one channel. Catalog rows are
// never written; a gcp_service_account credential may perform its token
// exchange (cached under the credential) so discovery can authenticate.
func PreviewCatalogRefresh(ctx context.Context, db *sqlx.DB, channelID int64, resolve DiscoveryCredentialResolver) (CatalogRefreshPreview, error) {
	channel, err := loadCatalogChannel(ctx, db, channelID)
	if err != nil {
		return CatalogRefreshPreview{}, err
	}
	discovered, discovery, err := discoverChannelCatalog(ctx, db, channel, resolve)
	if err != nil {
		return CatalogRefreshPreview{}, err
	}
	rows, err := loadCatalogModels(ctx, db, channelID)
	if err != nil {
		return CatalogRefreshPreview{}, err
	}
	discoveryOrigin := DiscoveryOriginForChannel(channelID)
	added, kept, manualOnly, manualKept, offline, staleRows := computeCatalogDiff(rows, discovered, discoveryOrigin)

	stale := make([]CatalogStaleModel, 0, len(staleRows))
	for _, row := range staleRows {
		grantCount, routeIDs, err := staleImpact(ctx, db, row.ID)
		if err != nil {
			return CatalogRefreshPreview{}, err
		}
		stale = append(stale, CatalogStaleModel{ID: row.ID, Name: row.Name, Enabled: row.Enabled, GrantCount: grantCount, RouteIDs: routeIDs})
	}

	legacyNative := 0
	for _, row := range rows {
		if row.Origin == NativeOrigin {
			legacyNative++
		}
	}
	return CatalogRefreshPreview{
		ChannelID:         channelID,
		BasisToken:        catalogBasisToken(rows, discovered, channel.DiscoveryIdentity),
		Discovery:         discovery,
		UpstreamCount:     len(discovered),
		Added:             added,
		Stale:             stale,
		Kept:              kept,
		Offline:           offline,
		ManualOnly:        manualOnly,
		ManualKept:        manualKept,
		LegacyNativeCount: legacyNative,
	}, nil
}

// ApplyCatalogRefresh applies a previously previewed diff. The basis token is
// recomputed from a fresh discovery plus the locked catalog rows; any change
// since the preview — catalog edits, concurrent applies, upstream movement —
// fails with 409 and requires a new preview.
func ApplyCatalogRefresh(ctx context.Context, db *sqlx.DB, channelID int64, basisToken string, resolve DiscoveryCredentialResolver) (CatalogRefreshApplyResult, error) {
	if strings.TrimSpace(basisToken) == "" {
		return CatalogRefreshApplyResult{}, invalid("basisToken is required")
	}
	channel, err := loadCatalogChannel(ctx, db, channelID)
	if err != nil {
		return CatalogRefreshApplyResult{}, err
	}
	discovered, _, err := discoverChannelCatalog(ctx, db, channel, resolve)
	if err != nil {
		return CatalogRefreshApplyResult{}, err
	}

	result := CatalogRefreshApplyResult{AffectedRouteIDs: []int64{}}
	discoveryOrigin := DiscoveryOriginForChannel(channelID)
	err = Write(ctx, db, func(tx *sqlx.Tx) error {
		// Lock the channel row so concurrent applies serialize here.
		var exists int
		if err := tx.GetContext(ctx, &exists, tx.Rebind(lockQuery(tx, `SELECT 1 FROM upstream_channels WHERE id=?`)), channelID); err != nil {
			return err
		}
		rows := []catalogModelRow{}
		if err := tx.SelectContext(ctx, &rows, tx.Rebind(`SELECT id,name,enabled,origin_key FROM upstream_models WHERE channel_id=? ORDER BY id`), channelID); err != nil {
			return err
		}
		if channel.DiscoveryIdentity != "" {
			var current catalogChannelRow
			if err := tx.GetContext(ctx, &current, tx.Rebind(`SELECT id,name,provider,enabled,endpoint_config,proxy,channel_proxy,custom_header,request_config FROM upstream_channels WHERE id=?`), channelID); err != nil {
				return err
			}
			if current.RequestConfig.Provider == nil {
				return conflict("Channel discovery configuration changed; retry discovery")
			}
			_, code, err := channelplugins.New(tx).DescribeProvider(ctx, *current.RequestConfig.Provider)
			if err != nil || providerCatalogChannelIdentity(&current)+":"+code != channel.DiscoveryIdentity {
				return conflict("Channel discovery configuration changed; retry discovery")
			}
		}
		if catalogBasisToken(rows, discovered, channel.DiscoveryIdentity) != basisToken {
			return conflict("Preview is stale: the catalog or upstream models changed since it was computed; run the preview again")
		}
		added, _, _, _, _, staleRows := computeCatalogDiff(rows, discovered, discoveryOrigin)
		affected := map[int64]bool{}
		for _, name := range added {
			if _, err := InsertWithOrigin(ctx, tx, discoveryOrigin, "models", channelID, name, true); err != nil {
				return err
			}
			result.AddedCount++
		}
		for _, row := range staleRows {
			if !row.Enabled {
				continue
			}
			// Origin guard: a row reattributed between preview and apply is
			// simply not ours to touch.
			res, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE upstream_models SET enabled=? WHERE id=? AND origin_key=?`), false, row.ID, discoveryOrigin)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				result.StaleCount++
			}
			_, routeIDs, err := staleImpactTx(ctx, tx, row.ID)
			if err != nil {
				return err
			}
			for _, id := range routeIDs {
				affected[id] = true
			}
		}
		for id := range affected {
			result.AffectedRouteIDs = append(result.AffectedRouteIDs, id)
		}
		slices.Sort(result.AffectedRouteIDs)
		return nil
	})
	return result, err
}

func staleImpactTx(ctx context.Context, tx *sqlx.Tx, modelID int64) (int, []int64, error) {
	var grantCount int
	if err := tx.GetContext(ctx, &grantCount, tx.Rebind(`SELECT COUNT(*) FROM upstream_grants WHERE model_id=?`), modelID); err != nil {
		return 0, nil, err
	}
	routeIDs := []int64{}
	if err := tx.SelectContext(ctx, &routeIDs, tx.Rebind(`SELECT DISTINCT rg.route_id FROM upstream_route_groups rg JOIN upstream_group_items i ON i.group_id=rg.group_id JOIN upstream_grants g ON g.id=i.grant_id WHERE g.model_id=? ORDER BY rg.route_id`), modelID); err != nil {
		return 0, nil, err
	}
	return grantCount, routeIDs, nil
}
