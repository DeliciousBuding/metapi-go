package backup

// The exported QueryTableAsJSON wrapper is gone: nothing in production called
// it, and its only caller was a test in the external test package, so the public
// surface existed to be tested rather than to be used. The guard behind it is
// still load-bearing -- a table name reaching queryTableAsJSON is interpolated
// into the statement, and IsKnownTable is the only thing between it and the SQL
// parser -- so it is proven here against the function production actually calls.

import (
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/jmoiron/sqlx"
)

func setupInternalBackupDB(t *testing.T) *sqlx.DB {
	t.Helper()

	return sqlitetest.Open(t).DB
}

func TestQueryTableAsJSONRejectsUnknownTable(t *testing.T) {
	db := setupInternalBackupDB(t)

	for _, table := range []string{
		"settings; DROP TABLE settings",
		"settings--",
		"settings WHERE 1=1",
		"not_a_table",
		"",
	} {
		rows, _, err := queryTableAsJSON(db, table, nil)
		if err == nil {
			t.Errorf("queryTableAsJSON(%q) returned %d rows and no error, want an unknown-table rejection", table, len(rows))
			continue
		}
		if !strings.Contains(err.Error(), "unknown table") {
			t.Errorf("queryTableAsJSON(%q) error = %v, want it to name the unknown table", table, err)
		}
	}
}

func TestQueryTableAsJSONReadsAKnownTable(t *testing.T) {
	db := setupInternalBackupDB(t)

	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('theme', '"dark"')`); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	rows, _, err := queryTableAsJSON(db, "settings", nil)
	if err != nil {
		t.Fatalf("queryTableAsJSON(settings) = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want the one seeded row", len(rows))
	}
}

// A single oversized cell must not abort the whole backup: the scheduled
// WebDAV backup would then fail forever until someone manually cleaned the
// row. The export truncates the cell, keeps going, and records a warning.
func TestBuildPayloadTruncatesOversizedCells(t *testing.T) {
	db := setupInternalBackupDB(t)
	oldMax := MaxExportCellBytes
	MaxExportCellBytes = 64
	t.Cleanup(func() { MaxExportCellBytes = oldMax })

	huge := strings.Repeat("x", 4096)
	if _, err := db.Exec(`INSERT INTO events (type, title, message, level, read, related_id, related_type, created_at)
		VALUES ('proxy', 't', ?, 'error', 0, 0, 'route', '2026-01-01T00:00:00Z')`, huge); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	payload, err := BuildPayload(db, "all")
	if err != nil {
		t.Fatalf("BuildPayload must not fail on an oversized cell: %v", err)
	}
	metadata, _ := payload["metadata"].(map[string]any)
	warnings, _ := metadata["warnings"].(map[string]int)
	if warnings["events.message"] != 1 {
		t.Fatalf("warnings = %#v, want events.message counted once", warnings)
	}
	tables, _ := payload["tables"].(map[string]any)
	rows, _ := tables["events"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("events rows = %d, want 1", len(rows))
	}
	message, _ := rows[0]["message"].(string)
	if len(message) > 64 {
		t.Fatalf("exported cell = %d bytes, want at most 64", len(message))
	}
	if !strings.Contains(message, "…[truncated by metapi: original 4096 bytes]") {
		t.Fatalf("truncation marker missing: %q", message)
	}
}
