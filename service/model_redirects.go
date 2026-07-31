package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/tokendancelab/metapi-go/routing"
)

// ---- Model Name Redirects (all-api-hub borrow K1a) ----
//
// Maps a canonical route model name to the actual upstream name per account
// (e.g. claude-3-5-sonnet → claude-3-5-sonnet-20241022). Sync generation is
// best-effort and never overwrites manual mappings; see
// docs/analysis/competitive/k1-model-redirect-design-2026-08-01.md.

// dateSuffixRe matches a date or date+revision tail: -20241022, -20241022-v2.
var dateSuffixRe = regexp.MustCompile(`^-\d{8}(-v\d+)?$`)

// versionSuffixRe matches a generic dash-prefixed tail: -latest, -beta.
var versionSuffixRe = regexp.MustCompile(`^-[\w.]+$`)

// CanonicalFromActual finds the canonical candidate for an actual model name.
// Rule priority: exact (case-folded) → date suffix → version suffix. Returns
// ok=false when no candidate matches. candidates are case-folded canonical
// names; canonicalName is the original (unfolded) form matched against.
func CanonicalFromActual(actual string, candidates []string) (canonical string, ok bool) {
	actual = strings.TrimSpace(actual)
	if actual == "" {
		return "", false
	}
	folded := strings.ToLower(actual)
	for _, c := range candidates {
		cf := strings.ToLower(strings.TrimSpace(c))
		if cf == "" {
			continue
		}
		if cf == folded {
			// Exact match — no redirect needed.
			return c, true
		}
	}
	for _, c := range candidates {
		cf := strings.ToLower(strings.TrimSpace(c))
		if cf == "" {
			continue
		}
		if strings.HasPrefix(folded, cf) {
			tail := folded[len(cf):]
			if dateSuffixRe.MatchString(tail) || versionSuffixRe.MatchString(tail) {
				return c, true
			}
		}
	}
	return "", false
}

// loadRedirectCandidates collects canonical model names: global allowed
// models (settings) plus exact token_routes model patterns (no wildcards).
func loadRedirectCandidates(ctx context.Context, db *sqlx.DB) ([]string, error) {
	var raw string
	_ = db.GetContext(ctx, &raw, `SELECT value FROM settings WHERE key = 'global_allowed_models'`)
	var candidates []string
	if strings.TrimSpace(raw) != "" {
		var list []string
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			for _, part := range strings.Split(raw, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					list = append(list, part)
				}
			}
		}
		candidates = append(candidates, list...)
	}

	type routePattern struct {
		Pattern string `db:"model_pattern"`
	}
	var routes []routePattern
	if err := db.SelectContext(ctx, &routes, `SELECT model_pattern FROM token_routes WHERE enabled = ?`, true); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(candidates)+len(routes))
	out := make([]string, 0, len(candidates)+len(routes))
	appendUnique := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if strings.ContainsAny(name, "*?") {
			return // skip wildcard patterns
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	for _, c := range candidates {
		appendUnique(c)
	}
	for _, r := range routes {
		appendUnique(r.Pattern)
	}
	return out, nil
}

// GenerateModelRedirects sync-generates mappings for one account's actual
// model list. Best-effort: never overwrites manual rows, idempotent (upsert),
// updates last_seen_at for still-visible actual names. Returns the number of
// created mappings.
func GenerateModelRedirects(ctx context.Context, db *sqlx.DB, accountID int64, actualModels []string) (int, error) {
	if db == nil || accountID <= 0 || len(actualModels) == 0 {
		return 0, nil
	}
	candidates, err := loadRedirectCandidates(ctx, db)
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	created := 0
	for _, actual := range actualModels {
		canonical, ok := CanonicalFromActual(actual, candidates)
		if !ok || strings.EqualFold(canonical, strings.TrimSpace(actual)) {
			continue
		}
		// Manual rows are never touched by sync generation; an existing
		// mapping keeps its first-chosen actual (stability priority) and is
		// only last_seen-refreshed when that actual is still visible.
		var existing struct {
			Source string `db:"source"`
			Actual string `db:"actual"`
		}
		err := db.GetContext(ctx, &existing, db.Rebind(
			`SELECT source, actual FROM model_name_redirects WHERE account_id = ? AND canonical = ?`), accountID, canonical)
		if err == nil {
			if existing.Source == "manual" || existing.Actual != actual {
				continue
			}
			if _, err := db.ExecContext(ctx, db.Rebind(
				`UPDATE model_name_redirects SET last_seen_at = ?, updated_at = ? WHERE account_id = ? AND canonical = ?`),
				now, now, accountID, canonical); err != nil {
				slog.Warn("model-redirects: touch failed", "account_id", accountID, "canonical", canonical, "error", err)
			}
			continue
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO model_name_redirects (account_id, canonical, actual, source, last_seen_at, created_at, updated_at)
			VALUES (?, ?, ?, 'sync', ?, ?, ?)
			ON CONFLICT (account_id, canonical) DO UPDATE SET last_seen_at = excluded.last_seen_at, updated_at = excluded.updated_at`),
			accountID, canonical, actual, now, now, now); err != nil {
			slog.Warn("model-redirects: insert failed", "account_id", accountID, "canonical", canonical, "error", err)
			continue
		}
		created++
	}
	return created, nil
}

// RedirectFixCandidate is one disabled-model entry that a redirect can fix.
type RedirectFixCandidate struct {
	SiteID     int64  `db:"site_id" json:"siteId"`
	SiteName   string `db:"site_name" json:"siteName"`
	AccountID  int64  `db:"account_id" json:"accountId"`
	ModelName  string `db:"model_name" json:"modelName"`
	Canonical  string `db:"canonical" json:"canonical"`
	Actual     string `db:"actual" json:"actual"`
}

// ListRedirectFixCandidates finds site_disabled_models entries whose model is
// a redirect canonical with an actually-available upstream model (available=1
// in model_availability). These are safe to re-enable once confirmed.
func ListRedirectFixCandidates(ctx context.Context, db *sqlx.DB) ([]RedirectFixCandidate, error) {
	var out []RedirectFixCandidate
	err := db.SelectContext(ctx, &out, db.Rebind(`
		SELECT s.id AS site_id, s.name AS site_name,
			r.account_id AS account_id,
			sdm.model_name AS model_name,
			r.canonical AS canonical,
			r.actual AS actual
		FROM site_disabled_models sdm
		INNER JOIN model_name_redirects r ON r.canonical = sdm.model_name
		INNER JOIN accounts a ON a.id = r.account_id AND a.site_id = sdm.site_id
		INNER JOIN sites s ON s.id = sdm.site_id
		WHERE EXISTS (
			SELECT 1 FROM model_availability ma
			WHERE ma.account_id = r.account_id AND ma.model_name = r.actual AND ma.available = 1
		)
		ORDER BY s.name ASC, sdm.model_name ASC`))
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ApplyRedirectFixes removes the disabled entries that ListRedirectFixCandidates
// reports (only after operator confirmation). Returns the number removed.
func ApplyRedirectFixes(ctx context.Context, db *sqlx.DB, candidates []RedirectFixCandidate) (int, error) {
	removed := 0
	for _, c := range candidates {
		res, err := db.ExecContext(ctx, db.Rebind(
			`DELETE FROM site_disabled_models WHERE site_id = ? AND model_name = ?`), c.SiteID, c.ModelName)
		if err != nil {
			return removed, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed++
		}
	}
	return removed, nil
}

// ReloadRedirectRegistry (K1b) reloads the in-process redirect registry from
// model_name_redirects so the routing hot path sees the latest canonical→actual
// mappings. Call at startup and after any redirect mutation (sync generation,
// manual edits, deletes). Best-effort: on query failure it logs and leaves the
// previous registry intact.
func ReloadRedirectRegistry(ctx context.Context, db *sqlx.DB) {
	if db == nil {
		return
	}
	rows, err := db.QueryContext(ctx, `SELECT account_id, canonical, actual FROM model_name_redirects`)
	if err != nil {
		slog.Warn("reload redirect registry: query failed", "err", err)
		return
	}
	defer rows.Close()
	byAccount := make(map[int64]map[string]string)
	for rows.Next() {
		var accountID int64
		var canonical, actual string
		if err := rows.Scan(&accountID, &canonical, &actual); err != nil {
			slog.Warn("reload redirect registry: scan failed", "err", err)
			return
		}
		if byAccount[accountID] == nil {
			byAccount[accountID] = make(map[string]string)
		}
		byAccount[accountID][canonical] = actual
	}
	routing.SetModelRedirects(byAccount)
	slog.Debug("redirect registry reloaded", "accounts", len(byAccount))
}
