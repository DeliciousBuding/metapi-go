package store

import (
	"strings"
	"testing"
)

// TestDialectIsPG verifies PG dialect detection.
func TestDialectIsPG(t *testing.T) {
	if !isPG(DialectPostgres) {
		t.Error("isPG(postgres) should be true")
	}
	if isPG(DialectSQLite) {
		t.Error("isPG(sqlite) should be false")
	}
	if isPG("mysql") {
		t.Error("isPG(mysql) should be false")
	}
	if isPG("") {
		t.Error("isPG('') should be false")
	}
}

// TestResolveSQLitePathEmpty returns default hub.db.
func TestResolveSQLitePathEmpty(t *testing.T) {
	path := ResolveSQLitePath("", "/data")
	if !strings.HasSuffix(path, "hub.db") {
		t.Errorf("empty DB_URL should resolve to hub.db, got %q", path)
	}
}

// TestResolveSQLitePathMemory preserves :memory:.
func TestResolveSQLitePathMemory(t *testing.T) {
	path := ResolveSQLitePath(":memory:", "/data")
	if path != ":memory:" {
		t.Errorf(":memory: should stay :memory:, got %q", path)
	}
}

// TestResolveSQLitePathFilePrefix strips file:// prefix.
func TestResolveSQLitePathFilePrefix(t *testing.T) {
	path := ResolveSQLitePath("file:///tmp/test.db", "/data")
	// file:// prefix should be stripped.
	if strings.HasPrefix(path, "file://") {
		t.Errorf("file:// prefix should be stripped, got %q", path)
	}
}

// TestResolveSQLitePathSQLitePrefix handles sqlite:// prefix.
func TestResolveSQLitePathSQLitePrefix(t *testing.T) {
	// We can't easily test absolute path resolution, but we can verify
	// the prefix is stripped.
	path := ResolveSQLitePath("sqlite://mydb.sqlite", "/data")
	if strings.HasPrefix(path, "sqlite://") {
		t.Errorf("sqlite:// prefix should be stripped, got %q", path)
	}
	if !strings.HasSuffix(path, "mydb.sqlite") {
		t.Errorf("expected path ending with mydb.sqlite, got %q", path)
	}
}

// TestDialectNameConstants verifies the dialect constants.
func TestDialectNameConstants(t *testing.T) {
	if DialectSQLite != "sqlite" {
		t.Errorf("DialectSQLite: expected 'sqlite', got %q", DialectSQLite)
	}
	if DialectPostgres != "postgres" {
		t.Errorf("DialectPostgres: expected 'postgres', got %q", DialectPostgres)
	}
}

// TestOpenInvalidDialect returns error for unsupported dialect.
func TestOpenInvalidDialect(t *testing.T) {
	_, err := Open("mysql", ":memory:", false)
	if err == nil {
		t.Error("expected error for unsupported 'mysql' dialect")
	}
	if !strings.Contains(err.Error(), "unsupported dialect") {
		t.Errorf("error should mention 'unsupported dialect': %v", err)
	}
}

func TestApplyPostgresSSLModeURL(t *testing.T) {
	got := applyPostgresSSLMode("postgres://user:pass@example.com:5432/metapi", "verify-full")
	if !strings.Contains(got, "sslmode=verify-full") {
		t.Fatalf("expected sslmode=verify-full in %q", got)
	}
}

func TestApplyPostgresSSLModeReplacesExistingURLParam(t *testing.T) {
	got := applyPostgresSSLMode("postgres://user:pass@example.com:5432/metapi?sslmode=disable&connect_timeout=5", "require")
	if strings.Contains(got, "sslmode=disable") {
		t.Fatalf("old sslmode was not replaced: %q", got)
	}
	if strings.Count(got, "sslmode=") != 1 {
		t.Fatalf("expected exactly one sslmode parameter, got %q", got)
	}
	if !strings.Contains(got, "sslmode=require") || !strings.Contains(got, "connect_timeout=5") {
		t.Fatalf("expected sslmode=require and preserved connect_timeout, got %q", got)
	}
}

func TestApplyPostgresSSLModeKeywordDSN(t *testing.T) {
	got := applyPostgresSSLMode("host=localhost dbname=metapi sslmode=disable connect_timeout=5", "verify-ca")
	if got != "host=localhost dbname=metapi sslmode=verify-ca connect_timeout=5" {
		t.Fatalf("unexpected keyword DSN: %q", got)
	}
}

func TestApplyPostgresSSLModeKeywordDSNAppend(t *testing.T) {
	got := applyPostgresSSLMode("host=localhost dbname=metapi", "require")
	if got != "host=localhost dbname=metapi sslmode=require" {
		t.Fatalf("unexpected keyword DSN append: %q", got)
	}
}

func TestOpenWithPostgresSSLModeRejectsInvalidMode(t *testing.T) {
	_, err := OpenWithPostgresSSLMode(DialectPostgres, "postgres://example.invalid/metapi", "invalid")
	if err == nil {
		t.Fatal("expected invalid sslmode error")
	}
	if !strings.Contains(err.Error(), "unsupported postgres sslmode") {
		t.Fatalf("unexpected error: %v", err)
	}
}
