package admin

import (
	"net/http"
	"runtime"

	"github.com/deliciousbuding/metapi-go/internal/version"
	"github.com/go-chi/chi/v5"
)

// RegisterAboutRoutes registers GET /api/about, the build-provenance surface
// the About page renders.
//
// This is a dedicated endpoint rather than an extension of an existing one:
// /api/update-center/status is an explicit local-only stub whose contract is
// "no in-app version discovery" (its 0.0.0 placeholders are asserted by
// update_center_test.go), and /health + /ready are unauthenticated probes with
// a frozen payload that must not leak build provenance. Build metadata has no
// other owner, so it gets its own route.
//
// Takes no *sqlx.DB: every field comes from the linker or the Go runtime, so
// the endpoint keeps answering when the database is unavailable.
func RegisterAboutRoutes(r chi.Router) {
	r.Get("/api/about", aboutInfo)
}

// aboutInfo reports the build provenance of the running binary.
//
// `commit` and `buildTime` are empty for builds without ldflags injection
// (local `go build`, plain `docker build`). They stay empty on the wire — the
// frontend renders an em-dash for absent values, which is honest, whereas a
// synthesized SHA or a process-start timestamp would not be.
func aboutInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   version.Version,
		"commit":    version.Commit,
		"buildTime": version.BuildTime,
		"goVersion": runtime.Version(),
	})
}
