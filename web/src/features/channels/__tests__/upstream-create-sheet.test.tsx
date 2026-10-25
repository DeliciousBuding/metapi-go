import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18n from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'
import type { UpstreamConnectResult } from '@/lib/api/upstream-catalog'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import { UpstreamCreateSheet } from '../components/upstream-create-sheet'

const mocks = vi.hoisted(() => ({
  connect: vi.fn(),
  list: vi.fn(),
  resolve: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ api: { connectUpstream: mocks.connect } }))
vi.mock('@/lib/api/upstream-presets', () => ({
  upstreamPresetsApi: {
    getUpstreamPresets: mocks.list,
    resolveUpstreamPreset: mocks.resolve,
  },
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
    id: 'deepseek-openai',
    name: 'DeepSeek',
    label: 'DeepSeek',
    provider: 'deepseek',
    protocols: ['chat'],
  },
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
  {
    ...base,
    id: 'vertex-gemini',
    name: 'Google Vertex AI',
    label: 'Google Vertex AI',
    provider: 'gemini_vertex',
    group: 'other',
    defaultUrl: 'https://us-central1-aiplatform.googleapis.com',
    credentialMode: 'gcpServiceAccount' as const,
    requiresProject: true,
    protocols: ['gemini', 'geminiEmbeddings'] as UpstreamPreset['protocols'],
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
function editParameterJSON(value: string) {
  const item = screen
    .getByLabelText('Parameter overrides')
    .closest<HTMLElement>('[data-slot="form-item"]')
  if (!item) throw new Error('Parameter form field is missing')
  fireEvent.click(within(item).getByRole('button', { name: 'Edit JSON' }))
  fireEvent.change(screen.getByLabelText('Parameter overrides'), {
    target: { value },
  })
}
function fillRequestConfig() {
  fireEvent.click(screen.getByLabelText('Request headers'))
  fireEvent.change(screen.getByLabelText('Header name'), {
    target: { value: 'X-Fixture' },
  })
  fireEvent.change(screen.getByLabelText('Value'), {
    target: { value: 'synthetic-header-value' },
  })
  editParameterJSON('{"metadata":{"label":"synthetic-param"}}')
}
beforeEach(async () => {
  await i18n.changeLanguage('en')
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  mocks.list.mockResolvedValue({ items: presets })
  mocks.connect.mockResolvedValue(result)
  mocks.resolve.mockResolvedValue({
    presetId: 'deepseek-openai',
    provider: 'deepseek',
    baseUrl: 'https://upstream.example',
    endpointConfig: {
      chat: {
        url: 'https://upstream.example/chat',
        auth: 'bearer',
        profile: 'deepseek',
      },
    },
  })
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('platform quick connect', () => {
  it('shows a loading state, reports a failed preset fetch and retries explicitly', async () => {
    let fail!: (error: Error) => void
    mocks.list.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          fail = reject
        })
    )
    setup()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Loading platform presets'
    )
    expect(screen.getByRole('button', { name: 'Connect' })).toBeDisabled()
    await act(async () => fail(new Error('fixture unavailable')))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not load platform presets'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await choose('DeepSeek')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
  it('shows an explicit empty preset state', async () => {
    mocks.list.mockResolvedValue({ items: [] })
    setup()
    expect(await screen.findByText('No matching platforms')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Connect' })).toBeDisabled()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
  it('requires a key for API-key platforms without revealing advanced controls', async () => {
    setup()
    await choose('Alibaba Cloud Bailian')
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    expect(await screen.findByText('This field is required.')).toBeVisible()
    expect(mocks.connect).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Advanced settings' })
    ).toHaveAttribute('aria-expanded', 'false')
  })
  it('shows all default protocols and connects them without reading or caching credentials', async () => {
    const { onCreated } = setup()
    await choose('Alibaba Cloud Bailian')
    const defaults = within(
      screen.getByRole('group', { name: 'Preset default APIs' })
    )
    for (const name of ['Chat', 'Responses', 'Messages']) {
      expect(defaults.getByRole('checkbox', { name })).toBeChecked()
    }
    expect(
      screen.queryByRole('combobox', { name: 'Protocol adapter' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('textbox', { name: 'Base URL' })
    ).not.toBeInTheDocument()
    expect(screen.getByLabelText('API Key')).toHaveAttribute('type', 'password')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'bailian',
        protocols: ['chat', 'responses', 'messages'],
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
  it('requires the self-hosted New API address and sends the selected default protocols', async () => {
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
        protocols: ['chat', 'responses', 'messages'],
        apiKey: 'synthetic-platform-key',
        baseUrl: 'https://newapi.example',
      })
    )
  })
  it('allows a default protocol subset while keeping at least one selected', async () => {
    setup()
    await choose('Alibaba Cloud Bailian')
    key()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Responses' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Messages' }))
    expect(screen.getByRole('checkbox', { name: 'Chat' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Chat' })).toHaveAttribute(
      'aria-disabled',
      'true'
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Chat' }))
    expect(screen.getByRole('checkbox', { name: 'Chat' })).toBeChecked()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Messages' }))
    expect(screen.getByRole('checkbox', { name: 'Chat' })).not.toHaveAttribute(
      'aria-disabled',
      'true'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'bailian',
        protocols: ['chat', 'messages'],
        apiKey: 'synthetic-platform-key',
      })
    )
  })
  it('preserves a failed key for retry and prevents duplicate submissions or closing while connecting', async () => {
    mocks.connect.mockRejectedValueOnce(new Error('unavailable'))
    const { onClose } = setup()
    await choose('Alibaba Cloud Bailian')
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
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'ollama-native',
        protocols: ['chat', 'responses', 'messages'],
      })
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
    await choose('Alibaba Cloud Bailian')
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
    await choose('Alibaba Cloud Bailian')
    key('first-platform-key')
    fireEvent.click(screen.getByRole('checkbox', { name: 'Responses' }))
    fireEvent.click(screen.getByRole('button', { name: 'Change' }))
    await choose('Alibaba Cloud Coding Plan')
    expect(screen.getByLabelText('API Key')).toHaveValue('')
    expect(screen.getByRole('checkbox', { name: 'Responses' })).toBeChecked()
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
        protocols: ['chat', 'responses', 'messages'],
        apiKey: 'second-platform-key',
        name: 'My coding plan',
        useSystemProxy: true,
      })
    )
  })
})

describe('custom protocols during connection', () => {
  function addEndpoint(name: string, url: string) {
    fireEvent.click(screen.getByRole('checkbox', { name }))
    fireEvent.change(
      within(screen.getByRole('group', { name })).getByLabelText(
        'Endpoint URL'
      ),
      {
        target: { value: url },
      }
    )
  }

  it('adds Responses and Messages beyond a Chat-only template and retains adapters when switching modes', async () => {
    setup()
    await choose('DeepSeek')
    key()
    expect(
      screen.queryByRole('checkbox', { name: 'Responses' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    expect(
      await screen.findByRole('button', { name: 'Configure Chat endpoint' })
    ).toBeVisible()
    expect(mocks.resolve).toHaveBeenCalledWith(
      {
        presetId: 'deepseek-openai',
        baseUrl: base.defaultUrl,
      },
      expect.any(AbortSignal)
    )
    addEndpoint('Responses', 'https://upstream.example/v1/responses')
    addEndpoint('Messages', 'https://upstream.example/v1/messages')
    fireEvent.click(screen.getByRole('button', { name: 'Use preset defaults' }))
    expect(screen.getByRole('checkbox', { name: 'Chat' })).toBeChecked()
    expect(
      screen.queryByRole('checkbox', { name: 'Messages' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    expect(screen.getByRole('checkbox', { name: 'Responses' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Messages' })).toBeChecked()
    expect(mocks.resolve).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    const endpointConfig: ImportedEndpointConfig = {
      chat: {
        url: 'https://upstream.example/chat',
        auth: 'bearer',
        profile: 'deepseek',
      },
      responses: {
        url: 'https://upstream.example/v1/responses',
        auth: 'bearer',
      },
      messages: {
        url: 'https://upstream.example/v1/messages',
        auth: 'x-api-key',
      },
    }
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'deepseek-openai',
        apiKey: 'synthetic-platform-key',
        endpointConfig,
      })
    )
    expect(client.getMutationCache().getAll()).toHaveLength(0)
  })

  it('rejects an empty custom configuration instead of connecting with hidden defaults', async () => {
    setup()
    await choose('DeepSeek')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    await screen.findByRole('button', { name: 'Configure Chat endpoint' })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Chat' }))
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Configure at least one API endpoint.'
    )
    expect(
      screen.getByRole('group', { name: 'Custom API endpoints' })
    ).toHaveAttribute('aria-invalid', 'true')
    expect(mocks.connect).not.toHaveBeenCalled()
  })

  it('keeps custom mode and input after resolve fails, then allows an explicit retry', async () => {
    mocks.resolve.mockRejectedValueOnce(new Error('fixture unavailable'))
    setup()
    await choose('DeepSeek')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not prepare the endpoints.'
    )
    expect(screen.getByLabelText('API Key')).toHaveValue(
      'synthetic-platform-key'
    )
    expect(
      screen.getByRole('group', { name: 'Custom API endpoints' })
    ).toBeVisible()
    expect(mocks.connect).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Load selected preset endpoints' })
    )
    expect(
      await screen.findByRole('button', { name: 'Configure Chat endpoint' })
    ).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(mocks.resolve).toHaveBeenCalledTimes(2)
  })
  it('preserves manual endpoints across mode switches after preset initialization fails', async () => {
    mocks.resolve.mockRejectedValueOnce(new Error('fixture unavailable'))
    setup()
    await choose('DeepSeek')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    await screen.findByRole('alert')
    addEndpoint('Responses', 'https://manual.example/responses')
    fireEvent.click(screen.getByRole('button', { name: 'Use preset defaults' }))
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    expect(mocks.resolve).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('checkbox', { name: 'Chat' })).not.toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Responses' })).toBeChecked()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'deepseek-openai',
        apiKey: 'synthetic-platform-key',
        endpointConfig: {
          responses: {
            url: 'https://manual.example/responses',
            auth: 'bearer',
          },
        },
      })
    )
  })

  it('discards a pending preset response and all credentials when switching products', async () => {
    let finish!: (value: unknown) => void
    mocks.resolve.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        })
    )
    setup()
    await choose('Alibaba Cloud Bailian')
    key('synthetic-first-key')
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    await waitFor(() => expect(mocks.resolve).toHaveBeenCalledTimes(1))
    expect(screen.getByRole('button', { name: 'Connect' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Change' }))
    await choose('DeepSeek')
    expect(screen.getByLabelText('API Key')).toHaveValue('')
    expect(mocks.resolve.mock.calls[0][1].aborted).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    await screen.findByRole('button', { name: 'Configure Chat endpoint' })
    await act(async () =>
      finish({
        presetId: 'bailian',
        provider: 'bailian',
        baseUrl: base.defaultUrl,
        endpointConfig: {
          chat: { url: 'https://old.example/chat', auth: 'bearer' },
          messages: { url: 'https://old.example/messages', auth: 'x-api-key' },
        },
      })
    )
    expect(screen.getByRole('checkbox', { name: 'Messages' })).not.toBeChecked()
    expect(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    ).toHaveTextContent('https://upstream.example/chat')
    key('synthetic-second-key')
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'deepseek-openai',
        apiKey: 'synthetic-second-key',
        endpointConfig: {
          chat: {
            url: 'https://upstream.example/chat',
            auth: 'bearer',
            profile: 'deepseek',
          },
        },
      })
    )
  })

  it('updates protocol controls and descriptions when the language changes', async () => {
    setup()
    await choose('DeepSeek')
    await act(async () => {
      await i18n.changeLanguage('zhCN')
    })
    expect(screen.getByRole('group', { name: '预设默认接口' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '自定义端点' }))
    expect(
      await screen.findByRole('button', { name: '配置 Chat 端点' })
    ).toBeVisible()
    expect(screen.getByRole('group', { name: '自定义 API 端点' })).toBeVisible()
    expect(
      screen.getByRole('button', { name: '使用预设默认接口' })
    ).toBeVisible()
    expect(screen.getByText(/可添加模板以外的接口/)).toBeVisible()
  })
})

describe('advanced request configuration during connection', () => {
  it('submits headers and parameters in their original API formats, retains failure drafts and clears successful secrets', async () => {
    mocks.connect.mockRejectedValueOnce(new Error('fixture unavailable'))
    setup()
    await choose('DeepSeek')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fillRequestConfig()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'deepseek-openai',
        protocols: ['chat'],
        apiKey: 'synthetic-platform-key',
        customHeaders:
          '[{"header_key":"X-Fixture","header_value":"synthetic-header-value"}]',
        paramOverride: '{"metadata":{"label":"synthetic-param"}}',
      })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Connection failed'
    )
    expect(screen.getByDisplayValue('synthetic-header-value')).toBeVisible()
    expect(screen.getByLabelText('Parameter overrides')).toHaveValue(
      '{"metadata":{"label":"synthetic-param"}}'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2))
    await waitFor(() =>
      expect(
        screen.queryByDisplayValue('synthetic-header-value')
      ).not.toBeInTheDocument()
    )
    expect(screen.getByLabelText('Parameter overrides')).toHaveValue('')
    const cached = JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((entry) => entry.state.data)
    )
    expect(cached).not.toContain('synthetic-header-value')
    expect(cached).not.toContain('synthetic-param')
    expect(client.getMutationCache().getAll()).toHaveLength(0)
  })

  it('reveals invalid parameter input after submitting from a collapsed advanced section', async () => {
    setup()
    await choose('DeepSeek')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    editParameterJSON('[]')
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Advanced settings' })
      ).toHaveAttribute('aria-expanded', 'true')
    )
    expect(screen.getByLabelText('Parameter overrides')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(mocks.connect).not.toHaveBeenCalled()
  })

  it('clears request configuration and proxy drafts when changing the product', async () => {
    setup()
    await choose('DeepSeek')
    key()
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fillRequestConfig()
    fireEvent.change(screen.getByLabelText('Channel proxy'), {
      target: { value: 'http://synthetic-user:synthetic-pass@proxy.example' }, // # leak-guard-allow:LG-F4C6A155 synthetic fixture checks proxy credential removal
    })
    fireEvent.click(screen.getByRole('button', { name: 'Change' }))
    await choose('Alibaba Cloud Bailian')
    expect(screen.getByLabelText('API Key')).toHaveValue('')
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    expect(screen.getByLabelText('Channel proxy')).toHaveValue('')
    expect(
      screen.queryByDisplayValue('synthetic-header-value')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByDisplayValue('{"metadata":{"label":"synthetic-param"}}')
    ).not.toBeInTheDocument()
  })
})

describe('vertex preset connect', () => {
  async function chooseVertex() {
    fireEvent.click(await screen.findByRole('tab', { name: 'More' }))
    await choose('Google Vertex AI')
  }
  it('collects project and service-account material instead of an API key', async () => {
    setup()
    await chooseVertex()
    expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    expect(screen.getByLabelText(/GCP project/i)).toBeInTheDocument()
    const material = '{"type":"service_account","client_email":"a@b.iam"}'
    fireEvent.change(screen.getByLabelText(/GCP project/i), {
      target: { value: 'my-project' },
    })
    fireEvent.change(
      screen.getByPlaceholderText('{"type":"service_account",...}'),
      { target: { value: material } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'vertex-gemini',
        protocols: ['gemini', 'geminiEmbeddings'],
        gcpServiceAccount: material,
        projectId: 'my-project',
      })
    )
    expect(
      screen.getByPlaceholderText('{"type":"service_account",...}')
    ).toHaveValue('')
    expect(client.getMutationCache().getAll()).toHaveLength(0)
  })
  it('requires project and material before connecting', async () => {
    setup()
    await chooseVertex()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    expect(await screen.findAllByText('This field is required.')).toHaveLength(
      2
    )
    expect(mocks.connect).not.toHaveBeenCalled()
  })
  it('uses project only to prepare defaults and submits custom Vertex URLs without a project requirement', async () => {
    const endpoints: ImportedEndpointConfig = {
      gemini: {
        url: 'https://custom-vertex.example/google/models',
        auth: 'bearer',
        profile: 'vertex',
        modelPath: true,
      },
      geminiEmbeddings: {
        url: 'https://custom-vertex.example/google/models',
        auth: 'bearer',
        profile: 'vertex',
        modelPath: true,
      },
    }
    mocks.resolve.mockResolvedValue({
      presetId: 'vertex-gemini',
      provider: 'gemini_vertex',
      baseUrl: 'https://custom-vertex.example',
      endpointConfig: endpoints,
    })
    setup()
    await chooseVertex()
    fireEvent.change(screen.getByLabelText('GCP project'), {
      target: { value: 'synthetic-project' },
    })
    const material =
      '{"type":"service_account","client_email":"synthetic@example.invalid"}'
    fireEvent.change(screen.getByLabelText('GCP service account'), {
      target: { value: material },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Customize endpoints' }))
    await screen.findByRole('button', { name: 'Configure Gemini endpoint' })
    fireEvent.change(
      screen.getByLabelText('GCP project (for preset endpoints)'),
      { target: { value: '' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: 'vertex-gemini',
        gcpServiceAccount: material,
        endpointConfig: endpoints,
      })
    )
  })
})
