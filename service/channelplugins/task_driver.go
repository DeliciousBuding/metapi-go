package channelplugins

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// New persistent work captures and archives one installed program before I/O.
// Existing work uses its old executable while current installation/credential
// eligibility still controls whether another operation may start.
func (m *Manager) ResolveTaskDriver(ctx context.Context, binding store.DirectProviderBinding, protocol, kind, fingerprint string) (DriverInvocation, error) {
	if m == nil || m.db == nil {
		return DriverInvocation{}, ErrUnavailable
	}
	if !slices.Contains(binding.Protocols, protocol) || !slices.Contains([]string{"video", "seedanceVideo", "zenmuxVideo"}, protocol) {
		return DriverInvocation{}, ErrBinding
	}
	if fingerprint == "" {
		// Reuse the ordinary manager's compiled immutable program. Holding a
		// catalog transaction must not force a cold compiler for every submit.
		invocation, err := m.ResolveDriver(ctx, binding, protocol, kind)
		if err != nil {
			return DriverInvocation{}, err
		}
		db, ok := m.db.(*sqlx.DB)
		if !ok {
			return DriverInvocation{}, ErrUnavailable
		}
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			return DriverInvocation{}, ErrUnavailable
		}
		defer func() { _ = tx.Rollback() }()
		if store.LockChannelPluginBindings(ctx, tx) != nil {
			return DriverInvocation{}, ErrUnavailable
		}
		row, err := store.LoadChannelPlugin(ctx, tx, binding.Key)
		digest := sha256.Sum256([]byte(row.Source + "\x00" + row.Manifest))
		if err != nil || !row.Enabled || fmt.Sprintf("%x", digest) != invocation.Fingerprint() {
			return DriverInvocation{}, ErrUnavailable
		}
		version := store.ChannelPluginVersion{Key: row.Key, Fingerprint: invocation.Fingerprint(), Source: row.Source, Manifest: row.Manifest}
		if store.ArchiveChannelPluginVersion(ctx, tx, version) != nil || tx.Commit() != nil {
			return DriverInvocation{}, ErrUnavailable
		}
		return invocation, nil
	}
	if !contract.ValidFingerprint(fingerprint) {
		return DriverInvocation{}, ErrBinding
	}
	current, _, _, err := m.providerProgram(ctx, binding)
	if err != nil {
		return DriverInvocation{}, err
	}
	if !slices.Contains(current.Manifest().CredentialKinds, kind) {
		return DriverInvocation{}, ErrBinding
	}
	version, err := store.LoadChannelPluginVersion(ctx, m.db, binding.Key, fingerprint)
	if err != nil {
		return DriverInvocation{}, ErrUnavailable
	}
	digest := sha256.Sum256([]byte(version.Source + "\x00" + version.Manifest))
	if fmt.Sprintf("%x", digest) != fingerprint {
		return DriverInvocation{}, ErrBinding
	}
	cacheKey := "task:" + fingerprint
	m.mu.Lock()
	item, ok := m.cache[cacheKey]
	m.mu.Unlock()
	program := item.program
	if !ok || item.digest != digest {
		ready := m.compiling.DoChan(cacheKey, func() (any, error) {
			p, err := channelplugin.Compile(ctx, version.Source)
			if err != nil {
				return nil, err
			}
			metadata, _ := json.Marshal(p.Manifest())
			if p.Fingerprint() != fingerprint || p.Manifest().Key != binding.Key || string(metadata) != version.Manifest {
				return nil, ErrBinding
			}
			m.mu.Lock()
			if len(m.cache) >= 128 {
				clear(m.cache)
			}
			m.cache[cacheKey] = cached{digest: digest, program: p}
			m.mu.Unlock()
			return p, nil
		})
		select {
		case <-ctx.Done():
			return DriverInvocation{}, ctx.Err()
		case result := <-ready:
			if result.Err != nil {
				return DriverInvocation{}, result.Err
			}
			program = result.Val.(*channelplugin.Program)
		}
	}
	options, err := providerOptions(program.Manifest(), binding)
	if err != nil || !slices.Contains(program.Manifest().CredentialKinds, kind) {
		return DriverInvocation{}, ErrBinding
	}
	return DriverInvocation{program: program, protocol: protocol, credentialKind: kind, options: options}, nil
}
