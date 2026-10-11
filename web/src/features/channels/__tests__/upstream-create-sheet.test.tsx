import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { UpstreamConnectResult } from '@/lib/api/upstream-catalog'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import { UpstreamCreateSheet } from '../components/upstream-create-sheet'

const mocks = vi.hoisted(() => ({
  connect: vi.fn(),
  list: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ api: { connectUpstream: mocks.connect } }))
vi.mock('@/lib/api/upstream-presets', () => ({
  upstreamPresetsApi: { getUpstreamPresets: mocks.list },
}))
vi.mock('@/lib/toast', () => ({
  toast: { success: mocks.success, info: mocks.info },
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    ...props
  }: {
    to: string
    children: React.ReactNode
  }) => (
    <a href={to} {...props}>
      {children}
    </a>
  ),
}))
const base = {
  platform: 'openai',
  provider: 'bailian',
  group: 'domestic' as const,
  defaultUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  protocols: ['chat', 'responses', 'messages'] as UpstreamPreset['protocols'],
  recommendedModels: [],
  requiresBaseUrl: false,
  credentialMode: 'apiKey' as const,
}
const presets: UpstreamPreset[] = [
  {
    ...base,
    id: 'new-api-connection',
    name: 'New API',
    label: 'New API',
    provider: 'new-api',
    platform: 'new-api',
    group: 'gateway',
    defaultUrl: '',
    requiresBaseUrl: true,
  },
  { ...base, id: 'bailian', name: 'Bailian', label: 'Bailian' },
  {
    ...base,
    id: 'codingplan-openai',
    name: 'Bailian Coding Plan',
    label: 'Bailian Coding Plan',
    group: 'coding',
  },
  {
    ...base,
    id: 'ollama-native',
    name: 'Ollama',
    label: 'Ollama',
    provider: 'ollama',
    defaultUrl: 'http://localhost:11434',
    credentialMode: 'optional',
  },
  {
    ...base,
    id: 'codex',
    name: 'OpenAI Codex',
    label: 'OpenAI Codex',
    provider: 'codex',
    group: 'other',
    credentialMode: 'oauth',
  },
]
const result: UpstreamConnectResult = {
  id: 42,
  name: 'Bailian',
  enabled: true,
  ownership: 'native',
  modelCount: 3,
  routeCount: 3,
  discovery: { status: 'discovered' },
}
let client: QueryClient
function setup() {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  const onCreated = vi.fn()
  const onClose = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <UpstreamCreateSheet onCreated={onCreated} onClose={onClose} />
    </QueryClientProvider>
  )
  return { onCreated, onClose }
}
async function choose(name: string) {
  fireEvent.click(await screen.findByRole('button', { name: `Use ${name}` }))
}
function key(value = 'synthetic-platform-key') {
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value } })
}
beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  mocks.list.mockResolvedValue({ items: presets })
  mocks.connect.mockResolvedValue(result)
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('platform quick connect', () => {
  it('requires a key for API-key platforms without revealing advanced controls', async () => {
    setup()
    await choose('Bailian')
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    expect(await screen.findByText('This field is required.')).toBeVisible()
    expect(mocks.connect).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Advanced settings' })
    ).toHaveAttribute('aria-expanded', 'false')
  })
  it('connects a platform with only its key and never asks for protocol configuration', async () => {
    const { onCreated } = setup()
    await choose('Bailian')
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Protocol adapter' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('textbox', { name: 'Base URL' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Chat')).not.toBeInTheDocument()
    expect(screen.getByLabelText('API Key')).toHaveAttribute('type', 'password')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'bailian',
        apiKey: 'synthetic-platform-key',
      })
    )
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(42))
    expect(screen.getByLabelText('API Key')).toHaveValue('')
    expect(client.getMutationCache().getAll()).toHaveLength(0)
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((query) => query.state.data)
      )
    ).not.toContain('synthetic-platform-key')
  })
  it('requires the self-hosted New API address and sends no endpoint or permission selections', async () => {
    setup()
    await choose('New API')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    expect(await screen.findByText('This field is required.')).toBeVisible()
    expect(mocks.connect).not.toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('Base URL'), {
      target: { value: 'https://newapi.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'new-api-connection',
        apiKey: 'synthetic-platform-key',
        baseUrl: 'https://newapi.example',
      })
    )
  })
  it('preserves a failed key for retry and prevents duplicate submissions or closing while connecting', async () => {
    mocks.connect.mockRejectedValueOnce(new Error('unavailable'))
    const { onClose } = setup()
    await choose('Bailian')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Connection failed'
    )
    expect(screen.getByLabelText('API Key')).toHaveValue(
      'synthetic-platform-key'
    )
    let finish!: (value: UpstreamConnectResult) => void
    mocks.connect.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2))
    expect(screen.getByLabelText('API Key')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Connecting…' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onClose).not.toHaveBeenCalled()
    await act(async () => finish(result))
    expect(mocks.connect).toHaveBeenCalledTimes(2)
  })
  it('allows a local optional key without sending a fake secret', async () => {
    setup()
    await choose('Ollama')
    expect(screen.getByLabelText('API Key (optional)')).toHaveValue('')
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({ presetId: 'ollama-native' })
    )
  })
  it('routes OAuth products to the existing authorization flow', async () => {
    setup()
    fireEvent.change(
      screen.getByRole('textbox', { name: /Search platforms/ }),
      { target: { value: 'Codex' } }
    )
    await choose('OpenAI Codex')
    expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Authorize connection' })
    ).toHaveAttribute('href', '/oauth')
    expect(
      screen.queryByRole('button', { name: 'Connect' })
    ).not.toBeInTheDocument()
    expect(mocks.connect).not.toHaveBeenCalled()
  })
  it('distinguishes an empty discovery from a usable model configuration', async () => {
    mocks.connect.mockResolvedValue({
      ...result,
      modelCount: 0,
      routeCount: 0,
      discovery: { status: 'empty' },
    })
    const { onCreated } = setup()
    await choose('Bailian')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(42))
    expect(mocks.success).not.toHaveBeenCalled()
    expect(mocks.info).toHaveBeenCalledWith(
      'Connected. No models were discovered; review the model configuration.',
      { description: undefined }
    )
  })
  it('keeps optional naming and proxy controls in advanced settings and clears the key when switching platform', async () => {
    setup()
    await choose('Bailian')
    key('first-platform-key')
    fireEvent.click(screen.getByRole('button', { name: 'Change' }))
    await choose('Bailian Coding Plan')
    expect(screen.getByLabelText('API Key')).toHaveValue('')
    key('second-platform-key')
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.change(screen.getByLabelText('Name (optional)'), {
      target: { value: 'My coding plan' },
    })
    fireEvent.click(screen.getByRole('switch', { name: 'System proxy' }))
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'codingplan-openai',
        apiKey: 'second-platform-key',
        name: 'My coding plan',
        useSystemProxy: true,
      })
    )
  })
})
