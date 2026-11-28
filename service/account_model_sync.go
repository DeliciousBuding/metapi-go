package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/platform"
)

// routeRebuildLocalBudget bounds the closing rebuild of a pass. It is local
// database work over the availability the pass just wrote, so minutes are
// generosity, not need.
const routeRebuildLocalBudget = 5 * time.Minute

// ModelSyncSummary reports one periodic model-sync pass. Every pass ends with
// exactly one rebuild, so there is no "did it run" flag to report.
type ModelSyncSummary struct {
	Total   int
	Success int
	Failed  int

	Rebuild    RouteRebuildStats
	RebuildErr error
}

// SyncAllAccountModels refreshes upstream model lists for all candidate
// accounts sequentially (polite to upstreams, no concurrency races). Each
// account refresh skips its own route rebuild; after the pass exactly one route
// rebuild + one routing-cache invalidation runs for the whole batch.
// Per-account failures are logged and counted without aborting the pass.
//
// The closing rebuild always runs, on a context that survives this one (#1174).
// It only reads and writes the local database — the upstream work is already
// over — so abandoning it because the pass hit a deadline or its caller went
// away left routes composed from an older view of availability, which is worse
// than finishing the cheap local step. When nothing changed it short-circuits
// without a single write, so "always" costs a read pass, not churn.
func SyncAllAccountModels(ctx context.Context, db *sqlx.DB) ModelSyncSummary {
	var summary ModelSyncSummary
	if db == nil {
		slog.Error("model-sync: database not available")
		return summary
	}

	candidates := listModelSyncCandidates(db)
	summary.Total = len(candidates)

	for i, accountID := range candidates {
		if err := ctx.Err(); err != nil {
			slog.Warn("model-sync: pass aborted before finishing all candidates",
				"processed", i, "remaining", len(candidates)-i, "error", err)
			break
		}
		res := RefreshAccountModels(ctx, db, accountID, false, true)
		if res.Success {
			summary.Success++
			slog.Info("model-sync: account refreshed", "account_id", accountID, "models", len(res.Models))
			continue
		}
		summary.Failed++
		slog.Warn("model-sync: account refresh failed",
			"account_id", accountID, "error_code", res.ErrorCode, "error", res.ErrorMessage)
	}

	rebuildCtx, cancelRebuild := context.WithTimeout(
		context.WithoutCancel(ctx), routeRebuildLocalBudget)
	defer cancelRebuild()
	rebuildStats, rebuildErr := modelRefreshRebuildRoutes(rebuildCtx, db)
	summary.Rebuild = rebuildStats
	summary.RebuildErr = rebuildErr
	modelRefreshInvalidateCache()
	if rebuildErr != nil {
		slog.Warn("model-sync: route rebuild failed", "error", rebuildErr)
	}

	slog.Info("model-sync complete", "total", summary.Total, "success", summary.Success, "failed", summary.Failed)
	return summary
}

// listModelSyncCandidates returns the account IDs eligible for periodic model
// sync: status "active" or empty (disabled and other non-empty statuses are
// skipped), a resolvable credential (api_token or access_token), and a known
// site adapter. Accounts failing any check are not candidates at all — they
// never count against the pass's failed total.
func listModelSyncCandidates(db *sqlx.DB) []int64 {
	var ids []int64
	err := db.Select(&ids, db.Rebind(
		"SELECT id FROM accounts WHERE status = ? OR status = ? OR status IS NULL ORDER BY id",
	), "active", "")
	if err != nil {
		slog.Error("model-sync: failed to query candidate accounts", "error", err)
		return nil
	}

	candidates := make([]int64, 0, len(ids))
	for _, id := range ids {
		row, err := GetAccountWithSiteByID(db, id)
		if err != nil || row == nil {
			continue
		}
		if platform.GetAdapter(row.Site.Platform) == nil {
			continue
		}
		if resolveAccountModelToken(&row.Account) == "" {
			continue
		}
		candidates = append(candidates, id)
	}
	return candidates
}
