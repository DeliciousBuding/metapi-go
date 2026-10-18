package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestRunHealthcheckUsesConfiguredURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Fatalf("healthcheck path = %q, want /ready", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	t.Setenv("METAPI_HEALTHCHECK_URL", ts.URL+"/ready")
	if code := runHealthcheck(); code != 0 {
		t.Fatalf("runHealthcheck healthy exit = %d, want 0", code)
	}
}

func TestRunHealthcheckFailsOnUnreadyStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	t.Setenv("METAPI_HEALTHCHECK_URL", ts.URL+"/ready")
	if code := runHealthcheck(); code != 1 {
		t.Fatalf("runHealthcheck unready exit = %d, want 1", code)
	}
}

func TestBootstrapRuntimeFailsOnUnavailablePostgres(t *testing.T) {
	_ = store.CloseDatabase()
	t.Cleanup(func() { _ = store.CloseDatabase() })

	cfg, rt := config.Load(map[string]string{
		"DB_TYPE":      "postgres",
		"DATABASE_URL": "postgres://user:pass@127.0.0.1:1/metapi?sslmode=disable&connect_timeout=1",
		"DATA_DIR":     t.TempDir(),
	})

	err := bootstrapRuntime(cfg, rt, "")
	if err == nil {
		t.Fatal("bootstrapRuntime succeeded with unavailable PostgreSQL")
	}
	if !strings.Contains(err.Error(), "ensure runtime database") {
		t.Fatalf("bootstrapRuntime error = %v, want ensure runtime database context", err)
	}
	if !strings.Contains(err.Error(), "failed to ping postgres database") {
		t.Fatalf("bootstrapRuntime error = %v, want PostgreSQL connection failure", err)
	}
}

func TestStartupValidationRejectsDefaultTokens(t *testing.T) {
	cfg, rt := config.Load(map[string]string{})

	// Admin token checks live on the runtime snapshot. Downstream keys are
	// created or migrated after the database opens.
	var critical []string
	for _, err := range append(cfg.Validate(), rt.Validate()...) {
		if config.IsCritical(err) {
			critical = append(critical, err.Error())
		}
	}
	got := strings.Join(critical, "\n")
	if !strings.Contains(got, "auth_token") {
		t.Fatalf("startup validation critical errors = %q, want auth_token", got)
	}
	if strings.Contains(got, "proxy_token") {
		t.Fatalf("startup incorrectly requires a retired root token: %q", got)
	}
}

func TestBootstrapRuntimeInitializesSQLite(t *testing.T) {
	_ = store.CloseDatabase()

	dataDir := t.TempDir()
	t.Cleanup(func() { _ = store.CloseDatabase() })
	cfg, rt := config.Load(map[string]string{
		"DB_TYPE":    "sqlite",
		"DB_URL":     filepath.Join(dataDir, "bootstrap.db"),
		"DATA_DIR":   dataDir,
		"AUTH_TOKEN": "bootstrap-test-admin-token",
	})

	if err := bootstrapRuntime(cfg, rt, "bootstrap-test-admin-token"); err != nil {
		t.Fatalf("bootstrapRuntime sqlite: %v", err)
	}
	if db := store.GetDB(); db == nil {
		t.Fatal("bootstrapRuntime did not initialize active DB")
	}
}

// seedPersistedAuthToken writes a settings row the way the admin rotation path
// stores it (JSON-encoded string) into a fresh SQLite file.
func seedPersistedAuthToken(t *testing.T, dbPath, token string) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, dbPath, false)
	if err != nil {
		t.Fatalf("open seed database: %v", err)
	}
	defer db.Close()
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate seed database: %v", err)
	}
	if err := store.NewSettingsStore(db).Set("auth_token", `"`+token+`"`); err != nil {
		t.Fatalf("seed auth_token: %v", err)
	}
}

// M8: hydration runs after the env-based validation, so a database that still
// carries the built-in default must abort startup instead of silently
// downgrading a strong AUTH_TOKEN.
func TestBootstrapRuntimeRejectsPersistedDefaultAuthToken(t *testing.T) {
	_ = store.CloseDatabase()

	dataDir := t.TempDir()
	// Registered after TempDir so the database closes first (LIFO), letting
	// Windows remove the temp directory.
	t.Cleanup(func() { _ = store.CloseDatabase() })
	dbPath := filepath.Join(dataDir, "bootstrap.db")
	seedPersistedAuthToken(t, dbPath, config.DefaultAuthToken)

	cfg, rt := config.Load(map[string]string{
		"DB_TYPE":    "sqlite",
		"DB_URL":     dbPath,
		"DATA_DIR":   dataDir,
		"AUTH_TOKEN": "strong-env-admin-token",
	})
	err := bootstrapRuntime(cfg, rt, "strong-env-admin-token")
	if err == nil {
		t.Fatal("bootstrapRuntime accepted a persisted default auth_token")
	}
	if !strings.Contains(err.Error(), "auth_token") {
		t.Fatalf("bootstrapRuntime error = %v, want auth_token context", err)
	}
	if strings.Contains(err.Error(), config.DefaultAuthToken) {
		t.Fatalf("bootstrapRuntime error leaks the token value: %v", err)
	}
}

// A strong persisted token still wins over env (existing rotation behavior),
// and the conflict is reported without either value.
func TestBootstrapRuntimeUsesPersistedAuthTokenAndWarns(t *testing.T) {
	_ = store.CloseDatabase()

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	dataDir := t.TempDir()
	t.Cleanup(func() { _ = store.CloseDatabase() })
	dbPath := filepath.Join(dataDir, "bootstrap.db")
	const persisted = "strong-persisted-admin-token"
	const envToken = "strong-env-admin-token"
	seedPersistedAuthToken(t, dbPath, persisted)

	cfg, rt := config.Load(map[string]string{
		"DB_TYPE":    "sqlite",
		"DB_URL":     dbPath,
		"DATA_DIR":   dataDir,
		"AUTH_TOKEN": envToken,
	})
	if err := bootstrapRuntime(cfg, rt, envToken); err != nil {
		t.Fatalf("bootstrapRuntime: %v", err)
	}
	if rt.AuthToken != persisted {
		t.Fatalf("AuthToken = %q, want the persisted rotated token", rt.AuthToken)
	}
	output := logs.String()
	if !strings.Contains(output, "persisted auth_token overrides AUTH_TOKEN") {
		t.Fatalf("startup log missing the override warning: %s", output)
	}
	if strings.Contains(output, persisted) || strings.Contains(output, envToken) {
		t.Fatalf("startup log leaks a token value: %s", output)
	}
}
