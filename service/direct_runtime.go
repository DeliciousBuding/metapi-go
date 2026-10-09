package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
)

var _ routing.DirectRuntimeStore = (*ProxyRoutingStore)(nil)

func (s *ProxyRoutingStore) RecordDirectSuccess(ctx context.Context, itemID int64, latency, cost float64) error {
	if latency < 0 || cost < 0 || math.IsNaN(latency) || math.IsInf(latency, 0) || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return fmt.Errorf("invalid direct upstream observation")
	}
	result, err := s.execContext(ctx, `UPDATE upstream_grants SET success_count=success_count+1,
  total_latency_ms=total_latency_ms+?,total_cost=total_cost+?,fail_count=fail_count/2,
  cooldown_until=NULL,cooldown_reason_code=NULL,last_used_at=?
  WHERE id=(SELECT grant_id FROM upstream_group_items WHERE id=?)`, int64(latency), cost, time.Now().UTC().Format(time.RFC3339), itemID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("direct upstream item not found")
	}
	return nil
}
func (s *ProxyRoutingStore) RecordDirectFailure(ctx context.Context, itemID int64, failure routing.SiteRuntimeFailureContext, configuredMaxSec int) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state struct {
		ID        int64 `db:"id"`
		FailCount int64 `db:"fail_count"`
	}
	if err := tx.GetContext(ctx, &state, s.db.Rebind(`UPDATE upstream_grants SET fail_count=fail_count+1
  WHERE id=(SELECT grant_id FROM upstream_group_items WHERE id=?) RETURNING id,fail_count`), itemID); err != nil {
		return err
	}
	// Thirty-two Fibonacci steps already exceed the maximum allowed backoff;
	// bounding the input avoids overflow after a long-lived upstream outage.
	until := routing.ApplyFibonacciCooldown(min(state.FailCount, 32), time.Now().UnixMilli(), configuredMaxSec)
	reason, _ := routing.ClassifyCooldownReason(failure, routing.CooldownTriggerTraffic)
	if _, err = tx.ExecContext(ctx, s.db.Rebind(`UPDATE upstream_grants SET cooldown_until=?,cooldown_reason_code=?,last_fail_at=? WHERE id=?`), until, reason, time.Now().UTC().Format(time.RFC3339), state.ID); err != nil {
		return err
	}
	return tx.Commit()
}
