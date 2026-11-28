package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

const runtimeDatabaseConnectionTestTimeoutSec = "5"

// POST /api/settings/database/test-connection
func (h *databaseHandler) testConnection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dialect          string `json:"dialect"`
		ConnectionString string `json:"connectionString"`
		Ssl              bool   `json:"ssl"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	dialect, ok := normalizeRuntimeDatabaseDialect(body.Dialect)
	if !ok {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeInvalidDatabaseType, "database type must be sqlite or postgres")
		return
	}

	maskedConnection, err := testRuntimeDatabaseConnection(dialect, body.ConnectionString, body.Ssl)
	if err != nil {
		writeError(w, http.StatusBadRequest, "database connection test failed: "+sanitizeConnectionError(err, body.ConnectionString))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message":    "target database connection succeeded",
		"dialect":    dialect,
		"connection": maskedConnection,
	})
}

func testRuntimeDatabaseConnection(dialect string, connectionString string, ssl bool) (string, error) {
	trimmed := strings.TrimSpace(connectionString)
	if trimmed == "" {
		return "", fmt.Errorf("connection string is required")
	}

	dsn := trimmed
	if dialect == store.DialectSQLite {
		dsn = store.ResolveSQLitePath(trimmed, ".")
	} else if dialect == store.DialectPostgres {
		dsn = applyPostgresTestConnectTimeout(trimmed)
	}

	sslMode := ""
	if ssl {
		sslMode = "require"
	}
	db, err := store.OpenWithPostgresSSLMode(dialect, dsn, sslMode)
	if err != nil {
		return "", err
	}
	if err := db.Close(); err != nil {
		return "", err
	}
	return maskConnectionString(trimmed), nil
}

func applyPostgresTestConnectTimeout(dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if strings.Contains(strings.ToLower(trimmed), "connect_timeout=") {
		return dsn
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			q := parsed.Query()
			q.Set("connect_timeout", runtimeDatabaseConnectionTestTimeoutSec)
			parsed.RawQuery = q.Encode()
			return parsed.String()
		}
	}
	if trimmed == "" {
		return dsn
	}
	return strings.TrimRight(dsn, " ") + " connect_timeout=" + runtimeDatabaseConnectionTestTimeoutSec
}

func sanitizeConnectionError(err error, connectionString string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	raw := strings.TrimSpace(connectionString)
	if raw != "" {
		message = strings.ReplaceAll(message, raw, maskConnectionString(raw))
	}
	if at := strings.LastIndex(raw, "@"); at > 0 {
		for i := at - 1; i >= 0; i-- {
			if raw[i] == ':' {
				password := raw[i+1 : at]
				if password != "" {
					message = strings.ReplaceAll(message, password, "***")
				}
				break
			}
		}
	}
	return message
}

func maskConnectionString(conn string) string {
	if conn == "" {
		return ""
	}
	// Mask password in connection string
	// Simple approach: redact everything after @ for display
	if idx := strings.LastIndex(conn, "@"); idx >= 0 {
		return "****@" + conn[idx+1:]
	}
	return "****"
}
