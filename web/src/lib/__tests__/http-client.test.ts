// metapi-go/lib — http-client 401 interceptor regression tests.
//
// Protects the simplified 401 contract: on a 401 the interceptor re-reads
// storage once (another tab may have replaced the token), replays at most
// once, then clears the session and redirects to sign-in when no valid token
// remains. No refresh endpoint or rotation facade is involved.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from '../http-client'

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
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

describe('apiClient 401 interceptor', () => {
  const originalAdapter = apiClient.defaults.adapter
  const originalLocation = window.location
  const replaceMock = vi.fn()

  function installLocationMock(): void {
    // jsdom's Location methods are non-configurable, so `vi.spyOn` cannot
    // redefine them. Replace the whole `window.location` value with a minimal
    // stub that exposes a spyable `replace`.
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {
        pathname: '/accounts',
        href: 'http://localhost/accounts',
        replace: replaceMock,
        reload: () => {},
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
    installLocationMock()
  })

  afterEach(() => {
    apiClient.defaults.adapter = originalAdapter
    restoreLocation()
    vi.unstubAllGlobals()
  })

  function installAlways401Adapter(adapterCalls: unknown[]): void {
    apiClient.defaults.adapter = async (config) => {
      adapterCalls.push(config)
      const error = new Error(
        'Request failed with status code 401'
      ) as AdapterError
      error.config = config
      error.response = { status: 401, data: { error: 'unauthorized' } }
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
    installAlways401Adapter(adapterCalls)

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    // Initial request + exactly one replay — never an infinite retry loop.
    expect(adapterCalls).toHaveLength(2)
    expect(replaceMock).toHaveBeenCalledWith('/sign-in')
    // The exhausted retry clears the persisted token.
    expect(storage.has(TOKEN_KEY)).toBe(false)
  })

  it('redirects without replay when no valid token remains', async () => {
    vi.stubGlobal('localStorage', memoryStorage() as unknown as Storage)

    const adapterCalls: unknown[] = []
    installAlways401Adapter(adapterCalls)

    await expect(apiClient.get('/api/protected')).rejects.toThrow()

    // anonymous → no replay, a single adapter call, redirect to sign-in.
    expect(adapterCalls).toHaveLength(1)
    expect(replaceMock).toHaveBeenCalledWith('/sign-in')
  })
})
