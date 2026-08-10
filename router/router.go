package router

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/admin"
	"github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// New creates and configures the Chi router with the full middleware stack,
// route groups, SPA fallback, and asset caching.
func New(cfg *config.Config, webFS embed.FS) chi.Router {
	r := chi.NewRouter()

	// ---- Middleware stack ----
	r.Use(WithRequestID)
	r.Use(SecurityHeaders)
	r.Use(TrustedRealIP(cfg))
	r.Use(RequestLogger)
	r.Use(Recoverer)
	r.Use(BodyLimit(cfg.RequestBodyLimit))

	// ---- /health and /ready (design addition, not in TS) ----
	// Registered before route groups so it bypasses auth middleware.
	r.With(CORS()).Get("/health", app.Health)
	r.With(CORS()).Get("/ready", app.Ready)

	// ---- /metrics (Prometheus text format, bypasses auth) ----
	r.With(CORS()).Get("/metrics", app.PrometheusHandler)

	// ---- /api/* routes (excluding public routes) → admin auth middleware ----

	// Admin route registrars use absolute /api/... paths so they can also be
	// tested on standalone routers. Keep this as a middleware group rather
	// than Route("/api"), otherwise production paths become /api/api/....
	r.Group(func(r chi.Router) {
		r.Use(AdminCORS(cfg))
		r.Use(auth.AdminAuth(cfg))
		// Rate limiting: per-IP token bucket (100 req/s, burst 200)
		r.Use(auth.AdminRateLimit(100, 200))
		// Stricter OAuth rate limit: 10 req/s, burst 20 (only /api/oauth/*)
		r.Use(auth.OAuthRateLimit(10, 20))
		// B1: audit admin write operations.
		if db := store.GetDB(); db != nil {
			r.Use(admin.AuditMiddleware(db.DB))
		}

		// /debug/vars moved behind admin auth
		r.Get("/api/debug/vars", app.MetricsHandler)

		// Sites + Accounts + AccountTokens CRUD API
		db := store.GetDB()
		if db != nil {
			// K1b: load the in-process redirect registry so routing eligibility
			// and forward rewriting see canonical→actual mappings from boot.
			service.ReloadRedirectRegistry(context.Background(), db.DB)
			admin.RegisterSitesRoutes(r, db.DB)
			admin.RegisterAccountsRoutes(r, db.DB, cfg)
			admin.RegisterAccountTokensRoutes(r, db.DB)

			// Admin API routes
			admin.RegisterStatsRoutes(r, db.DB)
			admin.RegisterSettingsRoutes(r, db.DB, cfg)
			admin.RegisterDatabaseRoutes(r, db.DB, cfg)
			admin.RegisterBackupRoutes(r, db.DB)
			admin.RegisterNotifyRoutes(r)
			admin.RegisterMaintenanceRoutes(r, db.DB)
			admin.RegisterDownstreamKeysRoutes(r, db.DB)
			admin.RegisterEventsRoutes(r, db.DB)
			admin.RegisterSearchRoutes(r, db.DB)
			admin.RegisterTasksRoutes(r, db.DB)
			// I1: accounts/sites global tag system.
			admin.RegisterTagsRoutes(r, db.DB)
			// H1: product risk banners.
			admin.RegisterAnnouncementsRoutes(r, db.DB)
			// K1a: model name redirects.
			admin.RegisterModelRedirectRoutes(r, db.DB)
			// Read-only multiplier/rate overview.
			admin.RegisterModelRatesRoutes(r, db.DB)
			// B1: admin write-operation audit log.
			admin.RegisterAuditLogsRoutes(r, db.DB)
			// C1: unified recurring-scheduler run history.
			admin.RegisterSchedulerStatusRoutes(r, db.DB)
			admin.RegisterTestRoutes(r, db.DB, cfg)
			admin.RegisterSiteAnnouncementsRoutes(r, db.DB)
			admin.RegisterAuthSettingsRoutes(r, db.DB, cfg)
			admin.RegisterCheckinRoutes(r, db.DB, cfg)
			admin.RegisterTokenRoutesWithDeps(r, db.DB, tokenRoutesDeps())
			admin.RegisterChannelTestRoutes(r, db.DB, cfg)
			admin.RegisterUpdateCenterRoutes(r)
			admin.RegisterOauthRoutes(r, db.DB)
		} else {
			slog.Warn("router: database not initialized, P3 routes skipped")
		}

		// Monitor routes (includes LDOH proxy outside /api)
		if db := store.GetDB(); db != nil {
			admin.RegisterMonitorRoutes(r, db.DB, cfg)
		}

		r.Get("/api/desktop/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
	})

	// /v1/* proxy routes → proxy auth middleware
	r.Route("/v1", func(r chi.Router) {
		r.Use(CORS())
		r.Use(auth.ProxyAuth(cfg))
		proxyhandler.RegisterProxyRoutes(r)
		// N2: downstream-key-visible cross-site price catalog (not admin auth).
		// Mounted under /v1 so it inherits ProxyAuth; reuses the admin
		// modelPriceCompare data surface (no separate catalog to drift).
		if db := store.GetDB(); db != nil {
			admin.RegisterDownstreamPricingRoutes(r, db.DB)
		}
	})

	// Non-/v1 proxy routes (chat alias, responses aliases, Gemini native paths).
	// Use an inline group instead of Route("/") so proxy auth only applies to
	// the exact registered proxy paths and does not shadow the SPA fallback.
	r.Group(func(r chi.Router) {
		r.Use(CORS())
		r.Use(auth.ProxyAuth(cfg))
		proxyhandler.RegisterNonV1ProxyRoutes(r)
	})

	// ---- SPA static file fallback ----
	setupSPAFallback(r, webFS)

	// B2: live ops WebSocket. Mounted after the
	// admin auth group because browser WS cannot send the Authorization
	// header — the endpoint verifies the token via ?token= itself.
	admin.RegisterOpsWSRoutes(r, cfg)

	return r
}

// tokenRoutesDeps bridges app-published TokenRouter/RouteDecisionService into
// admin handlers without creating an app↔admin import cycle.
// Only assign non-nil concrete pointers so interface fields stay truly nil
// (avoid typed-nil interface traps).
func tokenRoutesDeps() admin.TokenRoutesDeps {
	router, decisions := app.TokenRouteDecisionRuntime()
	deps := admin.TokenRoutesDeps{}
	if router != nil {
		deps.Router = router
	}
	if decisions != nil {
		deps.Decisions = decisions
	}
	return deps
}

// setupSPAFallback configures static asset serving and SPA fallback.
// distFS is the embedded frontend filesystem, rooted at web/dist/.
func setupSPAFallback(r chi.Router, distFS fs.FS) {
	if rootedFS, err := fs.Sub(distFS, "dist"); err == nil {
		distFS = rootedFS
	}

	// /assets/* → immutable cache for 1 year
	assetsFS, err := fs.Sub(distFS, "assets")
	if err == nil {
		r.Handle("/assets/*", http.StripPrefix("/assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.FileServer(http.FS(assetsFS)).ServeHTTP(w, r)
		})))
	} else {
		slog.Warn("embedded web/dist/assets not found, asset serving disabled", "error", err)
	}

	// Root public files (logo, favicons) copied by Vite from web/public into
	// dist root. They are NOT under /assets/, so serve them explicitly here —
	// otherwise the SPA fallback answers 200 text/html and <img> renders blank.
	rootFiles := map[string]string{
		"logo.png":                  "image/png",
		"favicon.png":               "image/png",
		"favicon-64.png":            "image/png",
		"desktop-icon.png":          "image/png",
		"desktop-tray-template.png": "image/png",
	}
	for name, contentType := range rootFiles {
		fileName := name
		fileType := contentType
		r.Get("/"+fileName, func(w http.ResponseWriter, r *http.Request) {
			data, err := fs.ReadFile(distFS, fileName)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", fileType)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Write(data)
		})
	}

	// SPA fallback: non-API paths → index.html; API → 404 JSON
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/v1/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Not found"}`))
			return
		}
		data, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Not found"}`))
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
}
