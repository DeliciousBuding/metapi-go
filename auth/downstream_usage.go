package auth

import (
	"log/slog"
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// consumeManagedKeyRequest atomically reserves one request for a managed key.
// Called by ReauthorizeManagedKeyForTurn so each HTTP request and each
// Responses WebSocket message is counted exactly once.
// It returns false when the key no longer exists or has reached max_requests.
func consumeManagedKeyRequest(db *store.DB, keyID int64) bool {
	if db == nil {
		slog.Warn("consumeManagedKeyRequest: database not initialized, skipping")
		return false
	}

	nowISO := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(
		`UPDATE downstream_api_keys
		 SET used_requests = COALESCE(used_requests, 0) + 1,
		     last_used_at = ?,
		     updated_at = ?
		 WHERE id = ?
		   AND (max_requests IS NULL OR COALESCE(used_requests, 0) < max_requests)`,
		nowISO, nowISO, keyID,
	)
	if err != nil {
		slog.Warn("consumeManagedKeyRequest: failed to increment used_requests",
			"keyID", keyID, "error", err)
		return false
	}
	rows, err := result.RowsAffected()
	if err != nil {
		slog.Warn("consumeManagedKeyRequest: failed to read affected row count",
			"keyID", keyID, "error", err)
		return false
	}
	return rows > 0
}

// RecordManagedKeyCostUsage atomically increments used_cost for a managed key.
// Only called when estimatedCost > 0 (checked by caller).
// This is NOT called from the auth middleware; it is called by the proxy
// handler after the request completes.
func RecordManagedKeyCostUsage(db *store.DB, keyID int64, estimatedCost float64) {
	if estimatedCost <= 0 || math.IsNaN(estimatedCost) || math.IsInf(estimatedCost, 0) {
		return
	}

	if db == nil {
		slog.Warn("RecordManagedKeyCostUsage: database not initialized, skipping")
		return
	}

	nowISO := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`UPDATE downstream_api_keys
		 SET used_cost = COALESCE(used_cost, 0) + ?,
		     last_used_at = ?,
		     updated_at = ?
		 WHERE id = ?`,
		estimatedCost, nowISO, nowISO, keyID,
	)
	if err != nil {
		slog.Warn("RecordManagedKeyCostUsage: failed to increment used_cost",
			"keyID", keyID, "cost", estimatedCost, "error", err)
	}
}
