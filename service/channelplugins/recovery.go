package channelplugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var ErrVersionNotReferenced = errors.New("channel_plugin_version_not_referenced")

type ArchivedVersion struct {
	Key            string `json:"key" db:"plugin_key"`
	Fingerprint    string `json:"fingerprint" db:"fingerprint"`
	Available      bool   `json:"available"`
	TaskReferences int64  `json:"taskReferences"`
}

// Listing reads metadata only and includes missing programs referenced by tasks.
// Availability means a stored archive exists, not that it is executable or that
// current channel/credential eligibility allows a new operation.
func ListArchivedVersions(ctx context.Context, db *sqlx.DB) ([]ArchivedVersion, error) {
	// A read snapshot must not hold the plugin/catalog write lock while scanning
	// task references. PostgreSQL's default READ COMMITTED has per-query snapshots.
	var options *sql.TxOptions
	if db.DriverName() == "pgx" || db.DriverName() == "postgres" {
		options = &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	}
	tx, err := db.BeginTxx(ctx, options)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	refs, err := store.ChannelPluginVersionReferences(ctx, tx)
	if err != nil {
		return nil, err
	}
	var archived []ArchivedVersion
	if err = tx.SelectContext(ctx, &archived, `SELECT plugin_key,fingerprint FROM channel_plugin_versions LIMIT 513`); err != nil {
		return nil, err
	}
	if len(archived) > 512 {
		return nil, store.ErrChannelPluginReferences
	}
	out := make([]ArchivedVersion, 0, len(archived)+len(refs))
	for _, item := range archived {
		if !contract.ValidKey(item.Key) || !contract.ValidFingerprint(item.Fingerprint) {
			return nil, store.ErrChannelPluginReferences
		}
		ref := store.ChannelPluginVersionReference{Key: item.Key, Fingerprint: item.Fingerprint}
		item.Available, item.TaskReferences = true, refs[ref]
		delete(refs, ref)
		out = append(out, item)
	}
	for ref, count := range refs {
		out = append(out, ArchivedVersion{Key: ref.Key, Fingerprint: ref.Fingerprint, TaskReferences: count})
	}
	slices.SortFunc(out, func(a, b ArchivedVersion) int {
		if order := strings.Compare(a.Key, b.Key); order != 0 {
			return order
		}
		return strings.Compare(a.Fingerprint, b.Fingerprint)
	})
	return out, tx.Commit()
}

// Explicit administrator recovery validates the original executable fingerprint
// in the ordinary bounded compiler. It neither installs/enables a plugin nor
// substitutes the current program, rewrites task identities, or repairs corrupt
// immutable rows. Restoration is limited to missing, persisted task references.
func RestoreArchivedVersion(ctx context.Context, db *sqlx.DB, key, fingerprint, source string) error {
	if !contract.ValidKey(key) || !contract.ValidFingerprint(fingerprint) {
		return ErrBinding
	}
	program, err := channelplugin.Compile(ctx, source)
	if err != nil {
		return err
	}
	manifest := program.Manifest()
	if !manifest.IsProvider() || manifest.Key != key || program.Fingerprint() != fingerprint {
		return ErrBinding
	}
	metadata, err := json.Marshal(manifest)
	if err != nil {
		return err
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
	if refs[store.ChannelPluginVersionReference{Key: key, Fingerprint: fingerprint}] == 0 {
		return ErrVersionNotReferenced
	}
	if err = store.ArchiveChannelPluginVersion(ctx, tx, store.ChannelPluginVersion{Key: key, Fingerprint: fingerprint, Source: source, Manifest: string(metadata)}); err != nil {
		return err
	}
	return tx.Commit()
}
