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
func (m *Manager) ResolveTaskDriver(ctx context.Context, binding store.DirectProviderBinding, protocol, kind, fingerprint string, requestKinds ...string) (DriverInvocation, error) {
	return m.resolveTaskDriver(ctx, binding, protocol, kind, fingerprint, nil, requestKinds...)
}

// The task owner creates its pending identity in the archive transaction. It
// returns that row's opaque ID, which the manager verifies before committing.
// No JS or network I/O runs while the catalog transaction holds its lock.
func (m *Manager) CaptureTaskDriver(ctx context.Context, binding store.DirectProviderBinding, protocol, kind, requestKind, fingerprint string, capture func(*sqlx.Tx, string) (string, error)) (DriverInvocation, error) {
	if capture == nil {
		return DriverInvocation{}, ErrBinding
	}
	return m.resolveTaskDriver(ctx, binding, protocol, kind, fingerprint, capture, requestKind)
}

func (m *Manager) resolveTaskDriver(ctx context.Context, binding store.DirectProviderBinding, protocol, kind, fingerprint string, capture func(*sqlx.Tx, string) (string, error), requestKinds ...string) (DriverInvocation, error) {
	if m == nil || m.db == nil {
		return DriverInvocation{}, ErrUnavailable
	}
	if !slices.Contains(binding.Protocols, protocol) || !slices.Contains([]string{"video", "seedanceVideo", "zenmuxVideo"}, protocol) {
		return DriverInvocation{}, ErrBinding
	}
	if fingerprint == "" {
		if len(requestKinds) > 1 {
			return DriverInvocation{}, ErrBinding
		}
		// Reuse the ordinary manager's compiled immutable program. Holding a
		// catalog transaction must not force a cold compiler for every submit.
		invocation, err := m.ResolveDriver(ctx, binding, protocol, kind)
		if err != nil {
			return DriverInvocation{}, err
		}
		requestKind := "json"
		if len(requestKinds) == 1 {
			requestKind = requestKinds[0]
		}
		if !invocation.AcceptsRequestKind(requestKind) {
			return DriverInvocation{}, channelplugin.ErrRequestKind
		}
		if capture == nil && invocation.program.Manifest().DurableTaskCapture() {
			return DriverInvocation{}, ErrBinding
		}
		return m.captureTaskInvocation(ctx, binding.Key, invocation, invocation.Fingerprint(), capture)
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
	invocation := DriverInvocation{program: program, protocol: protocol, credentialKind: kind, options: options}
	if capture == nil {
		return invocation, nil
	}
	if len(requestKinds) != 1 || !invocation.AcceptsRequestKind(requestKinds[0]) {
		return DriverInvocation{}, channelplugin.ErrRequestKind
	}
	return m.captureTaskInvocation(ctx, binding.Key, invocation, current.Fingerprint(), capture)
}

// New work can reuse an archived program (remix), but cannot reuse its old
// installation eligibility. Pin the executable and pending task under the
// same catalog lock, after rechecking the current installation snapshot.
func (m *Manager) captureTaskInvocation(ctx context.Context, key string, invocation DriverInvocation, currentFingerprint string, capture func(*sqlx.Tx, string) (string, error)) (DriverInvocation, error) {
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
	row, err := store.LoadChannelPlugin(ctx, tx, key)
	digest := sha256.Sum256([]byte(row.Source + "\x00" + row.Manifest))
	if err != nil || !row.Enabled || fmt.Sprintf("%x", digest) != currentFingerprint {
		return DriverInvocation{}, ErrUnavailable
	}
	version := store.ChannelPluginVersion{Key: row.Key, Fingerprint: invocation.Fingerprint(), Source: row.Source, Manifest: row.Manifest}
	if invocation.Fingerprint() != currentFingerprint {
		version, err = store.LoadChannelPluginVersion(ctx, tx, key, invocation.Fingerprint())
		if err != nil {
			return DriverInvocation{}, ErrUnavailable
		}
	}
	if store.ArchiveChannelPluginVersion(ctx, tx, version) != nil {
		return DriverInvocation{}, ErrUnavailable
	}
	if capture != nil {
		id, err := capture(tx, invocation.Fingerprint())
		if err != nil || id == "" || len(id) > 256 {
			return DriverInvocation{}, ErrUnavailable
		}
		var upstreamID, identity string
		err = tx.QueryRowxContext(ctx, tx.Rebind(`SELECT upstream_video_id,direct_identity FROM proxy_video_tasks WHERE public_id=?`), id).Scan(&upstreamID, &identity)
		var reference struct {
			Version     int    `json:"version"`
			Key         string `json:"driverKey"`
			Fingerprint string `json:"driverFingerprint"`
		}
		if err != nil || upstreamID != "" || len(identity) > 256<<10 || json.Unmarshal([]byte(identity), &reference) != nil || reference.Version != 3 || reference.Key != key || reference.Fingerprint != invocation.Fingerprint() {
			return DriverInvocation{}, ErrBinding
		}
	}
	if tx.Commit() != nil {
		return DriverInvocation{}, ErrUnavailable
	}
	return invocation, nil
}
