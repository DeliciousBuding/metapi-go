// Package channelplugins owns installed wire-hook lifecycle and immutable
// attempt snapshots. Transport, credentials and routing stay in their owners.
package channelplugins

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sync/singleflight"
	"slices"
	"sync"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var (
	ErrUnavailable = errors.New("channel_plugin_unavailable")
	ErrBinding     = errors.New("channel_plugin_binding_invalid")
)

type cached struct {
	revision int64
	digest   [32]byte
	program  *channelplugin.Program
}

type Manager struct {
	db        sqlx.ExtContext
	mu        sync.Mutex
	cache     map[string]cached
	compiling singleflight.Group
}

func New(db sqlx.ExtContext) *Manager { return &Manager{db: db, cache: map[string]cached{}} }

type Invocation struct {
	Program  *channelplugin.Program
	Binding  store.DirectPluginBinding
	Revision int64
}

func (m *Manager) program(ctx context.Context, key string) (*channelplugin.Program, int64, error) {
	if m == nil || m.db == nil {
		return nil, 0, ErrUnavailable
	}
	row, err := store.LoadChannelPlugin(ctx, m.db, key)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !row.Enabled {
		return nil, 0, ErrUnavailable
	}
	if err != nil {
		return nil, 0, err
	}
	digest := sha256.Sum256([]byte(row.Source + "\x00" + row.Manifest))
	m.mu.Lock()
	item, ok := m.cache[key]
	m.mu.Unlock()
	if ok && item.revision == row.Revision && item.digest == digest {
		return item.program, row.Revision, nil
	}
	ready := m.compiling.DoChan(fmt.Sprintf("%s:%d:%x", key, row.Revision, digest), func() (any, error) {
		p, err := channelplugin.Compile(ctx, row.Source)
		if err != nil {
			return nil, err
		}
		metadata, err := json.Marshal(p.Manifest())
		if err != nil || p.Manifest().Key != key || string(metadata) != row.Manifest {
			return nil, ErrBinding
		}
		m.mu.Lock()
		if len(m.cache) >= 128 {
			clear(m.cache)
		}
		m.cache[key] = cached{row.Revision, digest, p}
		m.mu.Unlock()
		return p, nil
	})
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case result := <-ready:
		if result.Err != nil {
			return nil, 0, result.Err
		}
		return result.Val.(*channelplugin.Program), row.Revision, nil
	}
}

func (m *Manager) Resolve(ctx context.Context, bindings []store.DirectPluginBinding, protocol string) ([]Invocation, error) {
	var out []Invocation
	for _, binding := range bindings {
		if !slices.Contains(binding.Protocols, protocol) {
			continue
		}
		p, revision, err := m.program(ctx, binding.Key)
		if err != nil {
			return nil, err
		}
		manifest := p.Manifest()
		if !slices.Contains(manifest.Protocols, protocol) {
			return nil, ErrBinding
		}
		options, err := manifest.ResolveOptions(binding.Options)
		if err != nil {
			return nil, err
		}
		binding.Protocols = slices.Clone(binding.Protocols)
		binding.Options = options
		out = append(out, Invocation{p, binding, revision})
	}
	return out, nil
}

func ValidateBindings(ctx context.Context, db sqlx.ExtContext, config store.DirectRequestConfig, endpoints store.DirectEndpoints) error {
	if err := config.ValidateEndpoints(endpoints); err != nil {
		return err
	}
	m := New(db)
	for _, binding := range config.Plugins {
		for _, protocol := range binding.Protocols {
			if _, err := m.Resolve(ctx, []store.DirectPluginBinding{binding}, protocol); err != nil {
				return err
			}
		}
	}
	return nil
}
