package store

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// PostgresPoolConfig is the application-side connection budget. MaxOpenConns
// must not exceed the production role CONNECTION LIMIT.
// ApplicationName is injected into the PostgreSQL DSN as application_name when
// the DSN does not already set it (visible in pg_stat_activity).
type PostgresPoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ApplicationName string
}

// DefaultPostgresPoolConfig returns the "normal" profile defaults (10/3).
// Large exclusive databases should raise MaxOpenConns explicitly or use
// DB_PROFILE=dedicated (20/5). Shared tiny roles use DB_PROFILE=shared-tiny (2/1).
func DefaultPostgresPoolConfig() PostgresPoolConfig {
	return PostgresPoolConfig{
		MaxOpenConns:    config.DefaultDbMaxOpenConnsNormal,
		MaxIdleConns:    config.DefaultDbMaxIdleConnsNormal,
		ConnMaxLifetime: time.Duration(config.DefaultDbConnMaxLifetimeSec) * time.Second,
		ConnMaxIdleTime: time.Duration(config.DefaultDbConnMaxIdleTimeSec) * time.Second,
	}
}

func normalizePostgresSSLMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return ""
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return ""
	}
}

func applyPostgresSSLMode(dsn string, mode string) string {
	if mode == "" {
		return dsn
	}
	trimmed := strings.TrimSpace(dsn)
	if strings.HasPrefix(strings.ToLower(trimmed), "postgres://") || strings.HasPrefix(strings.ToLower(trimmed), "postgresql://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			q := parsed.Query()
			q.Set("sslmode", mode)
			parsed.RawQuery = q.Encode()
			return parsed.String()
		}
	}

	if postgresKeywordSSLModeRE.MatchString(dsn) {
		return postgresKeywordSSLModeRE.ReplaceAllString(dsn, "${1}sslmode="+mode)
	}
	if strings.TrimSpace(dsn) == "" {
		return "sslmode=" + mode
	}
	return strings.TrimRight(dsn, " ") + " sslmode=" + mode
}

// applyPostgresApplicationName sets application_name when absent so operators can
// attribute connections in pg_stat_activity. Existing DSN values win.
func applyPostgresApplicationName(dsn string, appName string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return dsn
	}
	trimmed := strings.TrimSpace(dsn)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return dsn
		}
		q := parsed.Query()
		if strings.TrimSpace(q.Get("application_name")) != "" {
			return dsn
		}
		q.Set("application_name", appName)
		parsed.RawQuery = q.Encode()
		return parsed.String()
	}
	// keyword/value DSN
	if strings.Contains(lower, "application_name=") {
		return dsn
	}
	if strings.TrimSpace(dsn) == "" {
		return "application_name=" + appName
	}
	return strings.TrimRight(dsn, " ") + " application_name=" + appName
}

var postgresKeywordSSLModeRE = regexp.MustCompile(`(^|\s)sslmode=\S+`)

// configurePostgresPool sets connection pool defaults for PostgreSQL.
func configurePostgresPool(db *DB, pool PostgresPoolConfig) error {
	if pool.MaxOpenConns < 1 {
		return fmt.Errorf("max open connections must be >= 1")
	}
	if pool.MaxIdleConns < 0 || pool.MaxIdleConns > pool.MaxOpenConns {
		return fmt.Errorf("max idle connections must be between 0 and max open connections")
	}
	if pool.ConnMaxLifetime < 0 || pool.ConnMaxIdleTime < 0 {
		return fmt.Errorf("connection lifetimes must be >= 0")
	}
	db.SetMaxOpenConns(pool.MaxOpenConns)
	db.SetMaxIdleConns(pool.MaxIdleConns)
	db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	db.SetConnMaxIdleTime(pool.ConnMaxIdleTime)
	return nil
}
