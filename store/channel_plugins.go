package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

var ErrChannelPluginConflict = errors.New("channel_plugin_revision_conflict")

type ChannelPlugin struct {
	Key      string `db:"plugin_key" json:"key"`
	Source   string `db:"source" json:"source"`
	Manifest string `db:"manifest" json:"-"`
	Revision int64  `db:"revision" json:"revision"`
	Enabled  bool   `db:"enabled" json:"enabled"`
}

func buildChannelPluginsDDL(d string) string {
	boolType := "INTEGER"
	if isPG(d) {
		boolType = "BOOLEAN"
	}
	return `CREATE TABLE IF NOT EXISTS channel_plugins (
		plugin_key TEXT PRIMARY KEY, source TEXT NOT NULL, manifest TEXT NOT NULL,
		revision BIGINT NOT NULL CHECK(revision > 0), enabled ` + boolType + ` NOT NULL
	)`
}

func LoadChannelPlugin(ctx context.Context, db sqlx.ExtContext, key string) (ChannelPlugin, error) {
	var out ChannelPlugin
	err := sqlx.GetContext(ctx, db, &out, db.Rebind(`SELECT plugin_key,source,manifest,revision,enabled FROM channel_plugins WHERE plugin_key=?`), key)
	return out, err
}

func ListChannelPlugins(ctx context.Context, db sqlx.ExtContext) ([]ChannelPlugin, error) {
	out := []ChannelPlugin{}
	err := sqlx.SelectContext(ctx, db, &out, `SELECT plugin_key,manifest,revision,enabled FROM channel_plugins ORDER BY plugin_key`)
	return out, err
}

// CAS also governs enabled changes. An administrator editing an old detail
// cannot accidentally re-enable or overwrite a newly upgraded program.
func SaveChannelPlugin(ctx context.Context, db sqlx.ExtContext, key, source, manifest string, enabled bool, expected int64) (ChannelPlugin, error) {
	var result sql.Result
	var err error
	if expected == 0 {
		result, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO channel_plugins(plugin_key,source,manifest,revision,enabled) VALUES(?,?,?,1,?) ON CONFLICT(plugin_key) DO NOTHING`), key, source, manifest, enabled)
	} else {
		result, err = db.ExecContext(ctx, db.Rebind(`UPDATE channel_plugins SET source=?,manifest=?,enabled=?,revision=revision+1 WHERE plugin_key=? AND revision=?`), source, manifest, enabled, key, expected)
	}
	if err != nil {
		return ChannelPlugin{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return ChannelPlugin{}, err
	}
	if n != 1 {
		return ChannelPlugin{}, ErrChannelPluginConflict
	}
	return ChannelPlugin{Key: key, Source: source, Manifest: manifest, Enabled: enabled, Revision: expected + 1}, nil
}

func DeleteChannelPlugin(ctx context.Context, db sqlx.ExtContext, key string, expected int64) error {
	result, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM channel_plugins WHERE plugin_key=? AND revision=? AND enabled=?`), key, expected, false)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrChannelPluginConflict
	}
	return nil
}

// Registry changes exclude concurrent channel writes while checking saved
// bindings. This is a DB lock, so multiple gateway processes share it too.
func LockChannelPluginBindings(ctx context.Context, tx *sqlx.Tx) error {
	query := `UPDATE upstream_channels SET id=id WHERE 1=0`
	if tx.DriverName() == "pgx" || tx.DriverName() == "postgres" {
		query = `LOCK TABLE upstream_channels,channel_plugins IN SHARE ROW EXCLUSIVE MODE`
	}
	_, err := tx.ExecContext(ctx, query)
	return err
}
