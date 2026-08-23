package catalogsync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"github.com/deliciousbuding/metapi-go/store"
)

func setupCatalogDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestStore_EnsureDefaultsSeedsPresetsAndLegacyURL(t *testing.T) {
	db := setupCatalogDB(t)
	s := NewStore(db.DB)
	ctx := context.Background()

	if err := s.EnsureDefaults(ctx, ""); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	rows, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("sources = %d, want 3 presets", len(rows))
	}
	if rows[0].Name != "llm-metadata" || rows[0].Type != SourceTypeOfficial || !rows[0].Enabled {
		t.Errorf("first preset = %+v, want enabled official llm-metadata", rows[0])
	}
	if rows[1].Name != "models.dev" {
		t.Errorf("second preset = %q, want models.dev", rows[1].Name)
	}
	if rows[2].Name != "llm-metadata ratios" {
		t.Errorf("third preset = %q, want llm-metadata ratios", rows[2].Name)
	}

	// Idempotent: second call must not duplicate.
	if err := s.EnsureDefaults(ctx, ""); err != nil {
		t.Fatalf("EnsureDefaults (2nd): %v", err)
	}
	rows, _ = s.ListSources(ctx)
	if len(rows) != 3 {
		t.Fatalf("sources after 2nd seed = %d, want 3", len(rows))
	}

	// Legacy PRICING_CATALOG_URL value becomes the top custom source.
	db2 := setupCatalogDB(t)
	s2 := NewStore(db2.DB)
	if err := s2.EnsureDefaults(ctx, "https://internal.example.com/catalog.json"); err != nil {
		t.Fatalf("EnsureDefaults legacy: %v", err)
	}
	rows, err = s2.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources legacy: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("sources = %d, want 4 (legacy + 3 presets)", len(rows))
	}
	if rows[0].URL != "https://internal.example.com/catalog.json" || rows[0].Type != SourceTypeCustom {
		t.Errorf("legacy row = %+v, want top-priority custom source", rows[0])
	}

	// A legacy URL equal to a preset must dedup instead of duplicating.
	db3 := setupCatalogDB(t)
	s3 := NewStore(db3.DB)
	if err := s3.EnsureDefaults(ctx, "https://models.dev/api.json"); err != nil {
		t.Fatalf("EnsureDefaults dedup: %v", err)
	}
	rows, _ = s3.ListSources(ctx)
	if len(rows) != 3 {
		t.Fatalf("sources = %d, want 3 after URL dedup", len(rows))
	}
}

func TestStore_CRUDAndReposition(t *testing.T) {
	db := setupCatalogDB(t)
	s := NewStore(db.DB)
	ctx := context.Background()
	if err := s.EnsureDefaults(ctx, ""); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	created, err := s.CreateSource(ctx, SourceInput{Name: "custom-mirror", URL: "https://mirror.example.com/all.json"})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	if created.SortOrder != 3 || created.Type != SourceTypeCustom {
		t.Errorf("created = %+v, want order 3 custom (after 3 presets)", created)
	}

	// Update name + toggle enabled.
	enabled := false
	updated, err := s.UpdateSource(ctx, created.ID, SourceInput{Name: "renamed-mirror", Enabled: &enabled})
	if err != nil {
		t.Fatalf("UpdateSource: %v", err)
	}
	if updated.Name != "renamed-mirror" || updated.Enabled {
		t.Errorf("updated = %+v, want renamed + disabled", updated)
	}

	// Reposition to front.
	order := 0
	updated, err = s.UpdateSource(ctx, created.ID, SourceInput{SortOrder: &order})
	if err != nil {
		t.Fatalf("reposition: %v", err)
	}
	rows, _ := s.ListSources(ctx)
	if rows[0].ID != created.ID || rows[1].Name != "llm-metadata" || rows[2].Name != "models.dev" {
		names := make([]string, len(rows))
		for i, r := range rows {
			names[i] = r.Name
		}
		t.Errorf("order after reposition = %v, want [renamed-mirror llm-metadata models.dev ...]", names)
	}

	// Invalid URL rejected.
	if _, err := s.CreateSource(ctx, SourceInput{Name: "bad", URL: "ftp://nope"}); err == nil {
		t.Error("ftp URL must be rejected")
	}

	if err := s.DeleteSource(ctx, created.ID); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	rows, _ = s.ListSources(ctx)
	if len(rows) != 3 {
		t.Errorf("sources after delete = %d, want 3 (presets)", len(rows))
	}
}

func TestStore_StatusAndAutoSyncToggle(t *testing.T) {
	db := setupCatalogDB(t)
	s := NewStore(db.DB)
	ctx := context.Background()
	if err := s.EnsureDefaults(ctx, ""); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	rows, _ := s.ListSources(ctx)

	// Missing settings key → enabled by default.
	enabled, err := s.AutoSyncEnabled(ctx)
	if err != nil || !enabled {
		t.Errorf("AutoSyncEnabled default = %v/%v, want true/nil", enabled, err)
	}

	if err := s.SetAutoSyncEnabled(ctx, false); err != nil {
		t.Fatalf("SetAutoSyncEnabled: %v", err)
	}
	enabled, _ = s.AutoSyncEnabled(ctx)
	if enabled {
		t.Error("AutoSyncEnabled = true after disabling")
	}

	success := time.Now().UTC().Add(-time.Minute)
	if err := s.RecordStatus(ctx, newReport(rows[0].ID, 123, &success, "")); err != nil {
		t.Fatalf("RecordStatus: %v", err)
	}
	if err := s.RecordStatus(ctx, newReport(rows[1].ID, 0, nil, "boom")); err != nil {
		t.Fatalf("RecordStatus error case: %v", err)
	}
	rows, _ = s.ListSources(ctx)
	if rows[0].LastCount != 123 || rows[0].LastSuccessAt == nil {
		t.Errorf("row0 status = %+v, want count 123 + success time", rows[0])
	}
	if rows[1].LastError == nil || *rows[1].LastError != "boom" || rows[1].LastCount != 0 {
		t.Errorf("row1 status = %+v, want error boom count 0", rows[1])
	}
}

func newReport(id int64, count int, success *time.Time, err string) pricingcatalog.SourceReport {
	return pricingcatalog.SourceReport{
		ID:          id,
		ModelCount:  count,
		LastSuccess: success,
		LastError:   err,
		AttemptedAt: time.Now().UTC(),
	}
}

const mockCatalog = `{"openai":{"id":"openai","models":{"gpt-4o":{"id":"gpt-4o","cost":{"input":2.5,"output":10},"description":"mock gpt-4o","tool_call":true,"modalities":{"input":["text","image"],"output":["text"]}}}}}`

func TestManager_SyncAllAndSingleSourceMerge(t *testing.T) {
	db := setupCatalogDB(t)
	ctx := context.Background()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockCatalog))
	}))
	defer primary.Close()

	manager, err := NewManager(db.DB, Options{
		Interval:  time.Hour,
		LegacyURL: primary.URL, // seeds as top custom source above the presets
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Sync the legacy custom source only (presets are real internet URLs —
	// must not be fetched in this test).
	rows, _ := manager.ListSources(ctx)
	if len(rows) != 4 {
		t.Fatalf("sources = %d, want 4 (legacy + 3 presets)", len(rows))
	}
	status, err := manager.SyncNow(ctx, rows[0].ID)
	if err != nil {
		t.Fatalf("SyncNow single: %v", err)
	}
	if status.Snapshot.Models != 1 {
		t.Errorf("snapshot models = %d, want 1", status.Snapshot.Models)
	}
	if status.Sources[0].LastCount != 1 || status.Sources[0].LastSuccessAt == nil {
		t.Errorf("source status = %+v, want success with 1 model", status.Sources[0])
	}
	if status.IntervalMin != 60 {
		t.Errorf("intervalMin = %d, want 60", status.IntervalMin)
	}
	if !status.AutoSync {
		t.Error("autoSync default must be true")
	}

	entry, ok := manager.Snapshot().Lookup("gpt-4o")
	if !ok || entry.Description != "mock gpt-4o" {
		t.Errorf("snapshot gpt-4o = %+v ok=%v, want hydrated entry", entry, ok)
	}

	// Auto-sync toggle round-trips through settings.
	if err := manager.SetAutoSyncEnabled(ctx, false); err != nil {
		t.Fatalf("SetAutoSyncEnabled: %v", err)
	}
	status = manager.Status(ctx)
	if status.AutoSync {
		t.Error("status autoSync = true after disabling")
	}

	// Persisted status survives a manager rebuild (process restart shape).
	manager2, err := NewManager(db.DB, Options{Interval: time.Hour, LegacyURL: primary.URL})
	if err != nil {
		t.Fatalf("NewManager rebuild: %v", err)
	}
	status2 := manager2.Status(ctx)
	found := false
	for _, src := range status2.Sources {
		if src.ID == rows[0].ID && src.LastCount == 1 && src.LastSuccessAt != nil {
			found = true
		}
	}
	if !found {
		t.Errorf("persisted status not restored after rebuild: %+v", status2.Sources)
	}
	if status2.AutoSync {
		t.Error("autoSync toggle not restored after rebuild")
	}
}
