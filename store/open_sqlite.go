package store

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveSQLitePath resolves the SQLite database file path.
// Mirrors TS index.ts resolveSqlitePath() behavior:
// - ":memory:" stays ":memory:"
// - empty DB_URL defaults to {DATA_DIR}/hub.db
// - "file://..." / "sqlite://..." prefixes are stripped
// - Other values are treated as file paths (resolved against cwd).
func ResolveSQLitePath(dbURL string, dataDir string) string {
	raw := strings.TrimSpace(dbURL)
	if raw == "" {
		return filepath.Join(dataDir, "hub.db")
	}
	if raw == ":memory:" {
		return raw
	}
	if strings.HasPrefix(raw, "file://") {
		path := raw[len("file://"):]
		decoded, err := decodeURIPath(path)
		if err == nil {
			return decoded
		}
		return path
	}
	if strings.HasPrefix(raw, "sqlite://") {
		rel := strings.TrimSpace(raw[len("sqlite://"):])
		abs, err := filepath.Abs(rel)
		if err != nil {
			return rel
		}
		return abs
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return raw
	}
	return abs
}

// decodeURIPath decodes percent-encoded characters in a URI path.
func decodeURIPath(path string) (string, error) {
	result := strings.Builder{}
	i := 0
	for i < len(path) {
		if path[i] == '%' && i+2 < len(path) {
			var b byte
			_, err := fmt.Sscanf(path[i:i+3], "%%%02x", &b)
			if err == nil {
				result.WriteByte(b)
				i += 3
				continue
			}
		}
		result.WriteByte(path[i])
		i++
	}
	return result.String(), nil
}

// applySQLitePragmas enables WAL mode, foreign key enforcement, and the
// runtime tuning pragmas for single-file SQLite databases.
//
// Correctness (fatal on failure):
//   - journal_mode=WAL: concurrency + crash safety
//   - foreign_keys=ON: FKs default to OFF in SQLite
//
// Performance/durability tuning (non-fatal, logged on failure):
//   - synchronous=NORMAL: crash-safe in WAL mode, skips fsync per commit to
//     cut write amplification (dev/test databases; Postgres is production)
//   - busy_timeout=5000: wait instead of failing on concurrent writers
//   - cache_size=10000: ~40MB page cache for faster reads on single-file DBs
func applySQLitePragmas(db *DB) error {
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("PRAGMA journal_mode=WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("PRAGMA foreign_keys=ON: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous = NORMAL"); err != nil {
		slog.Warn("store: failed to set SQLite synchronous=NORMAL", "error", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		slog.Warn("store: failed to set SQLite busy_timeout", "error", err)
	}
	if _, err := db.Exec("PRAGMA cache_size = 10000"); err != nil {
		slog.Warn("store: failed to set SQLite cache_size", "error", err)
	}
	return nil
}

// probeSQLiteWritable verifies, before SQLite opens the file, that the data
// directory (and an existing database file) are writable by the current
// process. The container image runs as a non-root user (uid 1001), so a Docker
// bind-mounted data directory owned by root produces cryptic failures later:
// "unable to open database file" on fresh installs and "attempt to write a
// readonly database" when migrating data created by the old root-running
// TypeScript container. Fail fast with an actionable fix instead.
func probeSQLiteWritable(dbPath string) error {
	dir := filepath.Dir(dbPath)
	// Owner-only directory: the database holds upstream credentials (audit L6).
	// MkdirAll applies the mode only to directories it creates, so an operator's
	// existing (possibly stricter) permissions are left untouched.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store: failed to create data directory %q: %w", dir, err)
	}

	// An existing database file (e.g. hub.db left behind by the TypeScript
	// container) must be writable, not just the directory.
	if _, err := os.Stat(dbPath); err == nil {
		file, openErr := os.OpenFile(dbPath, os.O_WRONLY, 0)
		if openErr != nil {
			return writableDataDirError(dir)
		}
		_ = file.Close()
	}

	// WAL mode creates sibling files (-wal/-shm), so the directory itself must
	// accept new files, not only the existing database file.
	probe, err := os.CreateTemp(dir, ".metapi-write-probe-*")
	if err != nil {
		return writableDataDirError(dir)
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return nil
}

func writableDataDirError(dir string) error {
	return fmt.Errorf(
		"store: data directory %q is not writable by the current process (uid %d). The metapi image runs as non-root uid 1001 — on the Docker host, make the mounted data directory writable by uid 1001, e.g. 'chown -R 1001:1001 <host-data-dir>' or 'chmod -R a+rwX <host-data-dir>'",
		dir, os.Getuid())
}

// ensureSQLiteFilePermissions creates a missing SQLite database file with
// owner-only permissions and strips group/other bits from an existing file
// (audit L6: the credential database was world-readable under a default umask).
//
// It never widens permissions: an existing stricter mode keeps its owner bits
// and loses only group/other access. Ownership is never changed. Platforms
// without Unix mode-bit semantics (Windows) are a no-op.
func ensureSQLiteFilePermissions(dbPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		file, createErr := os.OpenFile(dbPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if createErr != nil {
			return fmt.Errorf("store: failed to create SQLite file %q: %w", dbPath, createErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("store: failed to close SQLite file %q: %w", dbPath, closeErr)
		}
	} else if err != nil {
		return fmt.Errorf("store: failed to stat SQLite file %q: %w", dbPath, err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		return fmt.Errorf("store: failed to stat SQLite file %q: %w", dbPath, err)
	}
	mode := info.Mode().Perm()
	if mode&0o077 == 0 {
		return nil
	}
	if err := os.Chmod(dbPath, mode&^0o077); err != nil {
		return fmt.Errorf("store: failed to restrict SQLite file %q to owner-only access: %w (fix the file mode, e.g. 'chmod 600 <path>', and ensure the process owns it)", dbPath, err)
	}
	return nil
}
