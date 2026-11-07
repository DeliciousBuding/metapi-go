package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/jmoiron/sqlx"
)

// Only programs actually captured for persistent operations enter this archive.
// There is no independent revision axis: identity is the executable fingerprint.
type ChannelPluginVersion struct {
	Key         string `db:"plugin_key"`
	Fingerprint string `db:"fingerprint"`
	Source      string `db:"source"`
	Manifest    string `db:"manifest"`
}

func (ChannelPluginVersion) String() string { return "ChannelPluginVersion([REDACTED])" }

func buildChannelPluginVersionsDDL(string) string {
	return `CREATE TABLE IF NOT EXISTS channel_plugin_versions (
		fingerprint TEXT PRIMARY KEY, plugin_key TEXT NOT NULL,
		source TEXT NOT NULL, manifest TEXT NOT NULL
	)`
}

func LoadChannelPluginVersion(ctx context.Context, db sqlx.ExtContext, key, fingerprint string) (ChannelPluginVersion, error) {
	var row ChannelPluginVersion
	err := sqlx.GetContext(ctx, db, &row, db.Rebind(`SELECT plugin_key,fingerprint,source,manifest FROM channel_plugin_versions WHERE plugin_key=? AND fingerprint=?`), key, fingerprint)
	return row, err
}

// Caller holds LockChannelPluginBindings in the same transaction. Archive rows
// are immutable; an already-captured version is not counted or rewritten again.
func ArchiveChannelPluginVersion(ctx context.Context, tx *sqlx.Tx, row ChannelPluginVersion) error {
	digest := sha256.Sum256([]byte(row.Source + "\x00" + row.Manifest))
	if !contract.ValidKey(row.Key) || !contract.ValidFingerprint(row.Fingerprint) || hex.EncodeToString(digest[:]) != row.Fingerprint || len(row.Source) == 0 || len(row.Source) > 256<<10 || len(row.Manifest) > 256<<10 {
		return fmt.Errorf("invalid_channel_plugin_version")
	}
	var existing int
	if err := tx.GetContext(ctx, &existing, tx.Rebind(`SELECT COUNT(*) FROM channel_plugin_versions WHERE fingerprint=?`), row.Fingerprint); err != nil {
		return err
	}
	if existing != 0 {
		stored, err := LoadChannelPluginVersion(ctx, tx, row.Key, row.Fingerprint)
		if err != nil || stored.Source != row.Source || stored.Manifest != row.Manifest {
			return fmt.Errorf("invalid_channel_plugin_version")
		}
		return nil
	}
	var total, perKey int
	if err := tx.GetContext(ctx, &total, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil {
		return err
	}
	if err := tx.GetContext(ctx, &perKey, tx.Rebind(`SELECT COUNT(*) FROM channel_plugin_versions WHERE plugin_key=?`), row.Key); err != nil {
		return err
	}
	// This bounds durable executable storage, not user tasks or plugin upgrades.
	// Reaching it rejects new captures before any billable upstream submission.
	if total >= 512 || perKey >= 32 {
		return fmt.Errorf("channel_plugin_version_limit")
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO channel_plugin_versions(fingerprint,plugin_key,source,manifest) VALUES(?,?,?,?)`), row.Fingerprint, row.Key, row.Source, row.Manifest)
	return err
}
