package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// projectionRow is a lightweight projection row from proxy_logs.
type projectionRow struct {
	id               int64
	status           *string
	promptTokens     *int64
	completionTokens *int64
	tokens           *int64
	cost             *float64
	siteID           *int64
	platform         *string
	model            *string
	latencyMs        *int64
	createdAt        *string
}

func (s *UsageAggregationScheduler) fetchBatch(dbw *store.DB, afterID int64, limit int) ([]projectionRow, error) {
	rows, err := dbw.Query(`
		SELECT pl.id, pl.status, pl.prompt_tokens, pl.completion_tokens, pl.total_tokens, pl.estimated_cost,
		       s.id as site_id, s.platform, pl.model_actual, pl.latency_ms, pl.created_at
		FROM proxy_logs pl
		LEFT JOIN accounts a ON pl.account_id = a.id
		LEFT JOIN sites s ON a.site_id = s.id
		WHERE pl.id > ?
		ORDER BY pl.id ASC
		LIMIT ?
	`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch projection batch: %w", err)
	}
	defer rows.Close()

	var result []projectionRow
	for rows.Next() {
		var r projectionRow
		if err := rows.Scan(
			&r.id, &r.status, &r.promptTokens, &r.completionTokens, &r.tokens, &r.cost,
			&r.siteID, &r.platform, &r.model, &r.latencyMs, &r.createdAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan projection row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate projection rows: %w", err)
	}
	return result, nil
}

// applyBatch flushes one fetched batch into the usage aggregation tables.
//
// Failure-recovery semantics: proxy_logs rows above the checkpoint watermark
// are the "dirty set" waiting to be projected. The aggregate upserts and the
// watermark advance run inside a single transaction, so a flush failure rolls
// both back together — no increment is lost and the watermark never moves
// past un-persisted rows. The next pass re-fetches the same id range and
// retries (the equivalent of octopus restoreStatsDirty putting the extracted
// ids back into the dirty set).
func (s *UsageAggregationScheduler) applyBatch(dbw *store.DB, cp projectionCheckpoint, rows []projectionRow) (orphanCount int, err error) {
	// Aggregate deltas by key so one batch never multiplies the same bucket
	// with repeated single-row upserts (correctness + fewer writes).
	type dayKey struct {
		day    string
		siteID int64
	}
	type hourKey struct {
		hour   string
		siteID int64
	}
	type modelKey struct {
		day    string
		siteID int64
		model  string
	}
	type bucketDelta struct {
		calls        int
		successes    int
		failures     int
		tokens       int64
		summarySpend float64
		siteSpend    float64
		modelSpend   float64
		latencyMs    int64
		latencyCount int
	}

	dayDeltas := make(map[dayKey]*bucketDelta)
	hourDeltas := make(map[hourKey]*bucketDelta)
	modelDeltas := make(map[modelKey]*bucketDelta)

	for _, r := range rows {
		if r.siteID == nil {
			// Orphan logs without a site join still advance the watermark below;
			// they cannot contribute to site/model aggregates.
			orphanCount++
			continue
		}
		logTime := projectionTimestamp(r.createdAt)
		day := logTime.Format("2006-01-02")
		hour := logTime.Truncate(time.Hour).Format(time.RFC3339)
		model := "unknown"
		if r.model != nil && strings.TrimSpace(*r.model) != "" {
			model = strings.TrimSpace(*r.model)
		}
		platform := ""
		if r.platform != nil {
			platform = *r.platform
		}

		tokens := effectiveTokenCount(r.tokens, r.promptTokens, r.completionTokens)
		summarySpend := resolveSummarySpend(r.cost, tokens, platform)
		siteSpend := resolveSiteSpend(r.cost, tokens, platform)
		modelSpend := resolveModelSpend(r.cost, tokens)

		successes := 0
		failures := 1
		if r.status != nil && *r.status == "success" {
			successes = 1
			failures = 0
		}
		latencyMs := int64(0)
		latencyCount := 0
		if r.latencyMs != nil && *r.latencyMs > 0 {
			latencyMs = *r.latencyMs
			latencyCount = 1
		}

		dk := dayKey{day: day, siteID: *r.siteID}
		dd := dayDeltas[dk]
		if dd == nil {
			dd = &bucketDelta{}
			dayDeltas[dk] = dd
		}
		dd.calls++
		dd.successes += successes
		dd.failures += failures
		dd.tokens += tokens
		dd.summarySpend += summarySpend
		dd.siteSpend += siteSpend
		dd.latencyMs += latencyMs
		dd.latencyCount += latencyCount

		hk := hourKey{hour: hour, siteID: *r.siteID}
		hd := hourDeltas[hk]
		if hd == nil {
			hd = &bucketDelta{}
			hourDeltas[hk] = hd
		}
		hd.calls++
		hd.successes += successes
		hd.failures += failures
		hd.tokens += tokens
		hd.summarySpend += summarySpend
		hd.siteSpend += siteSpend
		hd.latencyMs += latencyMs
		hd.latencyCount += latencyCount

		mk := modelKey{day: day, siteID: *r.siteID, model: model}
		md := modelDeltas[mk]
		if md == nil {
			md = &bucketDelta{}
			modelDeltas[mk] = md
		}
		md.calls++
		md.successes += successes
		md.failures += failures
		md.tokens += tokens
		md.modelSpend += modelSpend
		md.latencyMs += latencyMs
		md.latencyCount += latencyCount
	}

	// Apply to usage tables within a transaction for atomicity
	tx, err := dbw.Beginx()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // safe to call after Commit

	now := time.Now().UTC().Format(time.RFC3339)

	// Build dialect-aware INSERT queries once before loop.
	siteDaySQL := `INSERT INTO site_day_usage (local_day, site_id, total_calls, success_calls, failed_calls, total_tokens, total_summary_spend, total_site_spend, total_latency_ms, latency_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (local_day, site_id) DO UPDATE SET
			total_calls = site_day_usage.total_calls + excluded.total_calls,
			success_calls = site_day_usage.success_calls + excluded.success_calls,
			failed_calls = site_day_usage.failed_calls + excluded.failed_calls,
			total_tokens = site_day_usage.total_tokens + excluded.total_tokens,
			total_summary_spend = site_day_usage.total_summary_spend + excluded.total_summary_spend,
			total_site_spend = site_day_usage.total_site_spend + excluded.total_site_spend,
			total_latency_ms = site_day_usage.total_latency_ms + excluded.total_latency_ms,
			latency_count = site_day_usage.latency_count + excluded.latency_count,
			updated_at = excluded.updated_at`
	siteHourSQL := `INSERT INTO site_hour_usage (bucket_start_utc, site_id, total_calls, success_calls, failed_calls, total_tokens, total_summary_spend, total_site_spend, total_latency_ms, latency_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (bucket_start_utc, site_id) DO UPDATE SET
			total_calls = site_hour_usage.total_calls + excluded.total_calls,
			success_calls = site_hour_usage.success_calls + excluded.success_calls,
			failed_calls = site_hour_usage.failed_calls + excluded.failed_calls,
			total_tokens = site_hour_usage.total_tokens + excluded.total_tokens,
			total_summary_spend = site_hour_usage.total_summary_spend + excluded.total_summary_spend,
			total_site_spend = site_hour_usage.total_site_spend + excluded.total_site_spend,
			total_latency_ms = site_hour_usage.total_latency_ms + excluded.total_latency_ms,
			latency_count = site_hour_usage.latency_count + excluded.latency_count,
			updated_at = excluded.updated_at`
	modelDaySQL := `INSERT INTO model_day_usage (local_day, site_id, model, total_calls, success_calls, failed_calls, total_tokens, total_spend, total_latency_ms, latency_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (local_day, site_id, model) DO UPDATE SET
			total_calls = model_day_usage.total_calls + excluded.total_calls,
			success_calls = model_day_usage.success_calls + excluded.success_calls,
			failed_calls = model_day_usage.failed_calls + excluded.failed_calls,
			total_tokens = model_day_usage.total_tokens + excluded.total_tokens,
			total_spend = model_day_usage.total_spend + excluded.total_spend,
			total_latency_ms = model_day_usage.total_latency_ms + excluded.total_latency_ms,
			latency_count = model_day_usage.latency_count + excluded.latency_count,
			updated_at = excluded.updated_at`

	for k, d := range dayDeltas {
		if _, err := tx.Exec(tx.Rebind(siteDaySQL),
			k.day, k.siteID, d.calls, d.successes, d.failures, d.tokens, d.summarySpend, d.siteSpend, d.latencyMs, d.latencyCount, now, now); err != nil {
			return 0, fmt.Errorf("failed to upsert site_day_usage: %w", err)
		}
	}
	for k, d := range hourDeltas {
		if _, err := tx.Exec(tx.Rebind(siteHourSQL),
			k.hour, k.siteID, d.calls, d.successes, d.failures, d.tokens, d.summarySpend, d.siteSpend, d.latencyMs, d.latencyCount, now, now); err != nil {
			return 0, fmt.Errorf("failed to upsert site_hour_usage: %w", err)
		}
	}
	for k, d := range modelDeltas {
		if _, err := tx.Exec(tx.Rebind(modelDaySQL),
			k.day, k.siteID, k.model, d.calls, d.successes, d.failures, d.tokens, d.modelSpend, d.latencyMs, d.latencyCount, now, now); err != nil {
			return 0, fmt.Errorf("failed to upsert model_day_usage: %w", err)
		}
	}

	// Write checkpoint for every fetched row (including orphans) so the
	// watermark never re-projects the same proxy_log id range.
	if len(rows) > 0 {
		lastID := rows[len(rows)-1].id
		lastCreatedAt := time.Now().UTC().Format(time.RFC3339)
		if rows[len(rows)-1].createdAt != nil && strings.TrimSpace(*rows[len(rows)-1].createdAt) != "" {
			lastCreatedAt = strings.TrimSpace(*rows[len(rows)-1].createdAt)
		}
		checkpointResult, err := tx.Exec(tx.Rebind(`UPDATE analytics_projection_checkpoints
			SET last_proxy_log_id = ?, watermark_created_at = ?, last_projected_at = ?, last_successful_at = ?, updated_at = ?
			WHERE projector_key = ?`),
			lastID, lastCreatedAt, now, now, now, usageProjectorKey)
		if err != nil {
			return 0, fmt.Errorf("failed to update projection checkpoint: %w", err)
		}
		if affected, _ := checkpointResult.RowsAffected(); affected == 0 {
			// Committing aggregates without advancing the watermark would
			// re-project this batch on the next pass (double counting). Fail
			// the whole batch instead; tryAcquireLease re-creates a missing
			// checkpoint row on the next pass, so the retry converges.
			return 0, fmt.Errorf("projection checkpoint update affected 0 rows (projector_key %s)", usageProjectorKey)
		}
	}

	return orphanCount, tx.Commit()
}

// effectiveTokenCount prefers total_tokens when present and positive; otherwise
// falls back to prompt+completion so partial upstream payloads still count.
func effectiveTokenCount(total, prompt, completion *int64) int64 {
	var totalVal, promptVal, completionVal int64
	if total != nil && *total > 0 {
		totalVal = *total
	}
	if prompt != nil && *prompt > 0 {
		promptVal = *prompt
	}
	if completion != nil && *completion > 0 {
		completionVal = *completion
	}
	if totalVal > 0 {
		return totalVal
	}
	sum := promptVal + completionVal
	if sum > 0 {
		return sum
	}
	return 0
}

func fallbackTokenCost(tokens int64, platform string) float64 {
	if tokens <= 0 {
		return 0
	}
	// Veloera-style platforms bill roughly tokens/1e6; other NewAPI-like
	// platforms use the historical tokens/5e5 fallback from the TS service.
	if strings.EqualFold(strings.TrimSpace(platform), "veloera") {
		return float64(tokens) / 1_000_000
	}
	return float64(tokens) / 500_000
}

func resolveSummarySpend(explicit *float64, tokens int64, platform string) float64 {
	if explicit != nil && *explicit > 0 {
		return *explicit
	}
	return fallbackTokenCost(tokens, platform)
}

func resolveSiteSpend(explicit *float64, tokens int64, platform string) float64 {
	if explicit != nil && *explicit > 0 {
		return *explicit
	}
	return fallbackTokenCost(tokens, platform)
}

func resolveModelSpend(explicit *float64, tokens int64) float64 {
	if explicit != nil && *explicit > 0 {
		return *explicit
	}
	if tokens <= 0 {
		return 0
	}
	return float64(tokens) / 500_000
}

func projectionTimestamp(raw *string) time.Time {
	if raw != nil {
		value := strings.TrimSpace(*raw)
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Now().UTC()
}
