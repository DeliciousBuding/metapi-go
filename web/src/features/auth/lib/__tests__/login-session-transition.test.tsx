import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  bootstrapAuthentication,
  clearAuthentication,
  hasValidAuthSession,
  persistSessionMeta,
  readSessionMeta,
  wasAuthSessionExpiredOnLastBoot,
} from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

import { useLogin } from '../../api'

const api = vi.hoisted(() => ({ login: vi.fn() }))
vi.mock('@/lib/api/session', () => ({ sessionApi: api }))

let client: QueryClient
beforeEach(() => {
  api.login.mockReset()
  clearAuthentication()
  useAuthStore.getState().auth.reset()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: true,
      json: async () => ({ authenticated: false }),
    }))
  )
})
afterEach(() => {
  cleanup()
  client.clear()
  clearAuthentication()
  vi.unstubAllGlobals()
})
function wrapper(props: PropsWithChildren) {
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

// Exercise the real login mutation together with the real synchronous route
// guard. A mocked navigate callback alone cannot detect a login redirect loop.
describe('login session transition', () => {
  it('admits protected routes immediately after an anonymous boot and successful login', async () => {
    await bootstrapAuthentication()
    expect(hasValidAuthSession()).toBe(false)
    const expiresAtMs = Date.now() + 60_000
    api.login.mockResolvedValue({
      authenticated: true,
      expiresAt: new Date(expiresAtMs).toISOString(),
    })
    const login = renderHook(() => useLogin(), { wrapper })

    await act(async () => {
      await login.result.current.mutateAsync({ token: 'fixture-admin-token' })
    })

    expect(hasValidAuthSession()).toBe(true)
    expect(useAuthStore.getState().auth.sessionExpiresAt).toBe(expiresAtMs)
    expect(readSessionMeta()).toEqual({ expiresAtMs })
    expect(localStorage.getItem('auth_token')).toBeNull()
  })

  it('replaces a revoked boot outcome on re-login instead of preserving the expired redirect', async () => {
    persistSessionMeta(Date.now() + 60_000)
    await bootstrapAuthentication()
    expect(wasAuthSessionExpiredOnLastBoot()).toBe(true)
    api.login.mockResolvedValue({
      authenticated: true,
      expiresAt: new Date(Date.now() + 120_000).toISOString(),
    })
    const login = renderHook(() => useLogin(), { wrapper })

    await act(async () => {
      await login.result.current.mutateAsync({ token: 'fixture-admin-token' })
    })

    expect(hasValidAuthSession()).toBe(true)
    expect(wasAuthSessionExpiredOnLastBoot()).toBe(false)
    clearAuthentication()
    expect(hasValidAuthSession()).toBe(false)
  })

  it('does not authenticate the guard when the server rejects login', async () => {
    await bootstrapAuthentication()
    api.login.mockRejectedValue({
      messageKey: 'errors.login.invalidToken',
      status: 401,
    })
    const login = renderHook(() => useLogin(), { wrapper })

    await act(async () => {
      await expect(
        login.result.current.mutateAsync({ token: 'rejected-fixture-token' })
      ).rejects.toMatchObject({ status: 401 })
    })

    expect(hasValidAuthSession()).toBe(false)
    expect(readSessionMeta()).toBeNull()
  })
})
