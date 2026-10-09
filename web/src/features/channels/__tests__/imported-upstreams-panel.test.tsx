import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ImportedUpstreamInventory } from '@/lib/api'

import { ImportedUpstreamsPanel } from '../components/imported-upstreams-panel'

const state = vi.hoisted(() => ({
  get: vi.fn(),
  patch: vi.fn(),
  detail: vi.fn(),
  credentials: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    getImportedUpstreams: state.get,
    setImportedUpstreamEnabled: state.patch,
    getImportedUpstream: state.detail,
    getImportedCredentials: state.credentials,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: { success: state.success, error: state.error },
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => (
    <a href='/token-routes'>{children}</a>
  ),
}))
let client: QueryClient
function mount() {
  client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  render(
    <QueryClientProvider client={client}>
      <ImportedUpstreamsPanel />
    </QueryClientProvider>
  )
}
function inventory(enabled = true): ImportedUpstreamInventory {
  return {
    items: [
      {
        id: 4,
        name: 'Example channel',
        originKey: 'fixture',
        dialect: 'generic',
        baseUrl: 'https://upstream.example/v1',
        enabled,
        credentialCount: 2,
        modelCount: 1,
      },
    ],
    members: [
      {
        id: 7,
        channelId: 4,
        groupId: 8,
        groupName: 'client-model',
        routeId: 9,
        modelName: 'gpt-6-sol',
        credentialName: 'primary',
        credentialEnabled: true,
        protocols: 14,
        mode: 'failover',
        activeItemId: 7,
        priority: 0,
        weight: 1,
      },
    ],
  }
}
beforeEach(() => {
  vi.clearAllMocks()
  state.get.mockResolvedValue(inventory())
  state.detail.mockImplementation(async () => ({
    ...inventory().items[0],
    provider: 'openai',
    useSystemProxy: false,
  }))
  state.credentials.mockResolvedValue({ items: [] })
})
afterEach(() => {
  cleanup()
  client?.clear()
})
describe('imported upstream inventory', () => {
  it('shows source models and authorized protocols without credentials', async () => {
    mount()
    expect(await screen.findByText('Example channel')).toBeInTheDocument()
    expect(screen.getByText('Chat → Responses → Messages')).toBeInTheDocument()
    expect(screen.getByText('gpt-6-sol')).toBeInTheDocument()
    expect(screen.getByRole('region')).toHaveAttribute('tabindex', '0')
  })
  it('keeps the upstream workspace accessible before the first upstream is created', async () => {
    state.get.mockResolvedValue({ items: [], members: [] })
    mount()
    await waitFor(() => expect(state.get).toHaveBeenCalledTimes(1))
    expect(screen.getByRole('region')).toBeInTheDocument()
    expect(
      screen.getByRole('tab', { name: /Independent upstreams\s*0/ })
    ).toBeInTheDocument()
  })
  it('shows each protocol destination when a channel uses separate endpoints', async () => {
    const data = inventory()
    data.items[0].endpointConfig = {
      chat: { url: 'https://chat.example/custom', auth: 'bearer' },
      messages: { url: 'https://messages.example/native', auth: 'x-api-key' },
    }
    data.members[0].protocols = 30
    state.get.mockResolvedValue(data)
    state.detail.mockResolvedValue({
      ...data.items[0],
      provider: 'openai',
      useSystemProxy: false,
    })
    mount()
    fireEvent.click(await screen.findByRole('button', { name: 'Manage' }))
    expect(
      await screen.findByDisplayValue('https://chat.example/custom')
    ).toBeVisible()
    expect(
      screen.getByDisplayValue('https://messages.example/native')
    ).toBeVisible()
    expect(screen.getByText('x-api-key')).toBeVisible()
    expect(
      screen.getAllByText('Chat → Responses → Messages → Gemini').length
    ).toBeGreaterThan(0)
  })
  it('reads back availability after a confirmed update', async () => {
    state.patch.mockImplementation(async () => {
      state.get.mockResolvedValue(inventory(false))
      return { success: true, id: 4, enabled: false }
    })
    mount()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Disable Example channel' })
    )
    expect(
      await screen.findByRole('button', { name: 'Enable Example channel' })
    ).toBeInTheDocument()
    expect(state.patch).toHaveBeenCalledWith(4, false)
    expect(state.get).toHaveBeenCalledTimes(2)
  })
  it('shows route-specific protocol choices rather than the shared grant union', async () => {
    const data = inventory()
    data.members[0].protocols = 30
    data.members[0].protocolOrder = [8, 4]
    state.get.mockResolvedValue(data)
    mount()
    expect(await screen.findByText('Messages → Responses')).toBeInTheDocument()
    expect(
      screen.queryByText('Chat → Responses → Messages → Gemini')
    ).not.toBeInTheDocument()
  })
  it('keeps the previous enabled state when a write fails', async () => {
    state.patch.mockRejectedValue(new Error('fixture write failed'))
    mount()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Disable Example channel' })
    )
    await waitFor(() => expect(state.patch).toHaveBeenCalled())
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Disable Example channel' })
      ).toBeEnabled()
    )
    expect(
      screen.getByRole('button', { name: 'Disable Example channel' })
    ).toBeInTheDocument()
    expect(state.success).not.toHaveBeenCalled()
  })
})
