package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// ---- B1: admin write-operation audit log ----

// Records every authenticated admin write (POST/PUT/PATCH/DELETE) into
// admin_audit_logs so operators can trace who changed what (and whether it
// succeeded). Best-effort by design: a failed insert logs a warning and never
// blocks the request. Read methods (GET/HEAD/OPTIONS) are not recorded to
// avoid log noise; authentication failures are already rejected by AdminAuth
// before this middleware runs.

// AuditMiddleware wraps admin handlers and records write operations.
// It must be mounted after AdminAuth so only authenticated requests pass.
// Panics inside the handler are recorded as status 500 before re-panicking,
// so chi's outer Recoverer still renders the standard error page.
func AuditMiddleware(db *sqlx.DB) func(http.Handler) http.Handler {
	if db == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			rec := &statusRecorder{ResponseWriter: w}
			func() {
				defer func() {
					if p := recover(); p != nil {
						recordAuditLog(db, r, http.StatusInternalServerError)
						panic(p) // re-panic: outer Recoverer renders the 500 page
					}
				}()
				next.ServeHTTP(rec, r)
			}()
			recordAuditLog(db, r, rec.status)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return // net/http ignores repeat calls; audit must record the first
	}
	r.wroteHeader = true
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Write records the implicit 200 when the handler never calls WriteHeader.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// actorFromToken derives a stable, non-reversible actor id from the admin
// bearer token: first 8 hex chars of its sha256 (collision-safe for
// identification, useless for token recovery).
func actorFromToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" || token == auth {
		return "unknown"
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:8]
}

// remoteIPFrom extracts the client IP (host part of RemoteAddr).
func remoteIPFrom(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// recordAuditLog writes one row; failures are logged, never propagated.
func recordAuditLog(db *sqlx.DB, r *http.Request, status int) {
	now := time.Now().UTC().Format(time.RFC3339)
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	_, err := db.Exec(
		db.Rebind(`INSERT INTO admin_audit_logs (actor, method, path, status, request_id, remote_ip, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		actorFromToken(r), r.Method, path, status,
		proxy.RequestIDFromContext(r.Context()), remoteIPFrom(r), now,
	)
	if err != nil {
		slog.Warn("admin audit: insert failed", "method", r.Method, "path", path, "error", err)
	}
}

// GET /api/admin/audit-logs?limit=&method=&path=
// Lists recent admin write operations newest-first. method filters exact
// methods (POST/PUT/PATCH/DELETE); path filters by substring.
func RegisterAuditLogsRoutes(r chi.Router, db *sqlx.DB) {
	h := &auditLogsHandler{db: db}
	r.Get("/api/admin/audit-logs", h.list)
}

type auditLogsHandler struct {
	db *sqlx.DB
}

func (h *auditLogsHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(getQueryInt(r, "limit", 50), 1, 200)
	method := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("method")))
	pathFilter := strings.TrimSpace(r.URL.Query().Get("path"))

	where := []string{"1 = 1"}
	args := []any{}
	if method != "" {
		where = append(where, "method = ?")
		args = append(args, method)
	}
	if pathFilter != "" {
		where = append(where, "path LIKE ?")
		args = append(args, "%"+pathFilter+"%")
	}

	var total int
	if err := h.db.Get(&total, "SELECT COUNT(*) FROM admin_audit_logs WHERE "+strings.Join(where, " AND "), args...); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to count audit logs"})
		return
	}

	rows := queryRows(h.db,
		"SELECT id, actor, method, path, status, request_id, remote_ip, created_at FROM admin_audit_logs WHERE "+
			strings.Join(where, " AND ")+" ORDER BY id DESC LIMIT ?",
		append(args, limit)...)

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"id":        coerceInt64(row["id"]),
			"actor":     coerceString(row["actor"]),
			"method":    coerceString(row["method"]),
			"path":      coerceString(row["path"]),
			"status":    coerceInt(row["status"]),
			"requestId": coerceString(row["requestId"]),
			"remoteIp":  coerceString(row["remoteIp"]),
			"createdAt": coerceString(row["createdAt"]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": total,
		"limit": limit,
	})
}
