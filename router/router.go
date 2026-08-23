package router

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/admin"
	"github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
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
	r.Use(BodyLimitPathAware(cfg.RequestBodyLimit, cfg.FileUploadLimitBytes))

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
		// Rate limiting: per-IP token bucket (configurable via ADMIN_RATE_LIMIT_*).
		// Fall back to defaults when unset (e.g. tests building Config{} directly).
		adminRps := cfg.AdminRateLimitRPS
		if adminRps <= 0 {
			adminRps = config.DefaultAdminRateLimitRPS
		}
		adminBurst := cfg.AdminRateLimitBurst
		if adminBurst <= 0 {
			adminBurst = config.DefaultAdminRateLimitBurst
		}
		r.Use(auth.AdminRateLimit(adminRps, adminBurst))
		// Stricter OAuth rate limit (configurable via OAUTH_RATE_LIMIT_*), only /api/oauth/*.
		oauthRps := cfg.OAuthRateLimitRPS
		if oauthRps <= 0 {
			oauthRps = config.DefaultOAuthRateLimitRPS
		}
		oauthBurst := cfg.OAuthRateLimitBurst
		if oauthBurst <= 0 {
			oauthBurst = config.DefaultOAuthRateLimitBurst
		}
		r.Use(auth.OAuthRateLimit(oauthRps, oauthBurst))
		// B1: audit admin write operations.
		if db := store.GetDB(); db != nil {
			r.Use(admin.AuditMiddleware(db.DB))
		}

		// /debug/vars moved behind admin auth
		r.Get("/api/debug/vars", app.MetricsHandler)

		// Build provenance for the About page. Registered outside the
		// db != nil block because every field comes from the linker or the
		// Go runtime, never the database.
		admin.RegisterAboutRoutes(r)

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
			// K1a: model-governance fix candidates (list + apply).
			admin.RegisterModelRedirectFixRoutes(r, db.DB)
			// Read-only multiplier/rate overview.
			admin.RegisterModelRatesRoutes(r, db.DB)
			// Model-catalog data source registry + manual/auto sync control.
			admin.RegisterCatalogSourceRoutes(r, db.DB)
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
			// Resin Tier 2 (#678): observability status endpoint for the
			// sticky-proxy-pool integration. Read-only surface.
			admin.RegisterResinRoutes(r, db.DB, cfg)
		} else {
			slog.Warn("router: database not initialized, P3 routes skipped")
		}

		// Monitor configuration routes (/api/monitor/*) stay inside this
		// Bearer-gated group. The cookie-authenticated LDOH iframe proxy
		// (/monitor-proxy/*) is registered outside the group below.
		if db := store.GetDB(); db != nil {
			admin.RegisterMonitorRoutes(r, db.DB, cfg)
		}

		r.Get("/api/desktop/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
	})

	// Wave 4 security handoff F1: the LDOH iframe proxy authenticates via the
	// HttpOnly meta_monitor_auth cookie — iframe sub-resource requests cannot
	// carry an Authorization header, so this surface must stay OUTSIDE the
	// Bearer AdminAuth group above (which answered 401 "Missing Authorization
	// header" and broke the iframe). The handler enforces its own cookie auth
	// (ensureMonitorAuth) and rejects ".." traversal paths (M1).
	if db := store.GetDB(); db != nil {
		admin.RegisterMonitorProxyRoutes(r, db.DB, cfg)
	}

	// /v1/* proxy routes → proxy rate limiting + proxy auth middleware.
	// Per-IP RPM limiter runs BEFORE auth so unauthenticated brute-force floods
	// are rejected without burning a DB lookup. The global-token cap runs AFTER
	// auth since it needs the resolved auth source to know whether the request
	// used the global PROXY_TOKEN (managed keys have their own RPM admission).
	r.Route("/v1", func(r chi.Router) {
		r.Use(CORS())
		r.Use(auth.ProxyRateLimit(cfg.ProxyRateLimitRPM))
		r.Use(auth.ProxyAuth(cfg))
		r.Use(auth.ProxyGlobalTokenRateLimit(cfg.ProxyGlobalTokenRPM))
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
	// Same rate-limiting stack as /v1: per-IP before auth, global-token after.
	r.Group(func(r chi.Router) {
		r.Use(CORS())
		r.Use(auth.ProxyRateLimit(cfg.ProxyRateLimitRPM))
		r.Use(auth.ProxyAuth(cfg))
		r.Use(auth.ProxyGlobalTokenRateLimit(cfg.ProxyGlobalTokenRPM))
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

	// /assets/* → legacy Vite asset layout, kept for compatibility with older
	// builds that still emit dist/assets.
	mountStaticSubdir(r, distFS, "assets", "/assets/*", "/assets/")

	// /static/* → Rsbuild output: /static/js/*.js (incl. async chunks),
	// /static/css/*.css and /static/font/*.woff2. index.html references these
	// paths; without this mount the SPA fallback answered 200 text/html and
	// nosniff browsers refused the assets (blank embedded UI).
	mountStaticSubdir(r, distFS, "static", "/static/*", "/static/")

	// Root public files (logo, favicons) copied from web/public into the dist
	// root. They are NOT under /assets/ or /static/, so serve them explicitly
	// here — otherwise the SPA fallback answers 200 text/html and <img>
	// renders blank.
	rootFiles := map[string]string{
		"logo.png":                  "image/png",
		"favicon.png":               "image/png",
		"favicon-64.png":            "image/png",
		"desktop-icon.png":          "image/png",
		"desktop-tray-template.png": "image/png",
		"logo.svg":                  "image/svg+xml",
		"favicon.svg":               "image/svg+xml",
	}
	for name, contentType := range rootFiles {
		fileName := name
		fileType := contentType
		r.Get("/"+fileName, withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, err := fs.ReadFile(distFS, fileName)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", fileType)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Write(data)
		})))
	}

	// SPA fallback: non-API paths → index.html; API → 404 JSON
	r.NotFound(withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})))
}

// mountStaticSubdir serves a subtree of the embedded dist under route with an
// immutable cache header (filenames are content-hashed).
//
// fs.Sub alone is not enough to detect a missing directory: embed.FS does not
// implement fs.SubFS, and the generic fallback hands back a broken empty
// subtree without an error. ReadDir here verifies the subtree is real so a
// wrong layout is logged at startup instead of silently answering every
// request with the SPA fallback HTML.
func mountStaticSubdir(r chi.Router, distFS fs.FS, dir, route, stripPrefix string) {
	subFS, err := fs.Sub(distFS, dir)
	if err != nil {
		slog.Warn("embedded web/dist subtree not available, serving disabled", "dir", dir, "error", err)
		return
	}
	if _, err := fs.ReadDir(subFS, "."); err != nil {
		slog.Warn("embedded web/dist subtree not readable, serving disabled", "dir", dir, "error", err)
		return
	}
	r.Handle(route, http.StripPrefix(stripPrefix, withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.FileServer(http.FS(subFS)).ServeHTTP(w, r)
	}))))
}
