// metapi-go/lib — http-client auth interceptor regression tests.
//
// Protects the #1034 session contract:
//   • on a 401 the interceptor clears the client-side session metadata and
//     redirects to sign-in (the credential itself is an HttpOnly cookie —
//     there is nothing to replay);
//   • the redirect preserves the interrupted path via ?redirect=…;
//   • a 403 with the backend's "Invalid token" body is treated like a 401,
//     while an IP-allowlist 403 keeps the session intact;
//   • a 403 reauthRequired body (sensitive op) surfaces to the caller
//     WITHOUT clearing the session or redirecting.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  apiClient,
  extractApiErrorBody,
  resolveResponseErrorCode,
} from '../http-client'

const toastErrorMock = vi.fn()

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: (...args: unknown[]) => toastErrorMock(...args),
    info: vi.fn(),
    warning: vi.fn(),
    message: vi.fn(),
    loading: vi.fn(),
    custom: vi.fn(),
    promise: vi.fn(),
    dismiss: vi.fn(),
  },
}))

const TOKEN_KEY = 'auth_token'
const EXPIRES_KEY = 'auth_token_expires_at'
const SESSION_META_KEY = 'metapi_session_meta'

interface MemoryStorage {
  getItem: (key: string) => string | null
  setItem: (key: string, value: string) => void
  removeItem: (key: string) => void
  has: (key: string) => boolean
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
  }
}

type AdapterError = Error & {
  config?: unknown
  response?: { status: number; data: unknown }
}

describe('apiClient auth interceptor', () => {
  const originalAdapter = apiClient.defaults.adapter
  const originalLocation = window.location
  const replaceMock = vi.fn()

  function installLocationMock(pathname = '/accounts'): void {
    // jsdom's Location methods are non-configurable, so `vi.spyOn` cannot
    // redefine them. Replace the whole `window.location` value with a minimal
    // stub that exposes a spyable `replace`.
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {
        pathname,
        search: '',
        origin: 'http://localhost',
        href: `http://localhost${pathname}`,
        replace: replaceMock,
      },
    })
  }

  function restoreLocation(): void {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    })
  }

  beforeEach(() => {
    replaceMock.mockReset()
    toastErrorMock.mockReset()
    installLocationMock()
  })

  afterEach(() => {
    apiClient.defaults.adapter = originalAdapter
    restoreLocation()
    vi.unstubAllGlobals()
  })

  function installStatusAdapter(
    adapterCalls: unknown[],
    status: number,
    data: unknown
  ): void {
    apiClient.defaults.adapter = async (config) => {
      adapterCalls.push(config)
      const error = new Error(
        `Request failed with status code ${status}`
      ) as AdapterError
      error.config = config
      error.response = { status, data }
      throw error
    }
  }

  it('clears the session metadata and redirects on 401 (no replay)', async () => {
    const storage = memoryStorage({
      [SESSION_META_KEY]: JSON.stringify({
        expiresAtMs: Date.now() + 60_000,
      }),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)

    const adapterCalls: unknown[] = []
    installStatusAdapter(adapterCalls, 401, { error: 'unauthorized' })

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    // The server ended the session; exactly one request, no retry loop.
    expect(adapterCalls).toHaveLength(1)
    // The interrupted path is preserved so sign-in can send the user back.
    expect(replaceMock).toHaveBeenCalledWith('/sign-in?redirect=%2Faccounts')
    // Client-side session metadata is cleared.
    expect(storage.has(SESSION_META_KEY)).toBe(false)
  })

  it('redirects without replay when no valid token remains', async () => {
    vi.stubGlobal('localStorage', memoryStorage() as unknown as Storage)

    const adapterCalls: unknown[] = []
    installStatusAdapter(adapterCalls, 401, { error: 'unauthorized' })

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    // anonymous → no replay, a single adapter call, redirect to sign-in.
    expect(adapterCalls).toHaveLength(1)
    expect(replaceMock).toHaveBeenCalledWith('/sign-in?redirect=%2Faccounts')
  })

  it('does not append a redirect param when already on the sign-in page', async () => {
    installLocationMock('/sign-in')
    vi.stubGlobal('localStorage', memoryStorage() as unknown as Storage)

    const adapterCalls: unknown[] = []
    installStatusAdapter(adapterCalls, 401, { error: 'unauthorized' })

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    expect(replaceMock).not.toHaveBeenCalled()
  })

  it('treats a 403 "Invalid token" like an expired session', async () => {
    const storage = memoryStorage({
      [SESSION_META_KEY]: JSON.stringify({
        expiresAtMs: Date.now() + 60_000,
      }),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)

    const adapterCalls: unknown[] = []
    installStatusAdapter(adapterCalls, 403, { error: 'Invalid token' })

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    // No replay: one request, then the session is cleared + redirect.
    expect(adapterCalls).toHaveLength(1)
    expect(storage.has(SESSION_META_KEY)).toBe(false)
    expect(replaceMock).toHaveBeenCalledWith('/sign-in?redirect=%2Faccounts')
  })

  it('surfaces a 403 reauthRequired without clearing or redirecting', async () => {
    const storage = memoryStorage({
      [SESSION_META_KEY]: JSON.stringify({
        expiresAtMs: Date.now() + 60_000,
      }),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)

    const adapterCalls: unknown[] = []
    installStatusAdapter(adapterCalls, 403, {
      error: 'Sensitive operation requires master token confirmation',
      reauthRequired: true,
    })

    await expect(apiClient.get('/api/settings/backup/export')).rejects.toThrow()

    // The session is fine — the master token must be re-presented. No clear,
    // no redirect; the caller opens the reauth prompt.
    expect(adapterCalls).toHaveLength(1)
    expect(storage.has(SESSION_META_KEY)).toBe(true)
    expect(replaceMock).not.toHaveBeenCalled()
  })

  it('keeps the session for an IP-allowlist 403', async () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'still-valid-token',
      [EXPIRES_KEY]: String(Date.now() + 60_000),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)

    const adapterCalls: unknown[] = []
    installStatusAdapter(adapterCalls, 403, { error: 'IP not allowed' })

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    // No auth replay, no session clear, no sign-in redirect for IP rejections.
    expect(adapterCalls).toHaveLength(1)
    expect(storage.has(TOKEN_KEY)).toBe(true)
    expect(replaceMock).not.toHaveBeenCalled()
    expect(toastErrorMock).toHaveBeenCalledWith('IP not allowed')
  })
})

describe('resolveResponseErrorCode — unified error envelope (#1065)', () => {
  it('returns the errorCode carried by a unified error body', () => {
    expect(
      resolveResponseErrorCode({
        error:
          'target database is the same as the running database; migration aborted',
        errorCode: 'sameMigrationTarget',
      })
    ).toBe('sameMigrationTarget')
  })

  it('returns undefined for byte-compatible legacy bodies without errorCode', () => {
    // Endpoints without a registered code omit the key entirely.
    expect(
      resolveResponseErrorCode({ error: 'some rejection reason' })
    ).toBeUndefined()
    // Legacy TS-era `{ message }` shape.
    expect(
      resolveResponseErrorCode({ message: 'legacy error shape' })
    ).toBeUndefined()
  })

  it('returns undefined for non-string or empty errorCode values', () => {
    expect(
      resolveResponseErrorCode({ error: 'x', errorCode: '' })
    ).toBeUndefined()
    expect(
      resolveResponseErrorCode({ error: 'x', errorCode: 42 })
    ).toBeUndefined()
  })

  it('returns undefined for non-object bodies', () => {
    expect(resolveResponseErrorCode(undefined)).toBeUndefined()
    expect(resolveResponseErrorCode(null)).toBeUndefined()
    expect(resolveResponseErrorCode('error text')).toBeUndefined()
  })
})

describe('extractApiErrorBody — axios rejection shape', () => {
  it('extracts the unified error body from an axios-shaped rejection', () => {
    expect(
      extractApiErrorBody({
        response: { data: { error: 'invalid id', errorCode: 'invalidId' } },
      })
    ).toEqual({ error: 'invalid id', errorCode: 'invalidId' })
  })

  it('returns null when the failure carries no JSON body', () => {
    expect(extractApiErrorBody(new Error('Network Error'))).toBeNull()
    expect(extractApiErrorBody({ response: {} })).toBeNull()
    expect(extractApiErrorBody({ response: { data: 'not json' } })).toBeNull()
    expect(extractApiErrorBody(null)).toBeNull()
  })
})
