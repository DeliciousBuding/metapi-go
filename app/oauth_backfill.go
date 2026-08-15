package app

import (
	"log/slog"

	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

// RunOauthIdentityBackfill runs the one-time OAuth identity column backfill at
// startup. It is a bounded migration (LIMIT-batched, settings-marker-gated) so
// the paginated admin list endpoint never pays the per-page scan+update cost
// that the old in-request backfill imposed.
//
// Safe to call on every boot: the settings marker short-circuits healthy
// instances, and a partial run (process killed mid-batch) leaves the marker
// unset and retries on the next boot. Must run after the runtime database is
// bootstrapped (store.EnsureRuntimeDatabase) so store.GetDB is non-nil.
func RunOauthIdentityBackfill() {
	db := store.GetDB()
	if db == nil {
		slog.Warn("oauth identity backfill skipped: database not initialized")
		return
	}
	oauth.RunOauthIdentityBackfillOnce(db)
}
