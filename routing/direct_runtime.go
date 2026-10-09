package routing

import (
	"context"
	"fmt"
)

// DirectRuntimeStore is the persistence boundary for imported grant health.
// Grant state is shared by its group items, not by unrelated credentials.
type DirectRuntimeStore interface {
	RecordDirectSuccess(context.Context, int64, float64, float64) error
	RecordDirectFailure(context.Context, int64, SiteRuntimeFailureContext, int) error
}

func (tr *TokenRouter) recordDirectSuccess(ctx context.Context, itemID int64, latency, cost float64) error {
	db, ok := tr.db.(DirectRuntimeStore)
	if !ok {
		return fmt.Errorf("direct upstream runtime persistence unavailable")
	}
	if err := db.RecordDirectSuccess(ctx, itemID, latency, cost); err != nil {
		return err
	}
	tr.cache.InvalidateAll()
	return nil
}
func (tr *TokenRouter) recordDirectFailure(ctx context.Context, itemID int64, failure SiteRuntimeFailureContext) error {
	db, ok := tr.db.(DirectRuntimeStore)
	if !ok {
		return fmt.Errorf("direct upstream runtime persistence unavailable")
	}
	if err := db.RecordDirectFailure(ctx, itemID, failure, tr.configuredMaxSec); err != nil {
		return err
	}
	tr.cache.InvalidateAll()
	return nil
}
