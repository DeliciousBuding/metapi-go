// metapi-go/lib — http-client auth interceptor regression tests.
//
// Protects the session contract:
//   • on a 401 the interceptor re-reads storage once (another tab may have
//     replaced the token), replays at most once, then clears the session and
//     redirects to sign-in when no valid token remains;
//   • the redirect preserves the interrupted path via ?redirect=…;
//   • a 403 with the backend's "Invalid token" body is treated like a 401,
//     while an IP-allowlist 403 keeps the session intact.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from '../http-client'

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

  it('replays exactly once with a re-read token, then clears and redirects', async () => {
    const storage = memoryStorage({
      [TOKEN_KEY]: 'fresh-token',
      [EXPIRES_KEY]: String(Date.now() + 60_000),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)

    const adapterCalls: unknown[] = []
    installStatusAdapter(adapterCalls, 401, { error: 'unauthorized' })

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    // Initial request + exactly one replay — never an infinite retry loop.
    expect(adapterCalls).toHaveLength(2)
    // The interrupted path is preserved so sign-in can send the user back.
    expect(replaceMock).toHaveBeenCalledWith('/sign-in?redirect=%2Faccounts')
    // The exhausted retry clears the persisted token.
    expect(storage.has(TOKEN_KEY)).toBe(false)
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
      [TOKEN_KEY]: 'rotated-away-token',
      [EXPIRES_KEY]: String(Date.now() + 60_000),
    })
    vi.stubGlobal('localStorage', storage as unknown as Storage)

    const adapterCalls: unknown[] = []
    installStatusAdapter(adapterCalls, 403, { error: 'Invalid token' })

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    // One replay with the re-read token, then the session is cleared.
    expect(adapterCalls).toHaveLength(2)
    expect(storage.has(TOKEN_KEY)).toBe(false)
    expect(replaceMock).toHaveBeenCalledWith('/sign-in?redirect=%2Faccounts')
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
