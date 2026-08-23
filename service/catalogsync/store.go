// Package catalogsync owns the model-catalog data source registry: a
// DB-persisted, ordered list of catalog sources (llm-metadata / models.dev
// presets plus operator-added custom URLs) that are fetched in order and
// merged first-wins into the runtime catalog snapshot consumed by routing
// and the models marketplace.
package catalogsync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

const (
	// SettingsAutoSyncKey is the settings-table key for the automatic
	// catalog sync toggle ("true"/"false"; missing = enabled).
	SettingsAutoSyncKey = "catalog_auto_sync_enabled"

	// SourceTypeOfficial marks a preset official dataset source.
	SourceTypeOfficial = "official"
	// SourceTypeCustom marks an operator-added source.
	SourceTypeCustom = "custom"
)

// DefaultSourcePresets is the seed registry: llm-metadata (primary,
// native-provider filtered, daily rebuilt) followed by models.dev
// (fallback). A legacy PRICING_CATALOG_URL value that differs from both
// presets is inserted in front at seed time (see EnsureDefaults).
func DefaultSourcePresets() []pricingcatalog.SourceSpec {
	return []pricingcatalog.SourceSpec{
		{Name: "llm-metadata", URL: pricingcatalog.DefaultLLMMetadataURL, Kind: pricingcatalog.SourceKindLLMMetadata},
		{Name: "models.dev", URL: pricingcatalog.DefaultCatalogURL, Kind: pricingcatalog.SourceKindModelsDev},
	}
}

// Source is one persisted registry row (wire/domain shape).
type Source struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Enabled       bool       `json:"enabled"`
	Type          string     `json:"type"`
	SortOrder     int        `json:"sortOrder"`
	LastSuccessAt *time.Time `json:"lastSuccessAt"`
	LastError     *string    `json:"lastError"`
	LastCount     int        `json:"lastCount"`
	LastAttemptAt *time.Time `json:"lastAttemptAt"`
}

// sourceRow is the DB scan shape: TEXT timestamps are scanned as strings and
// parsed manually (modernc/sqlite does not auto-parse RFC3339 into time.Time).
type sourceRow struct {
	ID            int64   `db:"id"`
	Name          string  `db:"name"`
	URL           string  `db:"url"`
	Enabled       bool    `db:"enabled"`
	Type          string  `db:"type"`
	SortOrder     int     `db:"sort_order"`
	LastSuccessAt *string `db:"last_success_at"`
	LastError     *string `db:"last_error"`
	LastCount     int     `db:"last_count"`
	LastAttemptAt *string `db:"last_attempt_at"`
}

func (r sourceRow) toSource() Source {
	return Source{
		ID:            r.ID,
		Name:          r.Name,
		URL:           r.URL,
		Enabled:       r.Enabled,
		Type:          r.Type,
		SortOrder:     r.SortOrder,
		LastSuccessAt: parseTime(r.LastSuccessAt),
		LastError:     r.LastError,
		LastCount:     r.LastCount,
		LastAttemptAt: parseTime(r.LastAttemptAt),
	}
}

func parseTime(value *string) *time.Time {
	if value == nil {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil
	}
	return &parsed
}

// SourceInput is the create/update payload for a source row.
type SourceInput struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled *bool  `json:"enabled"`
	Type    string `json:"type"`
	// SortOrder, when >= 0, repositions the source (update only).
	SortOrder *int `json:"sortOrder"`
}

func (in SourceInput) validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("catalogsync: source name is required")
	}
	url := strings.TrimSpace(in.URL)
	if url == "" {
		return errors.New("catalogsync: source url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return errors.New("catalogsync: source url must be http(s)")
	}
	return nil
}

// Store is the DB registry for catalog sources. Manager embeds it so CRUD
// goes through one owner that also re-syncs the runtime provider.
type Store struct {
	db *sqlx.DB
}

// NewStore builds a registry store on the runtime DB.
func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

const sourceColumns = `id, name, url, enabled, type, sort_order, last_success_at, last_error, last_count, last_attempt_at`

// ListSources returns all registry rows in fetch order (sort_order, id).
func (s *Store) ListSources(ctx context.Context) ([]Source, error) {
	rows := []sourceRow{}
	query := s.db.Rebind(`SELECT ` + sourceColumns + ` FROM catalog_sources ORDER BY sort_order ASC, id ASC`)
	if err := s.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, fmt.Errorf("catalogsync: list sources: %w", err)
	}
	out := make([]Source, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toSource())
	}
	return out, nil
}

// EnsureDefaults seeds the registry with the preset sources when the table
// is empty. legacyURL (the PRICING_CATALOG_URL env value) is inserted as the
// top-priority custom source when it differs from both presets — this is the
// migration of the old single-URL configuration into the registry.
func (s *Store) EnsureDefaults(ctx context.Context, legacyURL string) error {
	count := 0
	if err := s.db.GetContext(ctx, &count, s.db.Rebind(`SELECT COUNT(*) FROM catalog_sources`)); err != nil {
		return fmt.Errorf("catalogsync: count sources: %w", err)
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	insert := `INSERT INTO catalog_sources (name, url, enabled, type, sort_order, last_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 0, ?, ?)`

	specs := DefaultSourcePresets()
	legacy := strings.TrimSpace(legacyURL)
	order := 0
	if legacy != "" && legacy != specs[0].URL && legacy != specs[1].URL {
		args := []any{"pricing-catalog (legacy env)", legacy, true, SourceTypeCustom, order, now, now}
		if _, err := s.db.ExecContext(ctx, s.db.Rebind(insert), args...); err != nil {
			return fmt.Errorf("catalogsync: seed legacy source: %w", err)
		}
		order++
	}
	for _, spec := range specs {
		args := []any{spec.Name, spec.URL, true, SourceTypeOfficial, order, now, now}
		if _, err := s.db.ExecContext(ctx, s.db.Rebind(insert), args...); err != nil {
			return fmt.Errorf("catalogsync: seed preset source %s: %w", spec.Name, err)
		}
		order++
	}
	return nil
}

// CreateSource inserts a new source at the end of the fetch order.
func (s *Store) CreateSource(ctx context.Context, in SourceInput) (Source, error) {
	if err := in.validate(); err != nil {
		return Source{}, err
	}
	if in.Type != SourceTypeOfficial {
		in.Type = SourceTypeCustom
	}
	enabled := in.Enabled == nil || *in.Enabled

	now := time.Now().UTC().Format(time.RFC3339)
	var maxOrder int
	if err := s.db.GetContext(ctx, &maxOrder, s.db.Rebind(`SELECT COALESCE(MAX(sort_order), -1) FROM catalog_sources`)); err != nil {
		return Source{}, fmt.Errorf("catalogsync: max sort_order: %w", err)
	}
	order := maxOrder + 1

	result, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO catalog_sources (name, url, enabled, type, sort_order, last_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`),
		strings.TrimSpace(in.Name), strings.TrimSpace(in.URL), enabled, in.Type, order, now, now)
	if err != nil {
		return Source{}, fmt.Errorf("catalogsync: create source: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Source{}, fmt.Errorf("catalogsync: create source id: %w", err)
	}
	return s.getSource(ctx, id)
}

// UpdateSource patches name/url/enabled/type/sortOrder of a source. Passing
// sortOrder repositions the row (other rows are shifted to keep the order
// contiguous).
func (s *Store) UpdateSource(ctx context.Context, id int64, in SourceInput) (Source, error) {
	existing, err := s.getSource(ctx, id)
	if err != nil {
		return existing, err
	}
	merged := SourceInput{
		Name:      existing.Name,
		URL:       existing.URL,
		Enabled:   &existing.Enabled,
		Type:      existing.Type,
		SortOrder: &existing.SortOrder,
	}
	if strings.TrimSpace(in.Name) != "" {
		merged.Name = strings.TrimSpace(in.Name)
	}
	if strings.TrimSpace(in.URL) != "" {
		merged.URL = strings.TrimSpace(in.URL)
	}
	if in.Enabled != nil {
		merged.Enabled = in.Enabled
	}
	if in.Type == SourceTypeOfficial || in.Type == SourceTypeCustom {
		merged.Type = in.Type
	}
	if in.SortOrder != nil {
		merged.SortOrder = in.SortOrder
	}
	if err := merged.validate(); err != nil {
		return existing, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if *merged.SortOrder != existing.SortOrder {
		if err := s.reposition(ctx, id, *merged.SortOrder, now); err != nil {
			return existing, err
		}
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE catalog_sources SET name = ?, url = ?, enabled = ?, type = ?, sort_order = ?, updated_at = ? WHERE id = ?`),
		strings.TrimSpace(merged.Name), strings.TrimSpace(merged.URL), *merged.Enabled, merged.Type, *merged.SortOrder, now, id)
	if err != nil {
		return existing, fmt.Errorf("catalogsync: update source: %w", err)
	}
	return s.getSource(ctx, id)
}

// reposition moves a row to the requested position, shifting the rows in
// between. position < 0 pins to the front, values beyond the tail pin to the
// end.
func (s *Store) reposition(ctx context.Context, id int64, position int, now string) error {
	rows, err := s.ListSources(ctx)
	if err != nil {
		return err
	}
	if position < 0 {
		position = 0
	}
	maxPos := len(rows) - 1
	if position > maxPos {
		position = maxPos
	}

	// Remove the moving row, then re-insert at position.
	ordered := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ID == id {
			continue
		}
		ordered = append(ordered, row.ID)
	}
	if position > len(ordered) {
		position = len(ordered)
	}
	ordered = append(ordered[:position], append([]int64{id}, ordered[position:]...)...)

	// Renumber contiguously so sort_order stays gap-free.
	for i, rowID := range ordered {
		_, err := s.db.ExecContext(ctx, s.db.Rebind(
			`UPDATE catalog_sources SET sort_order = ?, updated_at = ? WHERE id = ?`), i, now, rowID)
		if err != nil {
			return fmt.Errorf("catalogsync: reposition source: %w", err)
		}
	}
	return nil
}

// DeleteSource removes a registry row.
func (s *Store) DeleteSource(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM catalog_sources WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("catalogsync: delete source: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("catalogsync: delete source rows: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) getSource(ctx context.Context, id int64) (Source, error) {
	var row sourceRow
	err := s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT `+sourceColumns+` FROM catalog_sources WHERE id = ?`), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Source{}, sql.ErrNoRows
		}
		return Source{}, fmt.Errorf("catalogsync: get source: %w", err)
	}
	return row.toSource(), nil
}

// RecordStatus persists one source's sync outcome (success time / error /
// entry count). Successful outcomes clear the error column.
func (s *Store) RecordStatus(ctx context.Context, report pricingcatalog.SourceReport) error {
	if report.ID <= 0 {
		return nil // anonymous test sources are not persisted
	}
	now := time.Now().UTC()
	var lastSuccess, lastAttempt any
	if report.LastSuccess != nil {
		lastSuccess = report.LastSuccess.UTC().Format(time.RFC3339)
	}
	attemptedAt := report.AttemptedAt
	if attemptedAt.IsZero() {
		attemptedAt = now
	}
	lastAttempt = attemptedAt.UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE catalog_sources SET last_success_at = ?, last_error = ?, last_count = ?, last_attempt_at = ? WHERE id = ?`),
		lastSuccess, report.LastError, report.ModelCount, lastAttempt, report.ID)
	if err != nil {
		return fmt.Errorf("catalogsync: record source status: %w", err)
	}
	return nil
}

// AutoSyncEnabled reads the settings toggle (missing key = enabled).
func (s *Store) AutoSyncEnabled(ctx context.Context) (bool, error) {
	var raw string
	err := s.db.GetContext(ctx, &raw, s.db.Rebind(`SELECT value FROM settings WHERE key = ?`), SettingsAutoSyncKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, fmt.Errorf("catalogsync: read auto-sync setting: %w", err)
	}
	return !strings.EqualFold(strings.TrimSpace(raw), "false"), nil
}

// SetAutoSyncEnabled writes the settings toggle (upsert).
func (s *Store) SetAutoSyncEnabled(ctx context.Context, enabled bool) error {
	value := "true"
	if !enabled {
		value = "false"
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value`),
		SettingsAutoSyncKey, value)
	if err != nil {
		return fmt.Errorf("catalogsync: write auto-sync setting: %w", err)
	}
	return nil
}
