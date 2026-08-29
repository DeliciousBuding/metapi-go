package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/internal/version"
	"github.com/deliciousbuding/metapi-go/router"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/web"
	"github.com/joho/godotenv"
)

func main() {
	// ---- Help / version flags (must run before config load so they work
	// without a valid AUTH_TOKEN/PROXY_TOKEN, which --help/--version must not require) ----
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

	// ---- 1a. Validate config at startup ----
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

	if err := bootstrapRuntime(cfg, rt); err != nil {
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
	// N7: apply operator-configured cache-ratio fallback overrides to routing.
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

func bootstrapRuntime(cfg *config.Config, rt *config.RuntimeSettings) (err error) {
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

	slog.Info("bootstrap complete")
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
