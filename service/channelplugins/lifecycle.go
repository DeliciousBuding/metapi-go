package channelplugins

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

type Item struct {
	Key       string                  `json:"key"`
	Revision  int64                   `json:"revision"`
	Enabled   bool                    `json:"enabled"`
	Manifest  *channelplugin.Manifest `json:"manifest,omitempty"`
	LoadError string                  `json:"loadError,omitempty"`
}

// Invalid persisted metadata is an explicit unavailable row, never a successful
// empty list. Listing neither reads nor evaluates executable source.
func List(ctx context.Context, db *sqlx.DB) ([]Item, error) {
	rows, err := store.ListChannelPlugins(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, row := range rows {
		item := Item{Key: row.Key, Revision: row.Revision, Enabled: row.Enabled}
		var manifest channelplugin.Manifest
		if requestoverride.Decode([]byte(row.Manifest), &manifest) != nil || manifest.Validate() != nil || manifest.Key != row.Key {
			item.LoadError = "channel_plugin_load_failed"
		} else {
			item.Manifest = &manifest
		}
		out = append(out, item)
	}
	return out, nil
}

func Save(ctx context.Context, db *sqlx.DB, key, source string, enabled bool, expected int64) (store.ChannelPlugin, error) {
	if expected < 0 || !contract.ValidKey(key) {
		return store.ChannelPlugin{}, ErrBinding
	}
	p, err := channelplugin.Compile(ctx, source)
	if err != nil {
		return store.ChannelPlugin{}, err
	}
	manifest := p.Manifest()
	if manifest.Key != key {
		return store.ChannelPlugin{}, ErrBinding
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return store.ChannelPlugin{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = store.LockChannelPluginBindings(ctx, tx); err != nil {
		return store.ChannelPlugin{}, err
	}
	var configs []store.DirectRequestConfig
	if err = sqlx.SelectContext(ctx, tx, &configs, `SELECT request_config FROM upstream_channels`); err != nil {
		return store.ChannelPlugin{}, err
	}
	for _, config := range configs {
		if config.Provider != nil && config.Provider.Key == key {
			if _, err = providerOptions(manifest, *config.Provider); err != nil {
				return store.ChannelPlugin{}, ErrBinding
			}
		}
		for _, binding := range config.Plugins {
			if binding.Key != key {
				continue
			}
			if manifest.IsProvider() {
				return store.ChannelPlugin{}, ErrBinding
			}
			for _, protocol := range binding.Protocols {
				if !slices.Contains(manifest.Protocols, protocol) {
					return store.ChannelPlugin{}, ErrBinding
				}
			}
			if _, err = manifest.ResolveOptions(binding.Options); err != nil {
				return store.ChannelPlugin{}, ErrBinding
			}
		}
	}
	if expected == 0 {
		var count int
		if err = tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM channel_plugins`); err != nil {
			return store.ChannelPlugin{}, err
		}
		if count >= 128 {
			return store.ChannelPlugin{}, ErrBinding
		}
	}
	metadata, err := json.Marshal(manifest)
	if err != nil {
		return store.ChannelPlugin{}, err
	}
	row, err := store.SaveChannelPlugin(ctx, tx, key, source, string(metadata), enabled, expected)
	if err != nil {
		return store.ChannelPlugin{}, err
	}
	if err = tx.Commit(); err != nil {
		return store.ChannelPlugin{}, err
	}
	return row, nil
}
