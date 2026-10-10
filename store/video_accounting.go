package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// VideoTaskUsage is a cumulative upstream observation, not a new HTTP charge.
type VideoTaskUsage struct {
	Found               bool  `json:"found"`
	PromptTokens        int64 `json:"promptTokens"`
	CompletionTokens    int64 `json:"completionTokens"`
	TotalTokens         int64 `json:"totalTokens"`
	CacheReadTokens     int64 `json:"cacheReadTokens"`
	CacheCreationTokens int64 `json:"cacheCreationTokens"`
	ReasoningTokens     int64 `json:"reasoningTokens"`
}

type VideoTaskAccounting struct {
	Version     int            `json:"version"`
	Initialized bool           `json:"initialized"`
	KeyID       int64          `json:"keyId"`
	Usage       VideoTaskUsage `json:"usage"`
	Cost        float64        `json:"cost"`
}

type VideoTaskAccountingResult struct {
	Delta     VideoTaskUsage
	CostDelta float64
	Legacy    bool
}

// AccountVideoTask atomically advances the task's high water mark and all
// durable billing sinks. The no-op UPDATE takes the row/write lock before the
// read on both PostgreSQL and SQLite; no separate claim/async increment exists.
func AccountVideoTask(ctx context.Context, db *DB, publicID, owner string, grantID, keyID int64, observed VideoTaskUsage, cumulativeCost float64) (VideoTaskAccountingResult, error) {
	var result VideoTaskAccountingResult
	if db == nil || publicID == "" || owner == "" || grantID <= 0 || keyID < 0 || cumulativeCost < 0 || math.IsNaN(cumulativeCost) || math.IsInf(cumulativeCost, 0) {
		return result, fmt.Errorf("invalid video task accounting")
	}
	for _, value := range []int64{observed.PromptTokens, observed.CompletionTokens, observed.TotalTokens, observed.CacheReadTokens, observed.CacheCreationTokens, observed.ReasoningTokens} {
		if value < 0 {
			return result, fmt.Errorf("invalid cumulative video usage")
		}
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var raw sql.NullString
	var created, identity string
	err = tx.QueryRowContext(ctx, db.Rebind(`UPDATE proxy_video_tasks SET accounting_state=accounting_state
		WHERE public_id=? AND direct_identity IS NOT NULL RETURNING accounting_state,created_at,direct_identity`), publicID).Scan(&raw, &created, &identity)
	if err != nil {
		return result, err
	}
	var bound struct {
		Owner   string `json:"owner"`
		GrantID int64  `json:"grantId"`
	}
	if json.Unmarshal([]byte(identity), &bound) != nil || bound.Owner != owner || bound.GrantID != grantID {
		return result, fmt.Errorf("video accounting identity changed")
	}
	if !raw.Valid {
		// Pre-accounting tasks have no recoverable link to old random quota
		// events. Do not guess a zero baseline and bill them again on upgrade.
		result.Legacy = true
		return result, tx.Commit()
	}
	var state VideoTaskAccounting
	if json.Unmarshal([]byte(raw.String), &state) != nil || state.Version != 1 || (state.Initialized && state.KeyID != keyID) {
		return result, fmt.Errorf("invalid video accounting state")
	}
	createdAt, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return result, fmt.Errorf("invalid video creation time")
	}
	previous := state.Usage
	if observed.Found {
		state.Usage = VideoTaskUsage{Found: true,
			PromptTokens:        max(previous.PromptTokens, observed.PromptTokens),
			CompletionTokens:    max(previous.CompletionTokens, observed.CompletionTokens),
			TotalTokens:         max(previous.TotalTokens, observed.TotalTokens),
			CacheReadTokens:     max(previous.CacheReadTokens, observed.CacheReadTokens),
			CacheCreationTokens: max(previous.CacheCreationTokens, observed.CacheCreationTokens),
			ReasoningTokens:     max(previous.ReasoningTokens, observed.ReasoningTokens)}
		result.Delta = VideoTaskUsage{Found: true,
			PromptTokens:        state.Usage.PromptTokens - previous.PromptTokens,
			CompletionTokens:    state.Usage.CompletionTokens - previous.CompletionTokens,
			TotalTokens:         state.Usage.TotalTokens - previous.TotalTokens,
			CacheReadTokens:     state.Usage.CacheReadTokens - previous.CacheReadTokens,
			CacheCreationTokens: state.Usage.CacheCreationTokens - previous.CacheCreationTokens,
			ReasoningTokens:     state.Usage.ReasoningTokens - previous.ReasoningTokens}
		result.CostDelta = math.Max(0, cumulativeCost-state.Cost)
		state.Cost += result.CostDelta
	}
	state.Initialized, state.KeyID = true, keyID
	encoded, err := json.Marshal(state)
	if err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`UPDATE proxy_video_tasks SET accounting_state=? WHERE public_id=?`), string(encoded), publicID); err != nil {
		return result, err
	}
	if keyID > 0 {
		var tokens, cost any
		if state.Usage.Found {
			tokens, cost = state.Usage.TotalTokens, state.Cost
		}
		_, err = tx.ExecContext(ctx, db.Rebind(`INSERT INTO downstream_quota_usage(key_id,event_key,occurred_at,requests,total_tokens,cost)
			VALUES (?,?,?,1,?,?) ON CONFLICT(key_id,event_key) DO UPDATE SET total_tokens=excluded.total_tokens,cost=excluded.cost`),
			keyID, "video:"+publicID, createdAt.UnixMilli(), tokens, cost)
		if err != nil {
			return result, err
		}
		updated, err := tx.ExecContext(ctx, db.Rebind(`UPDATE downstream_api_keys SET used_cost=COALESCE(used_cost,0)+? WHERE id=?`), result.CostDelta, keyID)
		if err := requireVideoAccountingRow(updated, err); err != nil {
			return result, err
		}
	}
	updated, err := tx.ExecContext(ctx, db.Rebind(`UPDATE upstream_grants SET total_cost=total_cost+? WHERE id=?`), result.CostDelta, grantID)
	if err := requireVideoAccountingRow(updated, err); err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func requireVideoAccountingRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("video accounting sink is unavailable")
	}
	return nil
}
