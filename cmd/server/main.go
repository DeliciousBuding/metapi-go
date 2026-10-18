package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/admintoken"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/internal/version"
	"github.com/deliciousbuding/metapi-go/router"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/web"
	"github.com/joho/godotenv"
)

func main() {
	// ---- Help / version flags (must run before config load so they work
	// without a valid AUTH_TOKEN, which --help/--version must not require) ----
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println(version.Version)
			os.Exit(0)
		case "--help", "-h":
			fmt.Println("Usage: metapi [command]")
			fmt.Println()
			fmt.Println("Commands:")
			fmt.Println("  (none)          Run the Metapi server")
			fmt.Println("  healthcheck     Run a health check and exit (for Docker HEALTHCHECK)")
			fmt.Println("  --version, -v   Print version and exit")
			fmt.Println("  --help, -h      Print this help and exit")
			os.Exit(0)
		}
	}

	// ---- Healthcheck subcommand (for Docker HEALTHCHECK without curl) ----
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	// ---- 0. Load.env file (silently skip if not found) ----
	_ = godotenv.Load()

	// Build env map from os.Environ()
	env := environMap()

	// ---- 1. Load config ----
	// Load splits env-driven values into the static Config (frozen at boot)
	// and the RuntimeSettings draft (published below, mutable afterwards only
	// through config.UpdateRuntime).
	cfg, rt := config.Load(env)

	// Configure the slog threshold from LOG_LEVEL before validation so a
	// raised threshold also quiets non-critical validation warnings. The
	// default text handler to stderr is preserved; only the level changes.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: config.SlogLevel(cfg.LogLevel),
	})))

	// ---- 1a. Generate the admin token when the operator did not supply one ----
	// Tokens are never operator-chosen by default: an unset or shipped-default
	// AUTH_TOKEN is replaced by a cryptographically random value, persisted at
	// bootstrap and handed over once through DATA_DIR/admin-token (0600). An
	// explicit strong AUTH_TOKEN still wins, and an explicit weak one fails
	// validation below instead of being silently replaced.
	generatedAuthToken := ""
	if strings.TrimSpace(env["AUTH_TOKEN"]) == "" || rt.AuthToken == config.DefaultAuthToken {
		token, err := admintoken.Generate()
		if err != nil {
			slog.Error("admin token generation failed", "error", err)
			os.Exit(1)
		}
		rt.AuthToken = token
		generatedAuthToken = token
	}

	// ---- 1b. Validate config at startup ----
	errs := append(cfg.Validate(), rt.Validate()...)
	hasCritical := false
	for _, err := range errs {
		if config.IsCritical(err) {
			slog.Error("config validation", "error", err)
			hasCritical = true
		} else {
			slog.Warn("config validation", "error", err)
		}
	}
	if hasCritical {
		os.Exit(1)
	}

	// Normalize DataDir (E11: trailing slash / Windows backslash)
	cfg.DataDir = filepath.Clean(cfg.DataDir)

	if err := bootstrapRuntime(cfg, rt, env["AUTH_TOKEN"], generatedAuthToken); err != nil {
		slog.Error("startup bootstrap failed", "error", err)
		os.Exit(1)
	}
	// Publish both singletons only after hydration: Config is frozen from
	// here on; RuntimeSettings stays hot-updatable via config.UpdateRuntime.
	config.Set(cfg)
	config.SetRuntime(rt)
	// One-time OAuth identity column backfill (bounded, marker-gated). Runs
	// before the server accepts traffic so the admin list endpoint never
	// pays the old per-request scan+update cost. See app.RunOauthIdentityBackfill.
	app.RunOauthIdentityBackfill()
	// Apply operator-configured cache-ratio fallback overrides to routing.
	app.ApplyCacheRatioOverrides()
	if err := app.ConfigureProxyUpstream(cfg); err != nil {
		slog.Error("proxy upstream wiring failed", "error", err)
		os.Exit(1)
	}

	// ---- 12. Create HTTP router ----
	r := router.New(cfg, web.Dist)

	// Override /health handler with actual implementation
	// (router.New registers a placeholder; in production we'd pass the handler in)
	// For now, the router.New already registers a valid /health handler.

	// ---- 17. Start background services (stubs) ----
	app.StartBackgroundServices()

	// ---- 18. Start pprof debug server (port 6060, only with -tags debug) ----
	app.StartDebugServer(6060)

	// ---- 20. Register onClose hooks ----
	a := app.New(cfg, r)
	a.RegisterOnClose(func() {
		app.StopBackgroundServices()
		// Stop the models.dev catalog refresh loop before the store closes.
		app.ShutdownPricingCatalog()
		// Drain the proxy_log batch writer before app.cleanup() calls
		// store.CloseDatabase() so the final batch flushes against an
		// still-open DB. Bounded wait so a stuck flush cannot block shutdown.
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.ShutdownProxyLogBatchWriter(drainCtx); err != nil {
			slog.Warn("proxy_log batch writer drain did not complete within timeout", "error", err)
		}
	})

	// ---- 21-23. Listen + shutdown ----
	if err := a.Start(); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func bootstrapRuntime(cfg *config.Config, rt *config.RuntimeSettings, envAuthToken, generatedAuthToken string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("startup bootstrap panicked: %v", r)
		}
	}()

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data directory %q: %w", cfg.DataDir, err)
	}
	if err := store.EnsureRuntimeDatabase(cfg, rt); err != nil {
		return fmt.Errorf("ensure runtime database: %w", err)
	}
	if err := store.LoadRuntimeSettings(cfg, rt); err != nil {
		return fmt.Errorf("load runtime settings: %w", err)
	}
	// Hydration may have replaced the env token with the settings row; the
	// earlier Validate only saw the env value. Re-check the downgrade risk
	// before any listener starts.
	if err := validateHydratedAuthToken(rt, envAuthToken); err != nil {
		return err
	}
	// Persist a freshly generated token only when the settings table did not
	// already carry one (hydration would have replaced rt.AuthToken). Writing
	// it here means the next boot authenticates with the same token without
	// requiring the operator to edit the environment.
	if generatedAuthToken != "" && rt.AuthToken == generatedAuthToken {
		db := store.GetDB()
		if db == nil {
			return fmt.Errorf("persist generated admin token: database unavailable")
		}
		if err := store.NewSettingsStore(db).Set("auth_token", strconv.Quote(generatedAuthToken)); err != nil {
			return fmt.Errorf("persist generated admin token: %w", err)
		}
		path, writeErr := admintoken.WriteFile(cfg.DataDir, generatedAuthToken)
		if writeErr != nil {
			slog.Warn("generated admin token could not be written to disk", "error", writeErr)
		} else {
			slog.Warn("AUTH_TOKEN is not set; generated a random admin token",
				"path", path,
				"hint", "read it once, store it in your secret manager, and remove the file")
		}
	}

	slog.Info("bootstrap complete")
	return nil
}

// validateHydratedAuthToken re-checks the admin token after the settings table
// has overridden the env value. config.Validate ran against the env token only,
// so a migrated or tampered database could silently downgrade a strong token to
// the built-in default. Only auth_token is re-checked here: running the full
// runtime snapshot validation again would let unrelated persisted warnings
// block boot. Error and warning text never include a token value.
func validateHydratedAuthToken(rt *config.RuntimeSettings, envAuthToken string) error {
	if rt == nil {
		return fmt.Errorf("refusing to start: runtime settings are unavailable")
	}
	if rt.AuthToken == config.DefaultAuthToken {
		return fmt.Errorf("refusing to start: persisted auth_token resolves to the built-in default; rotate the admin token and restart")
	}
	// config.Validate only rejects the built-in default. Hydration can replace
	// the env value with any persisted string, so a tampered or migrated
	// database could still install a trivially brute-forceable token. Reject
	// the same weak class here regardless of which source produced it.
	if strings.TrimSpace(rt.AuthToken) == "" {
		return fmt.Errorf("refusing to start: admin auth_token is empty")
	}
	if len(rt.AuthToken) < 8 {
		return fmt.Errorf("refusing to start: admin auth_token is shorter than 8 bytes; use a 32+ byte random token")
	}
	if len(rt.AuthToken) < 16 {
		slog.Warn("security: admin auth_token is shorter than 16 bytes; use a 32+ byte random token")
	}
	if envAuthToken != "" && envAuthToken != rt.AuthToken {
		slog.Warn("settings: persisted auth_token overrides AUTH_TOKEN from the environment; serving with the database value")
	}
	return nil
}

func runHealthcheck() int {
	target := os.Getenv("METAPI_HEALTHCHECK_URL")
	if target == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "4000"
		}
		path := os.Getenv("METAPI_HEALTHCHECK_PATH")
		if path == "" {
			path = "/ready"
		}
		target = "http://127.0.0.1:" + port + path
	}

	client := http.Client{Timeout: 5 * time.Second, Transport: httpclient.SharedTransport()}
	resp, err := client.Get(target)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0
	}
	return 1
}

// environMap converts os.Environ() to a map[string]string.
func environMap() map[string]string {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				env[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return env
}
