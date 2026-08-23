// metapi-go/lib — auth-session regression tests.
//
// Protects the simplified auth contract after the refresh/rotation scaffold
// was removed: a valid persisted bundle authenticates, a missing/expired
// bundle clears and reports anonymous, and the one-shot post-401 re-read
// picks up a token written by another tab. No refresh endpoint, broadcast
// channel, or rotation facade is exercised here — those are gone by design.

import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  AUTH_SESSION_DURATION_MS,
  bootstrapAuthentication,
  getAccessToken,
  getAuthToken,
  hasValidAuthSession,
  isAuthSessionExpired,
  resolveAuthenticationAfterUnauthorized,
  setAuthBundle,
  wasAuthSessionExpiredOnLastBoot,
  type AuthBundle,
} from '../auth-session'

const TOKEN_KEY = 'auth_token'
const EXPIRES_KEY = 'auth_token_expires_at'
const NOW_MS = 1_750_000_000_000

interface MemoryStorage {
  getItem: (key: string) => string | null
  setItem: (key: string, value: string) => void
  removeItem: (key: string) => void
  has: (key: string) => boolean
  get: (key: string) => string | null
}

function memoryStorage(initial?: Record<string, string>): MemoryStorage {
  const store = new Map<string, string>(Object.entries(initial ?? {}))
  return {
    getItem: (key) => store.get(key) ?? null,
    setItem: (key, value) => {
      store.set(key, value)
    },
    removeItem: (key) => {
      store.delete(key)
    },
    has: (key) => store.has(key),
    get: (key) => store.get(key) ?? null,
  }
}

function bundle(token: string, expiresAtSec: number): AuthBundle {
  return {
    access_token: token,
    token_type: 'Bearer',
    access_expires_at: expiresAtSec,
  }
}

// ---------------------------------------------------------------------------
// Token resolution (the interceptor + route guards read through these)
// ---------------------------------------------------------------------------

describe('getAuthToken — token validity', () => {
  it('returns the token for a valid unexpired session', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'token-a',
      [EXPIRES_KEY]: String(NOW_MS + 60_000),
    })
    expect(getAuthToken(storage, NOW_MS)).toBe('token-a')
    expect(getAccessToken(storage, NOW_MS)).toBe('token-a')
    expect(hasValidAuthSession(storage, NOW_MS)).toBe(true)
  })

  it('returns null when no token is stored', () => {
    expect(getAuthToken(memoryStorage(), NOW_MS)).toBeNull()
  })

  it('clears stale keys and returns null for an expired token', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'stale-token',
      [EXPIRES_KEY]: String(NOW_MS - 1),
    })
    expect(getAuthToken(storage, NOW_MS)).toBeNull()
    expect(storage.has(TOKEN_KEY)).toBe(false)
    expect(storage.has(EXPIRES_KEY)).toBe(false)
  })

  it('clears and returns null for a non-numeric expiry', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'token-a',
      [EXPIRES_KEY]: 'not-a-number',
    })
    expect(getAuthToken(storage, NOW_MS)).toBeNull()
    expect(storage.has(TOKEN_KEY)).toBe(false)
  })

  it('re-persists a 12h TTL when expiry is missing (legacy migration)', () => {
    const storage = memoryStorage({ [TOKEN_KEY]: 'legacy-token' })
    expect(getAuthToken(storage, NOW_MS)).toBe('legacy-token')
    expect(storage.get(EXPIRES_KEY)).toBe(
      String(NOW_MS + AUTH_SESSION_DURATION_MS)
    )
  })
})

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

describe('setAuthBundle', () => {
  it('persists the token and converts unix-seconds expiry to ms epoch', () => {
    const storage = memoryStorage()
    const expiresAtSec = Math.floor(Date.now() / 1000) + 3600
    setAuthBundle(bundle('bundled-token', expiresAtSec), storage)
    expect(storage.get(TOKEN_KEY)).toBe('bundled-token')
    expect(storage.get(EXPIRES_KEY)).toBe(String(expiresAtSec * 1000))
  })
})

// ---------------------------------------------------------------------------
// Post-401 re-read — the "another tab wrote a new token" one-shot contract
// ---------------------------------------------------------------------------

describe('resolveAuthenticationAfterUnauthorized — one-shot re-read', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns authenticated when another tab wrote a valid token', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'fresh-token',
      [EXPIRES_KEY]: String(Date.now() + 60_000),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)

    const outcome = resolveAuthenticationAfterUnauthorized()
    expect(outcome.kind).toBe('authenticated')
    if (outcome.kind === 'authenticated') {
      expect(outcome.bundle.access_token).toBe('fresh-token')
      expect(outcome.bundle.token_type).toBe('Bearer')
    }
    // A valid token is kept, not cleared.
    expect(storage.has(TOKEN_KEY)).toBe(true)
  })

  it('clears and returns anonymous for an expired token', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'expired-token',
      [EXPIRES_KEY]: String(Date.now() - 1),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)

    expect(resolveAuthenticationAfterUnauthorized()).toEqual({
      kind: 'anonymous',
    })
    expect(storage.has(TOKEN_KEY)).toBe(false)
    expect(storage.has(EXPIRES_KEY)).toBe(false)
  })

  it('returns anonymous when storage is empty', () => {
    vi.stubGlobal('localStorage', memoryStorage() as unknown as Storage)
    expect(resolveAuthenticationAfterUnauthorized()).toEqual({
      kind: 'anonymous',
    })
  })
})

// ---------------------------------------------------------------------------
// Startup bootstrap — same resolve/clear contract
// ---------------------------------------------------------------------------

describe('bootstrapAuthentication', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns authenticated for a valid stored token', () => {
    vi.stubGlobal(
      'localStorage',
      memoryStorage({
        [TOKEN_KEY]: 'boot-token',
        [EXPIRES_KEY]: String(Date.now() + 60_000),
      }) as unknown as Storage
    )
    expect(bootstrapAuthentication().kind).toBe('authenticated')
  })

  it('returns anonymous and clears an expired token', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'expired-token',
      [EXPIRES_KEY]: String(Date.now() - 1),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)
    expect(bootstrapAuthentication()).toEqual({ kind: 'anonymous' })
    expect(storage.has(TOKEN_KEY)).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// Session-expired record — the authenticated guard reads this to render the
// "session expired" sign-in notice after the stale entry has been wiped.
// ---------------------------------------------------------------------------

describe('wasAuthSessionExpiredOnLastBoot', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('reports false when the storage was never populated', () => {
    vi.stubGlobal('localStorage', memoryStorage() as unknown as Storage)
    bootstrapAuthentication()
    expect(wasAuthSessionExpiredOnLastBoot()).toBe(false)
  })

  it('reports true when the bootstrap cleared an expired token', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'expired-token',
      [EXPIRES_KEY]: String(Date.now() - 1),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)
    expect(bootstrapAuthentication().kind).toBe('anonymous')
    // The stale entry is gone, but the fact survives for the route guard.
    expect(storage.has(TOKEN_KEY)).toBe(false)
    expect(wasAuthSessionExpiredOnLastBoot()).toBe(true)
  })

  it('stays false when the bootstrap found a live token', () => {
    vi.stubGlobal(
      'localStorage',
      memoryStorage({
        [TOKEN_KEY]: 'live-token',
        [EXPIRES_KEY]: String(Date.now() + 60_000),
      }) as unknown as Storage
    )
    expect(bootstrapAuthentication().kind).toBe('authenticated')
    expect(wasAuthSessionExpiredOnLastBoot()).toBe(false)
  })

  it('resets to false once a fresh session is persisted (relogin)', () => {
    vi.stubGlobal(
      'localStorage',
      memoryStorage({
        [TOKEN_KEY]: 'expired-token',
        [EXPIRES_KEY]: String(Date.now() - 1),
      }) as unknown as Storage
    )
    bootstrapAuthentication()
    expect(wasAuthSessionExpiredOnLastBoot()).toBe(true)

    // Same storage object simulates the user signing in again on the page
    // that was just redirected to.
    setAuthBundle(bundle('fresh-token', Math.floor(Date.now() / 1000) + 3600))
    expect(wasAuthSessionExpiredOnLastBoot()).toBe(false)
  })

  it('records expiry from the post-401 re-read path too', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'expired-token',
      [EXPIRES_KEY]: String(Date.now() - 1),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)
    expect(resolveAuthenticationAfterUnauthorized()).toEqual({
      kind: 'anonymous',
    })
    expect(wasAuthSessionExpiredOnLastBoot()).toBe(true)
  })

  it('raw probe distinguishes expired from missing without clearing', () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'expired-token',
      [EXPIRES_KEY]: String(Date.now() - 1),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)
    expect(isAuthSessionExpired()).toBe(true)
    expect(storage.has(TOKEN_KEY)).toBe(true)

    const missing = memoryStorage({})
    expect(isAuthSessionExpired(missing)).toBe(false)
    expect(missing.has(TOKEN_KEY)).toBe(false)
  })
})
