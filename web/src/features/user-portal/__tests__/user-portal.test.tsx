import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { AxiosError, type AxiosAdapter, type AxiosResponse } from 'axios'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import i18n from '@/i18n/config'
import { apiClient } from '@/lib/http-client'

import { userPortalApi } from '../api'
import { PortalAccount } from '../components/portal-account'
import { PortalKeys } from '../components/portal-keys'
import { PortalLogin } from '../components/portal-login'

const session = {
  user: { id: 1, username: 'person', displayName: 'Person', groupId: 2 },
  group: { id: 2, name: 'Standard' },
  expiresAt: '2099-01-01T00:00:00Z',
}
const originalAdapter = apiClient.defaults.adapter
let client: QueryClient
let handler: (
  url: string,
  method: string,
  body: Record<string, unknown>
) => unknown
let calls: Array<{ url: string; method: string; body: Record<string, unknown> }>

beforeEach(async () => {
  await i18n.changeLanguage('en')
  calls = []
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  handler = () => ({})
  const adapter: AxiosAdapter = async (config) => {
    const url = config.url ?? ''
    const method = config.method ?? 'get'
    const body = typeof config.data === 'string' ? JSON.parse(config.data) : {}
    calls.push({ url, method, body })
    const data = await handler(url, method, body)
    if (data instanceof Error) {
      if (data instanceof AxiosError) data.config = config
      throw data
    }
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  }
  apiClient.defaults.adapter = adapter
  Element.prototype.scrollIntoView = vi.fn()
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
    })),
  })
})
afterEach(() => {
  cleanup()
  client.clear()
  apiClient.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
})
function mount(children: ReactNode) {
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider defaultTheme='light'>
        <ThemeCustomizationProvider>{children}</ThemeCustomizationProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

describe('personal portal boundaries', () => {
  it('requires matching confirmation before changing a password and sends no confirmation field', async () => {
    mount(<PortalAccount session={session} />)
    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'current-test-password' },
    })
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'new-test-password' },
    })
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'different-test-password' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Change password' }))
    expect(await screen.findByText('The passwords do not match.')).toBeVisible()
    expect(calls).toHaveLength(0)
  })
  it('prevents closing a key creation while its one-time response is pending', async () => {
    let finish!: (value: unknown) => void
    handler = (_url, method) =>
      method === 'post'
        ? new Promise((resolve) => {
            finish = resolve
          })
        : { items: [] }
    mount(<PortalKeys userId={1} />)
    fireEvent.click(screen.getByRole('button', { name: 'Create key' }))
    fireEvent.change(await screen.findByLabelText('Key name'), {
      target: { value: 'Fixture' },
    })
    fireEvent.click(
      screen.getAllByRole('button', { name: 'Create key' }).at(-1)!
    )
    await waitFor(() =>
      expect(calls.some((call) => call.method === 'post')).toBe(true)
    )
    expect(
      screen.queryByRole('button', { name: 'Close' })
    ).not.toBeInTheDocument()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(screen.getByRole('dialog')).toBeVisible()
    await act(async () =>
      finish({
        item: {
          id: 1,
          name: 'Fixture',
          enabled: true,
          keyMasked: 'fixture',
          expiresAt: null,
          supportedModels: [],
          maxCost: null,
          maxRequests: null,
          usedCost: 0,
          usedRequests: 0,
          createdAt: '',
        },
        key: 'fixture-once',
      })
    )
    expect(await screen.findByLabelText('Secret API key')).toHaveValue(
      'fixture-once'
    )
  })
  it('validates response shapes instead of turning broken responses into empty data', async () => {
    handler = () => ({ items: 'not-an-array' })
    await expect(userPortalApi.keys()).rejects.toThrow()
    await expect(userPortalApi.models()).rejects.toThrow()
    await expect(userPortalApi.requests(1)).rejects.toThrow()
    await expect(userPortalApi.budget()).rejects.toThrow()
  })
  it('treats a user 401 as anonymous without invoking administrator session handling', async () => {
    localStorage.setItem(
      'metapi_session_meta',
      JSON.stringify({ expiresAtMs: 9999999999999 })
    )
    handler = () =>
      new AxiosError('unauthorized', 'ERR_BAD_REQUEST', undefined, undefined, {
        status: 401,
        data: { error: 'Unauthenticated' },
      } as AxiosResponse)
    await expect(userPortalApi.session()).resolves.toBeNull()
    expect(localStorage.getItem('metapi_session_meta')).toContain(
      '9999999999999'
    )
  })
  it('activates using a body-only code and clears all credential inputs after success', async () => {
    handler = (url) =>
      url === '/api/user/session' ? session : { success: true }
    mount(<PortalLogin />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Activate or reset my account' })
    )
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'person' },
    })
    fireEvent.change(screen.getByLabelText('Activation / reset code'), {
      target: { value: 'synthetic-one-time-code' },
    })
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'a-valid-test-password' },
    })
    await act(async () => {
      fireEvent.click(
        screen.getByRole('button', { name: 'Set password and sign in' })
      )
    })
    await waitFor(() =>
      expect(client.getQueryData(['userPortal', 'session'])).toEqual(session)
    )
    expect(
      calls.find((c) => c.url === '/api/user/auth/activate')?.body
    ).toEqual({
      username: 'person',
      code: 'synthetic-one-time-code',
      password: 'a-valid-test-password',
    })
    expect(screen.getByLabelText('New password')).toHaveValue('')
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((q) => q.state.data)
      )
    ).not.toContain('synthetic-one-time-code')
    expect(window.location.href).not.toContain('synthetic-one-time-code')
    expect(JSON.stringify(localStorage)).not.toContain('a-valid-test-password')
  })
  it('rejects invalid activation passwords and displays authentication failures', async () => {
    handler = () => new Error('fixture auth failed')
    mount(<PortalLogin />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Activate or reset my account' })
    )
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'person' },
    })
    fireEvent.change(screen.getByLabelText('Activation / reset code'), {
      target: { value: 'fixture-code' },
    })
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'short' },
    })
    await act(async () => {
      fireEvent.click(
        screen.getByRole('button', { name: 'Set password and sign in' })
      )
    })
    expect(await screen.findByRole('alert')).toHaveTextContent('12–72')
    expect(calls).toHaveLength(0)
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'a-valid-test-password' },
    })
    await act(async () => {
      fireEvent.click(
        screen.getByRole('button', { name: 'Set password and sign in' })
      )
    })
    expect(await screen.findByText(/Sign in failed/)).toBeVisible()
  })
  it('shows a generated Key once, never in Query caches, and discards it on close', async () => {
    const item = {
      id: 1,
      name: 'Editor',
      keyMasked: 'sk-••••fixture',
      enabled: true,
      expiresAt: null,
      supportedModels: [],
      maxCost: null,
      maxRequests: null,
      usedCost: 0,
      usedRequests: 0,
      createdAt: '2026-10-21T00:00:00Z',
    }
    let created = false
    handler = (url, method) => {
      if (url === '/api/user/keys' && method === 'post') {
        created = true
        return { item, key: 'synthetic-once-only-key' }
      }
      if (url === '/api/user/keys') return { items: created ? [item] : [] }
      return { usedRequests: 0, keyCount: 1 }
    }
    mount(<PortalKeys userId={1} />)
    expect(
      await screen.findByText(
        'No API keys yet. Create a key to connect your client.'
      )
    ).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Create key' }))
    fireEvent.change(await screen.findByLabelText('Key name'), {
      target: { value: 'Editor' },
    })
    await act(async () => {
      fireEvent.click(
        screen.getAllByRole('button', { name: 'Create key' }).at(-1)!
      )
    })
    expect(await screen.findByLabelText('Secret API key')).toHaveValue(
      'synthetic-once-only-key'
    )
    expect(screen.getByLabelText('Secret API key')).toHaveAttribute(
      'type',
      'password'
    )
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((q) => q.state.data)
      )
    ).not.toContain('synthetic-once-only-key')
    expect(client.getMutationCache().getAll()).toHaveLength(0)
    fireEvent.click(
      screen.getByRole('button', { name: 'I have saved the key' })
    )
    await waitFor(() =>
      expect(screen.queryByLabelText('Secret API key')).not.toBeInTheDocument()
    )
    expect(await screen.findByText('Editor')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Create key' }))
    expect(await screen.findByLabelText('Key name')).toHaveValue('')
    expect(
      screen.queryByDisplayValue('synthetic-once-only-key')
    ).not.toBeInTheDocument()
  })
  it('does not render failed Key fetches as empty and can retry', async () => {
    handler = () => new Error('offline')
    mount(<PortalKeys userId={1} />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load')
    expect(
      screen.queryByText(
        'No API keys yet. Create a key to connect your client.'
      )
    ).not.toBeInTheDocument()
    handler = () => ({ items: [] })
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByText(
        'No API keys yet. Create a key to connect your client.'
      )
    ).toBeVisible()
  })
  it('password change clears personal data and returns to the independent login state', async () => {
    handler = () => ({ success: true })
    client.setQueryData(['userPortal', 'session'], session)
    client.setQueryData(['userPortal', 1, 'keys'], {
      items: [{ name: 'private' }],
    })
    mount(<PortalAccount session={session} />)
    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'current-test-password' },
    })
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'new-test-password' },
    })
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'new-test-password' },
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Change password' }))
    })
    await waitFor(() =>
      expect(client.getQueryData(['userPortal', 'session'])).toBeNull()
    )
    expect(client.getQueryData(['userPortal', 1, 'keys'])).toBeUndefined()
    expect(
      calls.find((c) => c.url === '/api/user/auth/password')?.body
    ).toEqual({
      currentPassword: 'current-test-password',
      newPassword: 'new-test-password',
    })
  })
})
