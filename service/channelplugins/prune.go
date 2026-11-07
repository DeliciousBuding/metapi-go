package channelplugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var ErrVersionNotPrunable = errors.New("channel_plugin_version_not_prunable")

// An old host rejects taskCapture before compiling or archiving the program.
// A new host cannot submit it without committing its pending reference under
// this same catalog lock. Zero references are safe only with that contract.
func PruneArchivedVersion(ctx context.Context, db *sqlx.DB, key, fingerprint string) error {
	if !contract.ValidKey(key) || !contract.ValidFingerprint(fingerprint) {
		return ErrBinding
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = store.LockChannelPluginBindings(ctx, tx); err != nil {
		return err
	}
	refs, err := store.ChannelPluginVersionReferences(ctx, tx)
	if err != nil {
		return err
	}
	if refs[store.ChannelPluginVersionReference{Key: key, Fingerprint: fingerprint}] != 0 {
		return ErrVersionNotPrunable
	}
	version, err := store.LoadChannelPluginVersion(ctx, tx, key, fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	program, err := channelplugin.Compile(ctx, version.Source)
	if err != nil {
		return err
	}
	manifest := program.Manifest()
	metadata, err := json.Marshal(manifest)
	if err != nil || manifest.Key != key || program.Fingerprint() != fingerprint || string(metadata) != version.Manifest {
		return ErrBinding
	}
	if !manifest.DurableTaskCapture() {
		return ErrVersionNotPrunable
	}
	if _, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM channel_plugin_versions WHERE plugin_key=? AND fingerprint=?`), key, fingerprint); err != nil {
		return err
	}
	return tx.Commit()
}
