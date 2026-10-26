import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18n from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import { UpstreamConnectionForm } from '../components/upstream-connection-form'

const mocks = vi.hoisted(() => ({
  update: vi.fn(),
  requestConfig: vi.fn(),
  list: vi.fn(),
  resolve: vi.fn(),
  grant: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    updateImportedUpstream: mocks.update,
    getImportedRequestConfig: mocks.requestConfig,
    updateUpstreamGrant: mocks.grant,
  },
}))
vi.mock('@/lib/api/upstream-presets', () => ({
  upstreamPresetsApi: {
    getUpstreamPresets: mocks.list,
    resolveUpstreamPreset: mocks.resolve,
  },
}))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn() } }))
const detail: ImportedUpstreamDetail = {
  id: 4,
  name: 'New API',
  provider: 'new-api',
  dialect: 'generic',
  originKey: '',
  enabled: true,
  baseUrl: 'https://gateway.example',
  endpointConfig: {
    chat: {
      url: 'https://gateway.example/v1/chat/completions',
      auth: 'bearer',
    },
  },
  useSystemProxy: false,
  hasChannelProxy: false,
  hasCustomHeaders: false,
  hasParamOverride: false,
  channelProxyDisplay: '',
  openaiChatCompletionPath: '',
  openaiResponsePath: '',
  anthropicMessagePath: '',
}
const preset: UpstreamPreset = {
  id: 'new-api-connection',
  name: 'New API',
  label: 'New API',
  provider: 'new-api',
  platform: 'new-api',
  group: 'gateway',
  credentialMode: 'apiKey',
  defaultUrl: '',
  requiresBaseUrl: true,
  protocols: ['chat', 'responses', 'messages'],
  recommendedModels: [],
}
const resolved = {
  presetId: preset.id,
  provider: preset.provider,
  baseUrl: detail.baseUrl,
  endpointConfig: {
    chat: {
      url: 'https://gateway.example/v1/chat/completions',
      auth: 'bearer' as const,
    },
    responses: {
      url: 'https://gateway.example/v1/responses',
      auth: 'bearer' as const,
    },
    messages: {
      url: 'https://gateway.example/v1/messages',
      auth: 'x-api-key' as const,
    },
  },
}
let client: QueryClient
beforeEach(async () => {
  await i18n.changeLanguage('en')
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  mocks.update.mockResolvedValue({ success: true })
  mocks.list.mockResolvedValue({ items: [preset] })
  mocks.resolve.mockResolvedValue(resolved)
})
afterEach(() => {
  cleanup()
  client?.clear()
})
function mount(
  overrides: Partial<ImportedUpstreamDetail> = {},
  onManageModels = vi.fn()
) {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  render(
    <QueryClientProvider client={client}>
      <UpstreamConnectionForm
        detail={{ ...detail, ...overrides }}
        onDirtyChange={vi.fn()}
        onManageModels={onManageModels}
      />
    </QueryClientProvider>
  )
  return { onManageModels }
}
describe('advanced upstream connection settings', () => {
  it('keeps protocols visible while request settings remain optional and locked', async () => {
    mount()
    expect(
      screen.getByRole('button', { name: 'Advanced settings' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByRole('checkbox', { name: 'Chat' })).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Edit request settings' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('switch', { name: 'System proxy' })
    ).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'My gateway' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, { name: 'My gateway' })
    )
    expect(mocks.requestConfig).not.toHaveBeenCalled()
    expect(mocks.list).not.toHaveBeenCalled()
  })
  it('reveals an invalid collapsed endpoint without opening request settings', async () => {
    mount()
    expect(mocks.requestConfig).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    fireEvent.change(screen.getByLabelText('Endpoint URL'), {
      target: { value: 'invalid' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Text & conversation/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Advanced settings' })
      ).toHaveAttribute('aria-expanded', 'false')
    )
    expect(screen.getByLabelText('Endpoint URL')).toBeVisible()
    expect(screen.getByLabelText('Endpoint URL')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(mocks.update).not.toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('Endpoint URL'), {
      target: { value: 'https://gateway.example/custom/chat' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          chat: { url: 'https://gateway.example/custom/chat', auth: 'bearer' },
        },
      })
    )
  })
})

describe('completing upstream endpoints explicitly', () => {
  async function openPresets() {
    fireEvent.click(
      screen.getByRole('button', { name: 'Fill endpoints from a preset' })
    )
    await screen.findByRole('button', { name: 'Use New API' })
  }
  async function chooseNewAPI() {
    await openPresets()
    fireEvent.click(screen.getByRole('button', { name: 'Use New API' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Add missing endpoints' })
    )
  }

  it('adds missing APIs without overwriting custom URLs or expanding existing grants', async () => {
    const { onManageModels } = mount({
      hasChannelProxy: true,
      hasCustomHeaders: true,
      endpointConfig: {
        chat: {
          url: 'https://custom.example/chat',
          auth: 'bearer',
          profile: 'openrouter',
        },
      },
    })
    await openPresets()
    expect(
      screen.getByRole('button', { name: 'Add missing endpoints' })
    ).toBeDisabled()
    expect(mocks.resolve).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Use New API' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Add missing endpoints' })
    )
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Added 2 endpoints to this draft'
    )
    expect(mocks.resolve).toHaveBeenCalledWith(
      { presetId: preset.id, baseUrl: detail.baseUrl },
      expect.any(AbortSignal)
    )
    expect(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    ).toHaveTextContent('https://custom.example/chat')
    expect(screen.getByRole('checkbox', { name: 'Responses' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Messages' })).toBeChecked()
    expect(
      screen.getByText(/New endpoints do not change existing model access/)
    ).toBeVisible()
    expect(mocks.update).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Save and edit model access' })
    )
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          chat: {
            url: 'https://custom.example/chat',
            auth: 'bearer',
            profile: 'openrouter',
          },
          responses: resolved.endpointConfig.responses,
          messages: resolved.endpointConfig.messages,
        },
      })
    )
    await waitFor(() => expect(onManageModels).toHaveBeenCalledOnce())
    expect(mocks.grant).not.toHaveBeenCalled()
    expect(mocks.requestConfig).not.toHaveBeenCalled()
  })

  it('keeps API and Coding Plan as explicit product choices without inferring from the provider', async () => {
    mocks.list.mockResolvedValue({
      items: [
        {
          ...preset,
          id: 'bailian',
          name: 'Bailian',
          label: 'Bailian',
          provider: 'bailian',
          group: 'domestic',
        },
        {
          ...preset,
          id: 'codingplan-openai',
          name: 'Bailian Coding Plan',
          label: 'Bailian Coding Plan',
          provider: 'bailian',
          group: 'coding',
        },
      ],
    })
    mocks.resolve.mockResolvedValue({
      ...resolved,
      presetId: 'codingplan-openai',
      provider: 'bailian',
    })
    mount({ provider: 'bailian', name: 'My service' })
    fireEvent.click(
      screen.getByRole('button', { name: 'Fill endpoints from a preset' })
    )
    expect(
      await screen.findByRole('button', { name: 'Use Alibaba Cloud Bailian' })
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Use Alibaba Cloud Coding Plan' })
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Add missing endpoints' })
    ).toBeDisabled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Use Alibaba Cloud Coding Plan' })
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Add missing endpoints' })
    )
    await waitFor(() =>
      expect(mocks.resolve).toHaveBeenCalledWith(
        { presetId: 'codingplan-openai', baseUrl: detail.baseUrl },
        expect.any(AbortSignal)
      )
    )
    expect(screen.getByLabelText('Name')).toHaveValue('My service')
    expect(screen.getByLabelText('Base URL')).toHaveValue(detail.baseUrl)
  })

  it('preserves the latest adapter and model-specific URLs when a response arrives after endpoint edits', async () => {
    const modelWireUrls = {
      responses: 'https://responses.example/custom',
      messages: 'https://messages.example/custom',
    }
    const chat = {
      url: 'https://chat.example/custom',
      auth: 'bearer' as const,
      profile: 'opencode-go' as const,
      modelWireUrls,
    }
    mocks.list.mockResolvedValue({
      items: [
        {
          ...preset,
          id: 'opencode-go',
          name: 'OpenCode Go',
          label: 'OpenCode Go',
          provider: 'opencode_go',
        },
      ],
    })
    let finish!: (value: unknown) => void
    mocks.resolve.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        })
    )
    mount({ provider: 'opencode_go', endpointConfig: { chat } })
    fireEvent.click(
      screen.getByRole('button', { name: 'Fill endpoints from a preset' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Use OpenCode Go' })
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Add missing endpoints' })
    )
    await waitFor(() => expect(mocks.resolve).toHaveBeenCalledOnce())
    expect(
      screen.getByRole('button', { name: 'Save connection' })
    ).toBeDisabled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    const row = within(screen.getByRole('group', { name: 'Chat' }))
    fireEvent.change(row.getByLabelText('Endpoint URL'), {
      target: { value: 'https://chat.example/new-draft' },
    })
    await act(async () =>
      finish({ ...resolved, presetId: 'opencode-go', provider: 'opencode_go' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          chat: { ...chat, url: 'https://chat.example/new-draft' },
          responses: resolved.endpointConfig.responses,
          messages: resolved.endpointConfig.messages,
        },
      })
    )
  })

  it.each(['product', 'base URL'])(
    'ignores an older resolution after changing the %s',
    async (change) => {
      mocks.list.mockResolvedValue({
        items: [
          preset,
          {
            ...preset,
            id: 'custom',
            name: 'Custom',
            label: 'Custom',
            provider: 'generic',
          },
        ],
      })
      let finish!: (value: unknown) => void
      mocks.resolve.mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finish = resolve
          })
      )
      mount()
      await chooseNewAPI()
      await waitFor(() => expect(mocks.resolve).toHaveBeenCalledOnce())
      if (change === 'product') {
        fireEvent.click(screen.getByRole('button', { name: 'Change' }))
        fireEvent.click(
          screen.getByRole('button', { name: 'Use Custom gateway' })
        )
      } else {
        fireEvent.change(screen.getByLabelText('Base URL'), {
          target: { value: 'https://changed.example' },
        })
      }
      await act(async () => finish(resolved))
      expect(mocks.resolve.mock.calls[0][1].aborted).toBe(true)
      expect(
        screen.getByRole('checkbox', { name: 'Responses' })
      ).not.toBeChecked()
      expect(
        screen.getByRole('checkbox', { name: 'Messages' })
      ).not.toBeChecked()
      expect(
        screen.queryByText(/Added .* endpoints to this draft/)
      ).not.toBeInTheDocument()
      expect(mocks.update).not.toHaveBeenCalled()
    }
  )

  it('reports a failed or empty projection and permits retry without modifying the draft', async () => {
    mocks.resolve.mockRejectedValueOnce(new Error('fixture failure'))
    mount()
    await chooseNewAPI()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not prepare the endpoints'
    )
    expect(
      screen.getByRole('button', { name: 'Save connection' })
    ).toBeDisabled()
    mocks.resolve.mockResolvedValueOnce({ ...resolved, endpointConfig: {} })
    fireEvent.click(
      screen.getByRole('button', { name: 'Add missing endpoints' })
    )
    await waitFor(() => expect(mocks.resolve).toHaveBeenCalledTimes(2))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not prepare the endpoints'
    )
    expect(
      screen.getByRole('checkbox', { name: 'Responses' })
    ).not.toBeChecked()
    fireEvent.click(
      screen.getByRole('button', { name: 'Add missing endpoints' })
    )
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Added 2 endpoints to this draft'
    )
  })

  it('reports that no endpoints are missing without marking the form dirty', async () => {
    mount({ endpointConfig: resolved.endpointConfig })
    await chooseNewAPI()
    expect(await screen.findByRole('status')).toHaveTextContent(
      'All endpoints from this preset are already configured'
    )
    expect(
      screen.getByRole('button', { name: 'Save connection' })
    ).toBeDisabled()
    expect(mocks.update).not.toHaveBeenCalled()
  })

  it('preserves legacy paths and hidden request settings on unrelated edits', async () => {
    mount({
      endpointConfig: {},
      openaiChatCompletionPath: '/custom/chat',
      openaiResponsePath: '/custom/responses',
      hasChannelProxy: true,
      hasCustomHeaders: true,
    })
    expect(
      screen.getByText(/This connection uses legacy API paths/)
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Fill endpoints from a preset' })
    ).not.toBeInTheDocument()
    expect(screen.getByDisplayValue('/custom/chat')).toBeVisible()
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Legacy renamed' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, { name: 'Legacy renamed' })
    )
    expect(mocks.requestConfig).not.toHaveBeenCalled()
  })

  it('keeps the connection open when saving before model access fails', async () => {
    mocks.update.mockRejectedValueOnce(new Error('fixture save failure'))
    const { onManageModels } = mount()
    await chooseNewAPI()
    await screen.findByRole('status')
    fireEvent.click(
      screen.getByRole('button', { name: 'Save and edit model access' })
    )
    await waitFor(() => expect(mocks.update).toHaveBeenCalledOnce())
    expect(onManageModels).not.toHaveBeenCalled()
    expect(screen.getByRole('checkbox', { name: 'Responses' })).toBeChecked()
    expect(mocks.grant).not.toHaveBeenCalled()
  })
  it('prepares project-based endpoints with Enter without submitting the connection form', async () => {
    const endpoint = {
      url: 'https://vertex.example/google/models',
      auth: 'bearer' as const,
      profile: 'vertex' as const,
      modelPath: true,
    }
    mocks.list.mockResolvedValue({
      items: [
        {
          ...preset,
          id: 'vertex-gemini',
          name: 'Google Vertex AI',
          label: 'Google Vertex AI',
          provider: 'gemini_vertex',
          requiresProject: true,
          protocols: ['gemini', 'geminiEmbeddings'],
        },
      ],
    })
    mocks.resolve.mockResolvedValue({
      presetId: 'vertex-gemini',
      provider: 'gemini_vertex',
      baseUrl: detail.baseUrl,
      endpointConfig: { gemini: endpoint, geminiEmbeddings: endpoint },
    })
    mount({ provider: 'gemini_vertex', endpointConfig: { gemini: endpoint } })
    fireEvent.click(
      screen.getByRole('button', { name: 'Fill endpoints from a preset' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Use Google Vertex AI' })
    )
    fireEvent.change(screen.getByLabelText('GCP project'), {
      target: { value: 'synthetic-project' },
    })
    expect(
      fireEvent.keyDown(screen.getByLabelText('GCP project'), { key: 'Enter' })
    ).toBe(false)
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Added 1 endpoint to this draft'
    )
    expect(mocks.resolve).toHaveBeenCalledWith(
      {
        presetId: 'vertex-gemini',
        baseUrl: detail.baseUrl,
        projectId: 'synthetic-project',
      },
      expect.any(AbortSignal)
    )
    expect(mocks.update).not.toHaveBeenCalled()
  })
})
