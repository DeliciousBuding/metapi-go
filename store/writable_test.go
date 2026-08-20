package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeSQLiteWritableWritableDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hub.db")
	if err := probeSQLiteWritable(dbPath); err != nil {
		t.Fatalf("probeSQLiteWritable(writable dir) = %v, want nil", err)
	}
	// The probe must not create the database file itself.
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("probe created the database file: %v", err)
	}
}

func TestProbeSQLiteWritableExistingWritableFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hub.db")
	if err := os.WriteFile(dbPath, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := probeSQLiteWritable(dbPath); err != nil {
		t.Fatalf("probeSQLiteWritable(existing writable file) = %v, want nil", err)
	}
}

func TestProbeSQLiteWritableNonWritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := probeSQLiteWritable(filepath.Join(dir, "hub.db"))
	if err == nil {
		t.Fatal("probeSQLiteWritable(non-writable dir) = nil, want error")
	}
	if !strings.Contains(err.Error(), "chown") {
		t.Fatalf("error should carry the chown hint, got: %v", err)
	}
}

func TestProbeSQLiteWritableNonWritableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hub.db")
	if err := os.WriteFile(dbPath, []byte("stub"), 0o444); err != nil {
		t.Fatal(err)
	}

	err := probeSQLiteWritable(dbPath)
	if err == nil {
		t.Fatal("probeSQLiteWritable(non-writable file) = nil, want error")
	}
	if !strings.Contains(err.Error(), "chown") {
		t.Fatalf("error should carry the chown hint, got: %v", err)
	}
}
