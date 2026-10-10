import { readFileSync } from 'node:fs'

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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type {
  ImportedUpstreamDetail,
  ImportedMember,
} from '@/lib/api/imported-upstreams'
import type { UpstreamGrant } from '@/lib/api/upstream-catalog'

import { UpstreamConnectionForm } from '../components/upstream-connection-form'
import { UpstreamEndpointsEditor } from '../components/upstream-endpoints-editor'
import { UpstreamGrantForm } from '../components/upstream-model-forms'
import { UpstreamModels } from '../components/upstream-models'
import { UpstreamRouteBindings } from '../components/upstream-route-bindings'
import {
  connectionSchema,
  connectionValues,
  upstreamProtocols,
} from '../lib/upstream-config'

const state = vi.hoisted(() => ({
  update: vi.fn(),
  grant: vi.fn(),
  models: vi.fn(),
  credentials: vi.fn(),
  member: vi.fn(),
  dirty: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    updateImportedUpstream: state.update,
    updateUpstreamGrant: state.grant,
    getUpstreamModels: state.models,
    getImportedCredentials: state.credentials,
    updateImportedMember: state.member,
  },
}))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children: React.ReactNode }) => (
    <a href='/token-routes'>{props.children}</a>
  ),
}))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn() } }))

const detail: ImportedUpstreamDetail = {
  id: 4,
  name: 'Media API',
  originKey: '',
  provider: 'generic',
  dialect: 'generic',
  enabled: true,
  baseUrl: 'https://upstream.example',
  useSystemProxy: false,
  endpointConfig: {
    embeddings: { url: 'https://vectors.example/embed', auth: 'bearer' },
  },
  // A legacy Chat path must never replace the explicit vector endpoint.
  openaiChatCompletionPath: '/chat',
  openaiResponsePath: '',
  anthropicMessagePath: '',
  hasChannelProxy: false,
  hasCustomHeaders: false,
  hasParamOverride: false,
  channelProxyDisplay: '',
}
const credential = {
  id: 5,
  name: 'Media key',
  kind: 'api_key' as const,
  enabled: true,
  canRefresh: false,
}
const grant: UpstreamGrant = {
  id: 3,
  modelId: 9,
  credentialId: 5,
  credentialName: 'Media key',
  enabled: true,
  protocols: 64 | 256,
  memberCount: 1,
  ownership: 'native',
}
let client: QueryClient
function mount(node: React.ReactNode) {
  client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={client}>{node}</QueryClientProvider>
  )
}
beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  state.update.mockResolvedValue({ success: true })
  state.grant.mockResolvedValue({ success: true })
  state.member.mockResolvedValue({ success: true })
  state.credentials.mockResolvedValue({ items: [credential] })
  state.models.mockResolvedValue({
    items: [
      {
        id: 9,
        name: 'embedding-model',
        enabled: true,
        ownership: 'native',
        grants: [{ ...grant, protocols: 64 }],
      },
    ],
  })
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('explicit upstream media capabilities', () => {
  it('keeps fourteen configured New API endpoints compact and edits only the requested endpoint', async () => {
    const keys = [
      'chat',
      'responses',
      'messages',
      'gemini',
      'completions',
      'moderations',
      'embeddings',
      'imageGeneration',
      'imageEdit',
      'imageVariation',
      'audioSpeech',
      'audioTranscription',
      'audioTranslation',
      'video',
    ]
    const endpointConfig = Object.fromEntries(
      keys.map((key) => [
        key,
        { url: `https://newapi.example/${key}`, auth: 'bearer' as const },
      ])
    )
    mount(
      <UpstreamConnectionForm
        detail={{ ...detail, name: 'New API', endpointConfig }}
        onDirtyChange={state.dirty}
      />
    )
    expect(screen.queryAllByLabelText('Endpoint URL')).toHaveLength(0)
    expect(
      screen.queryByRole('combobox', { name: 'Protocol adapter' })
    ).not.toBeInTheDocument()
    expect(
      screen.getAllByRole('button', { name: /^Configure .* endpoint$/ })
    ).toHaveLength(6)
    expect(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('https://newapi.example/chat')).toBeVisible()

    fireEvent.click(
      screen.getByRole('button', { name: 'Audio 3 / 3 configured' })
    )
    expect(screen.queryAllByLabelText('Endpoint URL')).toHaveLength(0)
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Transcription endpoint' })
    )
    expect(screen.getAllByLabelText('Endpoint URL')).toHaveLength(1)
    fireEvent.change(screen.getByLabelText('Endpoint URL'), {
      target: { value: 'https://newapi.example/transcribe' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Transcription endpoint' })
    )
    expect(screen.queryByLabelText('Endpoint URL')).not.toBeInTheDocument()
    expect(screen.getByText('https://newapi.example/transcribe')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          ...endpointConfig,
          audioTranscription: {
            url: 'https://newapi.example/transcribe',
            auth: 'bearer',
          },
        },
      })
    )
  })

  it('opens a newly enabled empty endpoint and reveals it again when a collapsed invalid draft is submitted', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={state.dirty} />
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Audio 0 / 3 configured' })
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Speech' }))
    expect(screen.getByLabelText('Endpoint URL')).toHaveValue('')
    expect(
      screen.getByRole('button', { name: 'Configure Speech endpoint' })
    ).toHaveAttribute('aria-expanded', 'true')
    for (let attempt = 0; attempt < 2; attempt++) {
      fireEvent.click(
        screen.getByRole('button', { name: 'Configure Speech endpoint' })
      )
      fireEvent.click(
        screen.getByRole('button', { name: 'Audio 1 / 3 configured' })
      )
      expect(screen.queryByLabelText('Endpoint URL')).not.toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
      await waitFor(() =>
        expect(screen.getByLabelText('Endpoint URL')).toBeVisible()
      )
      expect(screen.getByLabelText('Endpoint URL')).toHaveAttribute(
        'aria-invalid',
        'true'
      )
      expect(
        screen.getByRole('button', { name: 'Audio 1 / 3 configured' })
      ).toHaveAttribute('aria-expanded', 'true')
      expect(state.update).not.toHaveBeenCalled()
    }
    fireEvent.change(screen.getByLabelText('Endpoint URL'), {
      target: { value: 'https://audio.example/speech' },
    })
    expect(screen.getByLabelText('Endpoint URL')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          ...detail.endpointConfig,
          audioSpeech: { url: 'https://audio.example/speech', auth: 'bearer' },
        },
      })
    )
  })

  it('requires the declared native wire profile and Bearer authentication', () => {
    const invalid = [
      {
        embeddings: {
          url: 'https://example.com/embed',
          auth: 'bearer',
          profile: 'jina-embeddings',
        },
      },
      { jinaEmbeddings: { url: 'https://example.com/embed', auth: 'bearer' } },
      {
        jinaEmbeddings: {
          url: 'https://example.com/embed',
          auth: 'x-api-key',
          profile: 'jina-embeddings',
        },
      },
      {
        modelscopeImageGeneration: {
          url: 'https://example.com/image',
          auth: 'bearer',
        },
      },
      {
        imageEdit: {
          url: 'https://example.com/image',
          auth: 'bearer',
          profile: 'minimax-image',
        },
      },
      {
        audioSpeech: {
          url: 'https://example.com/speech',
          auth: 'bearer',
          profile: 'deepseek',
        },
      },
    ]
    for (const endpointConfig of invalid) {
      expect(
        connectionSchema.safeParse({
          ...connectionValues(detail),
          endpointConfig,
        }).success
      ).toBe(false)
    }
  })

  it('sets required profiles for native Jina and ModelScope endpoints on a custom provider', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={state.dirty} />
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Jina embeddings' }))
    const jina = within(screen.getByRole('group', { name: 'Jina embeddings' }))
    fireEvent.change(jina.getByLabelText('Endpoint URL'), {
      target: { value: 'https://native.example/jina' },
    })
    expect(
      jina.getByRole('combobox', { name: 'Protocol adapter' })
    ).toBeDisabled()
    expect(
      jina.getByRole('combobox', { name: 'Protocol adapter' })
    ).toHaveTextContent('Jina embeddings')
    fireEvent.click(
      screen.getByRole('button', { name: 'Images 0 / 4 configured' })
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'ModelScope images' }))
    const image = within(
      screen.getByRole('group', { name: 'ModelScope images' })
    )
    fireEvent.change(image.getByLabelText('Endpoint URL'), {
      target: { value: 'https://native.example/modelscope' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          ...detail.endpointConfig,
          jinaEmbeddings: {
            url: 'https://native.example/jina',
            auth: 'bearer',
            profile: 'jina-embeddings',
          },
          modelscopeImageGeneration: {
            url: 'https://native.example/modelscope',
            auth: 'bearer',
            profile: 'modelscope-image',
          },
        },
      })
    )
  })

  it('offers explicit image wire adapters without exposing conversation or Codex adapters on a custom provider', async () => {
    mount(
      <UpstreamConnectionForm
        detail={{
          ...detail,
          endpointConfig: {
            imageGeneration: {
              url: 'https://image.example/generate',
              auth: 'x-api-key',
            },
          },
        }}
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(
      screen.getByRole('button', {
        name: /Configure Image (generation|edits) endpoint/,
      })
    )
    fireEvent.click(screen.getByRole('combobox', { name: 'Protocol adapter' }))
    expect(
      await screen.findByRole('option', { name: 'MiniMax image' })
    ).toBeVisible()
    expect(
      screen.getByRole('option', { name: 'ModelScope image' })
    ).toBeVisible()
    expect(
      screen.queryByRole('option', { name: 'Codex image' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'DeepSeek reasoning' })
    ).not.toBeInTheDocument()
    fireEvent.pointerDown(screen.getByRole('option', { name: 'MiniMax image' }))
    fireEvent.click(screen.getByRole('option', { name: 'MiniMax image' }))
    await waitFor(() =>
      expect(
        screen.getByRole('combobox', { name: 'Protocol adapter' })
      ).toHaveTextContent('MiniMax image')
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          imageGeneration: {
            url: 'https://image.example/generate',
            auth: 'bearer',
            profile: 'minimax-image',
            requestModel: undefined,
          },
        },
      })
    )
  })

  it('requires and saves a separate Codex image request model', async () => {
    mount(
      <UpstreamConnectionForm
        detail={{
          ...detail,
          provider: 'codex',
          endpointConfig: {
            imageEdit: {
              url: 'https://codex.example/responses',
              auth: 'bearer',
            },
          },
        }}
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(
      screen.getByRole('button', {
        name: /Configure Image (generation|edits) endpoint/,
      })
    )
    fireEvent.click(screen.getByRole('combobox', { name: 'Protocol adapter' }))
    const option = await screen.findByRole('option', { name: 'Codex image' })
    fireEvent.pointerDown(option)
    fireEvent.click(option)
    await screen.findByLabelText('Codex request model')
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    expect(
      await screen.findByText(
        'Check endpoint URLs and authentication settings.'
      )
    ).toBeVisible()
    expect(state.update).not.toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('Codex request model'), {
      target: { value: 'codex-text-model' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          imageEdit: {
            url: 'https://codex.example/responses',
            auth: 'bearer',
            profile: 'codex-image',
            requestModel: 'codex-text-model',
          },
        },
      })
    )
  })

  it('reveals the configured media group when a preset resolves after the editor mounts', () => {
    const view = render(
      <UpstreamEndpointsEditor
        value={{}}
        provider='generic'
        onChange={vi.fn()}
      />
    )
    expect(
      screen.getByRole('button', {
        name: 'Text & conversation 0 / 7 configured',
      })
    ).toHaveAttribute('aria-expanded', 'true')
    view.rerender(
      <UpstreamEndpointsEditor
        value={detail.endpointConfig ?? {}}
        provider='generic'
        onChange={vi.fn()}
      />
    )
    expect(
      screen.getByRole('button', {
        name: 'Vectors & retrieval 1 / 5 configured',
      })
    ).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('https://vectors.example/embed')).toBeVisible()
    expect(screen.queryByLabelText('Endpoint URL')).not.toBeInTheDocument()
  })

  it('keeps UI endpoint keys and persisted bits aligned with the backend contract', () => {
    const source = readFileSync('../store/direct_protocols.go', 'utf8')
    const bits = new Map(
      [...source.matchAll(/(DirectProtocol\w+)\s+= 1 << (\d+)/g)].map(
        (match) => [match[1], 1 << Number(match[2])]
      )
    )
    const entries = [
      ...source.matchAll(/\{"([^"]+)", (DirectProtocol\w+), e\.\w+\}/g),
    ].map((match) => ({ key: match[1], bit: bits.get(match[2]) }))
    expect(entries).toHaveLength(23)
    expect(upstreamProtocols.map(({ key, bit }) => ({ key, bit }))).toEqual(
      entries
    )
  })

  it('opens endpoint groups on demand and saves independent media paths and Gemini modelPath', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={state.dirty} />
    )
    expect(
      screen.getByRole('button', {
        name: 'Vectors & retrieval 1 / 5 configured',
      })
    ).toHaveAttribute('aria-expanded', 'true')
    expect(
      screen.queryByRole('checkbox', { name: 'Image generation' })
    ).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Embeddings endpoint' })
    )
    const vectors = within(screen.getByRole('group', { name: 'Embeddings' }))
    fireEvent.change(vectors.getByLabelText('Endpoint URL'), {
      target: { value: 'https://vectors.example/new' },
    })
    expect(
      vectors.queryByRole('combobox', { name: 'Protocol adapter' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Gemini embeddings' }))
    const gemini = within(
      screen.getByRole('group', { name: 'Gemini embeddings' })
    )
    fireEvent.change(gemini.getByLabelText('Endpoint URL'), {
      target: { value: 'https://gemini.example/v1beta/models' },
    })
    fireEvent.click(gemini.getByRole('checkbox', { name: /Append/ }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Images 0 / 4 configured' })
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Image generation' }))
    const image = within(
      screen.getByRole('group', { name: 'Image generation' })
    )
    fireEvent.change(image.getByLabelText('Endpoint URL'), {
      target: { value: 'https://images.example/generate' },
    })
    expect(
      image.getByRole('combobox', { name: 'Protocol adapter' })
    ).toHaveTextContent('Standard protocol')
    const group = screen.getByRole('button', {
      name: 'Vectors & retrieval 2 / 5 configured',
    })
    fireEvent.click(group)
    fireEvent.click(group)
    expect(vectors.getByLabelText('Endpoint URL')).toHaveValue(
      'https://vectors.example/new'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          embeddings: { url: 'https://vectors.example/new', auth: 'bearer' },
          geminiEmbeddings: {
            url: 'https://gemini.example/v1beta/models',
            auth: 'x-goog-api-key',
            modelPath: true,
          },
          imageGeneration: {
            url: 'https://images.example/generate',
            auth: 'bearer',
          },
        },
      })
    )
  })

  it('rejects an invalid media endpoint instead of bypassing connection validation', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={state.dirty} />
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Embeddings endpoint' })
    )
    fireEvent.change(screen.getByLabelText('Endpoint URL'), {
      target: { value: 'invalid-vector-url' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    expect(
      await screen.findByText(
        'Check endpoint URLs and authentication settings.'
      )
    ).toBeVisible()
    expect(state.update).not.toHaveBeenCalled()
  })

  it('disables adding a grant when every credential is already assigned and offers no Chat fallback', async () => {
    mount(
      <UpstreamModels
        detail={detail}
        active
        members={[]}
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Edit model embedding-model' })
    )
    expect(screen.getByRole('button', { name: 'Add access' })).toBeDisabled()
    expect(
      screen.getByText(
        'Every credential already has a grant for this model. Edit an existing grant, or add a credential in the Credentials tab.'
      )
    ).toBeVisible()
    expect(screen.getByRole('checkbox', { name: 'Embeddings' })).toBeChecked()
    expect(
      screen.queryByRole('checkbox', { name: 'Chat' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Credentials' })
    ).not.toBeInTheDocument()
  })

  it('edits exact capability bits and keeps the media draft and pending lock during refresh', async () => {
    let finish!: (value: { success: boolean }) => void
    state.grant.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        })
    )
    const node = (value: UpstreamGrant) => (
      <UpstreamGrantForm
        modelId={9}
        grant={value}
        credentials={[credential]}
        availableProtocols={[64, 256, 512]}
        members={[]}
        onDirtyChange={state.dirty}
      />
    )
    const view = mount(node(grant))
    expect(screen.getByRole('checkbox', { name: 'Embeddings' })).toBeChecked()
    fireEvent.click(
      screen.getByRole('button', { name: 'Images 1 / 2 selected' })
    )
    expect(
      screen.getByRole('checkbox', { name: 'Image generation' })
    ).toBeChecked()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Image edits' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save access' }))
    await waitFor(() =>
      expect(state.grant).toHaveBeenCalledWith(3, { protocols: [64, 256, 512] })
    )
    view.rerender(
      <QueryClientProvider client={client}>
        {node({ ...grant, enabled: false })}
      </QueryClientProvider>
    )
    expect(screen.getByRole('checkbox', { name: 'Image edits' })).toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: 'Image edits' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: 'Save access' })).toBeDisabled()
    await act(async () => finish({ success: true }))
    expect(state.grant).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('switch', { name: 'Enabled' })).not.toBeChecked()
  })

  it('sorts conversation conversion only while retaining exact media membership', async () => {
    const member: ImportedMember = {
      id: 7,
      groupId: 8,
      groupName: 'Mixed capabilities',
      routeId: 9,
      channelId: 4,
      modelName: 'multimodal',
      credentialName: 'Media key',
      credentialEnabled: true,
      protocols: 2 | 8 | 64 | 256,
      protocolOrder: [64, 8, 256, 2],
      mode: 'failover',
      activeItemId: 0,
      priority: 0,
      weight: 1,
      effectiveEnabled: true,
    }
    mount(
      <UpstreamRouteBindings members={[member]} onDirtyChange={state.dirty} />
    )
    expect(
      screen.getByText('Messages → Chat · Embeddings · Image generation')
    ).toBeVisible()
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit route Mixed capabilities' })
    )
    expect(
      screen.queryByRole('button', { name: /Move Embeddings/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Move Image generation/ })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Move Chat up' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save route' }))
    await waitFor(() =>
      expect(state.member).toHaveBeenCalledWith(7, {
        protocolOrder: [2, 8, 64, 256],
      })
    )
  })
})
