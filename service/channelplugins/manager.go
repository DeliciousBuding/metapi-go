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
		if manifest.IsProvider() || !slices.Contains(manifest.Protocols, protocol) {
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
	if config.Provider != nil {
		if _, _, _, err := m.providerProgram(ctx, *config.Provider); err != nil {
			return err
		}
	}
	for _, binding := range config.Plugins {
		for _, protocol := range binding.Protocols {
			if _, err := m.Resolve(ctx, []store.DirectPluginBinding{binding}, protocol); err != nil {
				return err
			}
		}
	}
	return nil
}

// A captured driver remains immutable for one operation. Runtime input cannot
// override its binding options, protocol or currently resolved credential kind.
type DriverInvocation struct {
	Revision       int64
	program        *channelplugin.Program
	protocol       string
	credentialKind string
	options        map[string]json.RawMessage
}

func (i DriverInvocation) Fingerprint() string {
	if i.program == nil {
		return ""
	}
	return i.program.Fingerprint()
}

func (m *Manager) DescribeProvider(ctx context.Context, binding store.DirectProviderBinding) (channelplugin.Manifest, string, error) {
	p, _, _, err := m.providerProgram(ctx, binding)
	if err != nil {
		return channelplugin.Manifest{}, "", err
	}
	return p.Manifest(), p.Fingerprint(), nil
}

func (m *Manager) ResolveCatalog(ctx context.Context, binding store.DirectProviderBinding, kind string) (DriverInvocation, error) {
	p, revision, options, err := m.providerProgram(ctx, binding)
	if err != nil {
		return DriverInvocation{}, err
	}
	if binding.Catalog == nil || !slices.Contains(p.Manifest().Hooks, "modelsRequest") || !slices.Contains(p.Manifest().CredentialKinds, kind) {
		return DriverInvocation{}, ErrBinding
	}
	return DriverInvocation{revision, p, "", kind, options}, nil
}

func (i DriverInvocation) Call(ctx context.Context, hook string, input channelplugin.DriverInput) (json.RawMessage, error) {
	if i.program == nil {
		return nil, ErrUnavailable
	}
	input.Protocol, input.CredentialKind, input.Options = i.protocol, i.credentialKind, i.options
	if hook == "modelsRequest" || hook == "modelsResponse" {
		input.Protocol = ""
	}
	return i.program.CallDriver(ctx, hook, input)
}

func (i DriverInvocation) HasHook(hook string) bool { return i.program.HasDriverHook(hook) }

func (i DriverInvocation) Observe(body []byte, event bool) (channelplugin.ObservationResult, error) {
	if i.program == nil {
		return channelplugin.ObservationResult{}, ErrUnavailable
	}
	return i.program.Observe(i.protocol, event, body)
}

func (m *Manager) ResolveDriver(ctx context.Context, binding store.DirectProviderBinding, protocol, credentialKind string) (DriverInvocation, error) {
	if !slices.Contains(binding.Protocols, protocol) {
		return DriverInvocation{}, ErrBinding
	}
	p, revision, options, err := m.providerProgram(ctx, binding)
	if err != nil {
		return DriverInvocation{}, err
	}
	if !slices.Contains(p.Manifest().CredentialKinds, credentialKind) {
		return DriverInvocation{}, ErrBinding
	}
	return DriverInvocation{revision, p, protocol, credentialKind, options}, nil
}

func (m *Manager) providerProgram(ctx context.Context, binding store.DirectProviderBinding) (*channelplugin.Program, int64, map[string]json.RawMessage, error) {
	if (store.DirectRequestConfig{Provider: &binding}).Validate() != nil {
		return nil, 0, nil, ErrBinding
	}
	p, revision, err := m.program(ctx, binding.Key)
	if err != nil {
		return nil, 0, nil, err
	}
	options, err := providerOptions(p.Manifest(), binding)
	return p, revision, options, err
}

func providerOptions(manifest channelplugin.Manifest, binding store.DirectProviderBinding) (map[string]json.RawMessage, error) {
	if !manifest.IsProvider() {
		return nil, ErrBinding
	}
	if binding.Catalog != nil && !slices.Contains(manifest.Hooks, "modelsRequest") {
		return nil, ErrBinding
	}
	for _, protocol := range binding.Protocols {
		if !slices.Contains(manifest.Protocols, protocol) {
			return nil, ErrBinding
		}
	}
	options, err := manifest.ResolveOptions(binding.Options)
	if err != nil {
		return nil, ErrBinding
	}
	return options, nil
}
