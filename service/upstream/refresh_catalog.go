package upstream

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
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
		rows, err := loadCatalogModels(ctx, tx, channelID)
		if err != nil {
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
			_, routeIDs, err := staleImpact(ctx, tx, row.ID)
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
