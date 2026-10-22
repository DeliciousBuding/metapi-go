import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { AxiosAdapter } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import i18n from '@/i18n/config'
import { apiClient } from '@/lib/http-client'

import { userPortalApi } from '../api'
import { PortalKeys } from '../components/portal-keys'

const item = {
  id: 17,
  name: 'Editor',
  keyMasked: 'sk-old****abcd',
  enabled: false,
  expiresAt: null,
  supportedModels: ['fixture-model'],
  maxCost: null,
  maxRequests: 25,
  usedCost: 0,
  usedRequests: 7,
  createdAt: '2026-10-21T00:00:00Z',
}
const rotatedItem = { ...item, keyMasked: 'sk-new****dcba' }
const secret = 'synthetic-rotation-once-only'
const originalAdapter = apiClient.defaults.adapter
let client: QueryClient
let calls: Array<{ url: string; method: string; body: unknown }>
let rotateResponse: () => Promise<unknown>
let rotated: boolean

beforeEach(async () => {
  await i18n.changeLanguage('en')
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  calls = []
  rotated = false
  rotateResponse = async () => {
    rotated = true
    return { item: rotatedItem, key: secret }
  }
  const adapter: AxiosAdapter = async (config) => {
    const url = config.url ?? ''
    calls.push({
      url,
      method: config.method ?? 'get',
      body:
        typeof config.data === 'string' ? JSON.parse(config.data) : undefined,
    })
    const data = url.endsWith('/rotate')
      ? await rotateResponse()
      : { items: [rotated ? rotatedItem : item] }
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

function mount() {
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider defaultTheme='light'>
        <ThemeCustomizationProvider>
          <PortalKeys userId={1} />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

async function openRotation() {
  fireEvent.click(
    await screen.findByRole('button', { name: 'Rotate key Editor' })
  )
  return screen.findByRole('button', { name: 'Replace secret now' })
}

function assertSecretNotStored() {
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((q) => q.state.data)
    )
  ).not.toContain(secret)
  expect(client.getMutationCache().getAll()).toHaveLength(0)
  expect(JSON.stringify(localStorage)).not.toContain(secret)
  expect(JSON.stringify(sessionStorage)).not.toContain(secret)
  expect(window.location.href).not.toContain(secret)
}

describe('personal key rotation', () => {
  it('explains immediate replacement without promising reactivation, and cancels before sending', async () => {
    mount()
    await openRotation()
    expect(
      screen.getByText(/old secret stops working immediately/)
    ).toBeVisible()
    expect(
      screen.getByText(
        /does not reset quota, change expiry or enable a disabled key/
      )
    ).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(calls.filter((c) => c.method === 'post')).toHaveLength(0)
  })

  it('submits once, refreshes masked metadata and discards the one-time secret on close', async () => {
    let finish!: (value: unknown) => void
    rotateResponse = () =>
      new Promise((resolve) => {
        finish = resolve
      })
    mount()
    const confirm = await openRotation()
    fireEvent.click(confirm)
    fireEvent.click(confirm)
    await waitFor(() =>
      expect(calls.filter((c) => c.method === 'post')).toEqual([
        { url: '/api/user/keys/17/rotate', method: 'post', body: {} },
      ])
    )
    expect(
      screen.getByRole('button', { name: 'Replacing secret…' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(
      screen.queryByRole('button', { name: 'Close' })
    ).not.toBeInTheDocument()
    await act(async () => {
      rotated = true
      finish({ item: rotatedItem, key: secret })
    })
    expect(await screen.findByLabelText('Secret API key')).toHaveValue(secret)
    expect(screen.getByLabelText('Secret API key')).toHaveAttribute(
      'type',
      'password'
    )
    expect(await screen.findByText(rotatedItem.keyMasked)).toBeInTheDocument()
    assertSecretNotStored()
    fireEvent.click(
      screen.getByRole('button', { name: 'I have saved the key' })
    )
    await waitFor(() =>
      expect(screen.queryByLabelText('Secret API key')).not.toBeInTheDocument()
    )
    await openRotation()
    expect(screen.queryByDisplayValue(secret)).not.toBeInTheDocument()
    expect(calls.filter((c) => c.method === 'post')).toHaveLength(1)
    assertSecretNotStored()
  })

  it('shows uncertain failures without claiming the old secret survived, then allows retry', async () => {
    rotateResponse = async () => {
      throw new Error('synthetic lost response')
    }
    mount()
    fireEvent.click(await openRotation())
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'The secret may already have changed'
    )
    expect(screen.queryByLabelText('Secret API key')).not.toBeInTheDocument()
    rotateResponse = async () => ({ item: rotatedItem, key: secret })
    fireEvent.click(screen.getByRole('button', { name: 'Replace secret now' }))
    expect(await screen.findByLabelText('Secret API key')).toHaveValue(secret)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    assertSecretNotStored()
  })

  it('discards a late rotation response after the portal unmounts', async () => {
    let finish!: (value: unknown) => void
    rotateResponse = () =>
      new Promise((resolve) => {
        finish = resolve
      })
    const view = mount()
    fireEvent.click(await openRotation())
    await waitFor(() =>
      expect(calls.some((c) => c.method === 'post')).toBe(true)
    )
    view.unmount()
    await act(async () => {
      finish({ item: rotatedItem, key: secret })
    })
    expect(screen.queryByDisplayValue(secret)).not.toBeInTheDocument()
    assertSecretNotStored()
  })

  it('rejects malformed rotation responses instead of showing success', async () => {
    rotateResponse = async () => ({ item: rotatedItem })
    await expect(userPortalApi.rotateKey(17)).rejects.toThrow()
  })
})
