// Tests for the #1034 session model client state (cookie credential,
// metadata-only localStorage). The module keeps boot-time state, so every
// test gets a fresh instance via vi.resetModules().

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type * as AuthSessionModule from '../auth-session'

type Module = typeof AuthSessionModule

const FIXED_NOW = new Date('2026-01-15T12:00:00Z').getTime()

let mod: Module

async function freshModule(): Promise<Module> {
  vi.resetModules()
  return import('../auth-session')
}

function mockFetchOnce(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn(async () => ({
    ok,
    status,
    json: async () => body,
  }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

beforeEach(async () => {
  localStorage.clear()
  vi.useFakeTimers()
  vi.setSystemTime(FIXED_NOW)
  mod = await freshModule()
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
  localStorage.clear()
})

describe('legacy plaintext token cleanup (#1034)', () => {
  it('wipes auth_token keys on any session access', () => {
    localStorage.setItem('auth_token', 'leaked-master-token')
    localStorage.setItem('auth_token_expires_at', String(FIXED_NOW + 60_000))

    mod.readSessionMeta(localStorage)

    expect(localStorage.getItem('auth_token')).toBeNull()
    expect(localStorage.getItem('auth_token_expires_at')).toBeNull()
  })

  it('never stores a credential, only expiry metadata', () => {
    mod.persistSessionMeta(FIXED_NOW + 3_600_000, localStorage)

    const stored = localStorage.getItem('metapi_session_meta')
    expect(stored).not.toBeNull()
    expect(stored).not.toContain('token')
    expect(JSON.parse(stored as string)).toEqual({
      expiresAtMs: FIXED_NOW + 3_600_000,
    })
  })
})

describe('session metadata', () => {
  it('reads back persisted expiry', () => {
    mod.persistSessionMeta(FIXED_NOW + 1_000, localStorage)
    expect(mod.readSessionMeta(localStorage)).toEqual({
      expiresAtMs: FIXED_NOW + 1_000,
    })
  })

  it('returns null for missing or corrupt metadata', () => {
    expect(mod.readSessionMeta(localStorage)).toBeNull()
    localStorage.setItem('metapi_session_meta', '{not json')
    expect(mod.readSessionMeta(localStorage)).toBeNull()
    localStorage.setItem('metapi_session_meta', '{"expiresAtMs":"soon"}')
    expect(mod.readSessionMeta(localStorage)).toBeNull()
  })

  it('clearAuthSession removes metadata and legacy keys', () => {
    localStorage.setItem('auth_token', 'legacy')
    mod.persistSessionMeta(FIXED_NOW + 1_000, localStorage)

    mod.clearAuthSession(localStorage)

    expect(mod.readSessionMeta(localStorage)).toBeNull()
    expect(localStorage.getItem('auth_token')).toBeNull()
  })

  it('isAuthSessionExpired only when metadata exists and elapsed', () => {
    expect(mod.isAuthSessionExpired(localStorage, FIXED_NOW)).toBe(false)

    mod.persistSessionMeta(FIXED_NOW + 1_000, localStorage)
    expect(mod.isAuthSessionExpired(localStorage, FIXED_NOW)).toBe(false)
    expect(mod.isAuthSessionExpired(localStorage, FIXED_NOW + 1_000)).toBe(true)
  })
})

describe('bootstrapAuthentication', () => {
  it('persists the server expiry when the session is valid', async () => {
    const expiresAt = new Date(FIXED_NOW + 7_200_000).toISOString()
    const fetchMock = mockFetchOnce({
      authenticated: true,
      source: 'session',
      expiresAt,
    })

    const outcome = await mod.bootstrapAuthentication()

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/auth/session',
      expect.objectContaining({ method: 'GET' })
    )
    expect(outcome).toEqual({
      kind: 'authenticated',
      expiresAtMs: Date.parse(expiresAt),
    })
    expect(mod.readSessionMeta(localStorage)).toEqual({
      expiresAtMs: Date.parse(expiresAt),
    })
    expect(mod.hasValidAuthSession(localStorage)).toBe(true)
    expect(mod.wasAuthSessionExpiredOnLastBoot()).toBe(false)
  })

  it('clears client state and reports expired when the server says no session', async () => {
    // Local metadata claims a live session; the server disagrees (the row
    // expired or was revoked) — the bootstrap must record "expired" so the
    // sign-in redirect can explain why.
    mod.persistSessionMeta(FIXED_NOW + 60_000, localStorage)
    mockFetchOnce({ authenticated: false })

    const outcome = await mod.bootstrapAuthentication()

    expect(outcome).toEqual({ kind: 'anonymous', expired: true })
    expect(mod.readSessionMeta(localStorage)).toBeNull()
    expect(mod.hasValidAuthSession(localStorage)).toBe(false)
    expect(mod.wasAuthSessionExpiredOnLastBoot()).toBe(true)
  })

  it('does not flag expired when there was never a session', async () => {
    mockFetchOnce({ authenticated: false })

    const outcome = await mod.bootstrapAuthentication()

    expect(outcome).toEqual({ kind: 'anonymous', expired: false })
    expect(mod.wasAuthSessionExpiredOnLastBoot()).toBe(false)
  })

  it('treats an unreachable server as anonymous (fail closed for UI)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('network down')
      })
    )

    const outcome = await mod.bootstrapAuthentication()

    expect(outcome.kind).toBe('anonymous')
    expect(mod.hasValidAuthSession(localStorage)).toBe(false)
  })
})

describe('post-401 resolution', () => {
  it('clears everything and marks the boot anonymous', async () => {
    const expiresAt = new Date(FIXED_NOW + 7_200_000).toISOString()
    mockFetchOnce({ authenticated: true, source: 'session', expiresAt })
    await mod.bootstrapAuthentication()
    expect(mod.hasValidAuthSession(localStorage)).toBe(true)

    const outcome = mod.resolveAuthenticationAfterUnauthorized()

    expect(outcome.kind).toBe('anonymous')
    expect(mod.readSessionMeta(localStorage)).toBeNull()
    expect(mod.hasValidAuthSession(localStorage)).toBe(false)
  })
})
