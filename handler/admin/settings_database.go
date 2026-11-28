package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterDatabaseRoutes registers all /api/settings/database routes.
func RegisterDatabaseRoutes(r chi.Router, db *store.DB, cfg *config.Config) {
	handler := &databaseHandler{db: db, cfg: cfg}

	r.Get("/api/settings/database/runtime", handler.getRuntime)
	r.Put("/api/settings/database/runtime", handler.saveRuntime)
	r.Post("/api/settings/database/test-connection", handler.testConnection)
	r.Post("/api/settings/database/migrate", handler.migrate)
}

type databaseHandler struct {
	db  *store.DB
	cfg *config.Config
}

type savedDatabaseConfig struct {
	Dialect                string `json:"dialect"`
	Connection             string `json:"connection"`
	ConnectionStringMasked string `json:"connectionStringMasked"`
	HasConnectionString    bool   `json:"hasConnectionString"`
	Ssl                    bool   `json:"ssl"`
	rawConnectionString    string
}

func normalizeRuntimeDatabaseDialect(dialect string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "sqlite":
		return "sqlite", true
	case "postgres", "postgresql":
		return "postgres", true
	default:
		return "", false
	}
}

func activeRuntimeDatabaseConfig(cfg *config.Config, rt *config.RuntimeSettings) map[string]any {
	if rt == nil {
		return map[string]any{
			"dialect":    store.DialectSQLite,
			"connection": "(default sqlite path)",
			"ssl":        false,
		}
	}

	dialect, ok := normalizeRuntimeDatabaseDialect(rt.DbType)
	if !ok {
		dialect = store.DialectSQLite
	}

	connection := strings.TrimSpace(rt.DbUrl)
	if dialect == store.DialectSQLite {
		if connection == "" {
			connection = "(default sqlite path)"
		}
	} else {
		connection = maskConnectionString(connection)
	}

	// SSL mode combines the static DB_SSLMODE env (on Config) with the
	// runtime-mutable DB_SSL flag (on the snapshot).
	ssl := false
	if cfg != nil {
		ssl = cfg.PostgresSSLMode(rt.DbSsl) != ""
	} else if rt.DbSsl {
		ssl = true
	}

	return map[string]any{
		"dialect":    dialect,
		"connection": connection,
		"ssl":        ssl,
	}
}

// GET /api/settings/database/runtime
func (h *databaseHandler) getRuntime(w http.ResponseWriter, r *http.Request) {
	saved, _ := loadSavedDatabaseConfig(h.db)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"active":          activeRuntimeDatabaseConfig(h.cfg, config.Runtime()),
		"saved":           saved,
		"restartRequired": savedDatabaseRequiresRestart(saved, h.cfg, config.Runtime()),
	})
}

// PUT /api/settings/database/runtime
func (h *databaseHandler) saveRuntime(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dialect          *string `json:"dialect"`
		ConnectionString *string `json:"connectionString"`
		Ssl              *bool   `json:"ssl"`
		Overwrite        *bool   `json:"overwrite"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if body.Dialect != nil {
		dialect, ok := normalizeRuntimeDatabaseDialect(*body.Dialect)
		if !ok {
			writeErrorCode(w, http.StatusBadRequest, ErrorCodeInvalidDatabaseType, "database type must be sqlite or postgres")
			return
		}
		upsertSettingDB(h.db, "db_type", dialect)
	}
	if body.ConnectionString != nil {
		upsertSettingDB(h.db, "db_url", strings.TrimSpace(*body.ConnectionString))
	}
	if body.Ssl != nil {
		upsertSettingDB(h.db, "db_ssl", *body.Ssl)
	}

	saved, _ := loadSavedDatabaseConfig(h.db)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"message":         "database runtime configuration saved",
		"active":          activeRuntimeDatabaseConfig(h.cfg, config.Runtime()),
		"saved":           saved,
		"restartRequired": savedDatabaseRequiresRestart(saved, h.cfg, config.Runtime()),
	})
}

func loadSavedDatabaseConfig(db *store.DB) (*savedDatabaseConfig, error) {
	values, err := store.NewSettingsStore(db).DatabaseConfigValues()
	if err != nil {
		return nil, err
	}
	dialect, _ := values["db_type"].(string)
	connection, _ := values["db_url"].(string)
	if dialect == "" || connection == "" {
		return nil, nil
	}
	ssl := parseRuntimeDatabaseSsl(values["db_ssl"])
	masked := maskConnectionString(connection)
	return &savedDatabaseConfig{
		Dialect:                dialect,
		Connection:             masked,
		ConnectionStringMasked: masked,
		HasConnectionString:    true,
		Ssl:                    ssl,
		rawConnectionString:    connection,
	}, nil
}

func savedDatabaseRequiresRestart(saved *savedDatabaseConfig, cfg *config.Config, rt *config.RuntimeSettings) bool {
	if saved == nil || rt == nil {
		return false
	}
	activeDialect, ok := normalizeRuntimeDatabaseDialect(rt.DbType)
	if !ok {
		activeDialect = store.DialectSQLite
	}
	if saved.Dialect != activeDialect {
		return true
	}
	if saved.Dialect == store.DialectSQLite {
		// SQLite connection strings appear in several equivalent spellings
		// (sqlite://, file://, bare path, default hub.db). Normalize both
		// sides so the same database does not demand a pointless restart.
		dataDir := ""
		if cfg != nil {
			dataDir = cfg.DataDir
		}
		if dataDir == "" {
			dataDir = config.DefaultDataDir
		}
		return store.ResolveSQLitePath(saved.rawConnectionString, dataDir) !=
			store.ResolveSQLitePath(rt.DbUrl, dataDir)
	}
	if strings.TrimSpace(saved.rawConnectionString) != strings.TrimSpace(rt.DbUrl) {
		return true
	}
	ssl := rt.DbSsl
	if cfg != nil {
		ssl = cfg.PostgresSSLMode(rt.DbSsl) != ""
	}
	return saved.Ssl != ssl
}

// parseRuntimeDatabaseSsl tolerates legacy string-encoded values ("true"/"1")
// in addition to the JSON bool written by the current save path.
func parseRuntimeDatabaseSsl(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return err == nil && parsed
	case float64:
		return v != 0
	}
	return false
}
