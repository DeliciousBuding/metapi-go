import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
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
  ImportedMember,
  ImportedUpstreamDetail,
} from '@/lib/api/imported-upstreams'

import { UpstreamConnectionForm } from '../components/upstream-connection-form'
import { UpstreamCredentials } from '../components/upstream-credentials'
import { UpstreamIdentity } from '../components/upstream-identity'
import { UpstreamGrantForm } from '../components/upstream-model-forms'
import { UpstreamRouteBindings } from '../components/upstream-route-bindings'
import { connectionSchema, connectionValues } from '../lib/upstream-config'

const state = vi.hoisted(() => ({
  update: vi.fn(),
  credentials: vi.fn(),
  createCredential: vi.fn(),
  updateCredential: vi.fn(),
  member: vi.fn(),
  grant: vi.fn(),
  dirty: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    updateImportedUpstream: state.update,
    getImportedCredentials: state.credentials,
    createUpstreamCredential: state.createCredential,
    updateImportedCredential: state.updateCredential,
    updateImportedMember: state.member,
    createUpstreamGrant: state.grant,
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
  name: 'Native upstream',
  originKey: '',
  provider: 'ollama',
  dialect: 'generic',
  enabled: false,
  baseUrl: 'http://localhost:11434',
  endpointConfig: {
    ollama: {
      url: 'http://localhost:11434/api/chat',
      auth: 'none',
      profile: 'ollama',
    },
  },
  useSystemProxy: false,
  openaiChatCompletionPath: '',
  openaiResponsePath: '',
  anthropicMessagePath: '',
  hasChannelProxy: false,
  hasCustomHeaders: false,
  hasParamOverride: false,
  channelProxyDisplay: '',
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
async function select(label: string, option: string) {
  fireEvent.click(screen.getByRole('combobox', { name: label }))
  const item = await screen.findByRole('option', { name: option })
  fireEvent.pointerDown(item)
  fireEvent.click(item)
}
beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  state.update.mockResolvedValue({ success: true })
  state.member.mockResolvedValue({ success: true })
  state.updateCredential.mockResolvedValue({ success: true })
  state.createCredential.mockResolvedValue({ id: 5 })
  state.grant.mockResolvedValue({ id: 6 })
  state.credentials.mockResolvedValue({ items: [] })
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('native upstream configuration', () => {
  it('shows Amazon Bedrock identity for the imported Anthropic AWS provider', () => {
    render(<UpstreamIdentity provider='anthropic_aws' dialect='generic' />)
    expect(screen.getByText('Amazon Bedrock')).toBeVisible()
    expect(screen.getByRole('img', { hidden: true })).toHaveAttribute(
      'src',
      expect.stringContaining('bedrock')
    )
  })
  it('edits native Ollama authentication while retaining its required wire format', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={state.dirty} />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Ollama endpoint' })
    )
    expect(
      screen.getByRole('combobox', { name: 'Protocol adapter' })
    ).toBeDisabled()
    expect(
      screen.getByRole('combobox', { name: 'Authentication' })
    ).toHaveTextContent('No authentication')
    await select('Authentication', 'Bearer')
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          ollama: {
            url: 'http://localhost:11434/api/chat',
            auth: 'bearer',
            profile: 'ollama',
          },
        },
      })
    )
  })

  it('sets Bedrock model-prefix semantics and removes them when returning to standard Messages', async () => {
    mount(
      <UpstreamConnectionForm
        detail={{
          ...detail,
          provider: 'generic',
          endpointConfig: {
            messages: {
              url: 'https://bedrock.example/model',
              auth: 'x-api-key',
            },
          },
        }}
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Messages endpoint' })
    )
    await select('Protocol adapter', 'Amazon Bedrock')
    expect(
      screen.getByLabelText('Model URL prefix (ending in /model)')
    ).toHaveValue('https://bedrock.example/model')
    expect(
      screen.getByRole('combobox', { name: 'Authentication' })
    ).toHaveTextContent('Bearer')
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          messages: {
            url: 'https://bedrock.example/model',
            auth: 'bearer',
            profile: 'bedrock',
            modelPath: true,
          },
        },
      })
    )
    await waitFor(() =>
      expect(
        screen.getByRole('combobox', { name: 'Protocol adapter' })
      ).toBeEnabled()
    )
    await select('Protocol adapter', 'Standard protocol')
    expect(screen.getByLabelText('Endpoint URL')).toHaveValue(
      'https://bedrock.example/model'
    )
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save connection' })
      ).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenLastCalledWith(4, {
        endpointConfig: {
          messages: {
            url: 'https://bedrock.example/model',
            auth: 'bearer',
            profile: undefined,
            modelPath: undefined,
          },
        },
      })
    )
  })

  it('rejects anonymous standard endpoints and misplaced native adapters', () => {
    const url = 'https://native.example/endpoint'
    for (const endpointConfig of [
      { chat: { url, auth: 'none' } },
      { ollama: { url, auth: 'none' } },
      { messages: { url, auth: 'none', profile: 'bedrock', modelPath: true } },
      { messages: { url, auth: 'bearer', profile: 'bedrock' } },
      { video: { url, auth: 'bearer', profile: 'seedance-video' } },
      { seedanceVideo: { url, auth: 'bearer', profile: 'zenmux-video' } },
      { systemOne: { url, auth: 'bearer', profile: 'codex-alpha-search' } },
    ]) {
      expect(
        connectionSchema.safeParse({
          ...connectionValues(detail),
          endpointConfig,
        }).success
      ).toBe(false)
    }
    expect(
      connectionSchema.safeParse({
        ...connectionValues(detail),
        endpointConfig: {
          messages: { url, auth: 'none', profile: 'ollama-messages' },
        },
      }).success
    ).toBe(true)
  })

  it('saves dedicated Seedance and ZenMux endpoints alongside exact native APIs', async () => {
    mount(
      <UpstreamConnectionForm
        detail={{ ...detail, provider: 'generic', endpointConfig: {} }}
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Video 0 / 3 configured' })
    )
    for (const name of ['Seedance video', 'ZenMux video']) {
      fireEvent.click(screen.getByRole('checkbox', { name }))
      const row = within(screen.getByRole('group', { name }))
      fireEvent.change(row.getByLabelText('Endpoint URL'), {
        target: { value: `https://video.example/${name.split(' ')[0]}` },
      })
      expect(
        row.getByRole('combobox', { name: 'Protocol adapter' })
      ).toBeDisabled()
    }
    fireEvent.click(
      screen.getByRole('button', { name: 'Native APIs 0 / 1 configured' })
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'System One' }))
    const systemOne = within(screen.getByRole('group', { name: 'System One' }))
    fireEvent.change(systemOne.getByLabelText('Endpoint URL'), {
      target: { value: 'https://typesafe.example/v1/systemone' },
    })
    expect(
      systemOne.queryByRole('combobox', { name: 'Protocol adapter' })
    ).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Vectors & retrieval 0 / 5 configured',
      })
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Alpha Search' }))
    fireEvent.change(
      within(
        screen.getByRole('group', { name: 'Alpha Search' })
      ).getByLabelText('Endpoint URL'),
      { target: { value: 'https://search.example/alpha/search' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          seedanceVideo: {
            url: 'https://video.example/Seedance',
            auth: 'bearer',
            profile: 'seedance-video',
          },
          zenmuxVideo: {
            url: 'https://video.example/ZenMux',
            auth: 'bearer',
            profile: 'zenmux-video',
          },
          systemOne: {
            url: 'https://typesafe.example/v1/systemone',
            auth: 'bearer',
          },
          alphaSearch: {
            url: 'https://search.example/alpha/search',
            auth: 'bearer',
          },
        },
      })
    )
  })

  it('creates anonymous credentials without leaking a previously typed key and edits them without a replacement field', async () => {
    const view = mount(
      <UpstreamCredentials
        id={4}
        active
        allowAnonymous
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Add credential' }))
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Local Ollama' },
    })
    expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    await select('Credential type', 'API Key')
    fireEvent.change(screen.getByLabelText('API Key'), {
      target: { value: 'synthetic-unused-key' },
    })
    await select('Credential type', 'No authentication')
    expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    fireEvent.click(
      screen.getAllByRole('button', { name: 'Add credential' })[1]
    )
    await waitFor(() =>
      expect(state.createCredential).toHaveBeenCalledWith(4, {
        name: 'Local Ollama',
        enabled: false,
        kind: 'none',
      })
    )
    view.unmount()
    state.credentials.mockResolvedValue({
      items: [
        {
          id: 5,
          name: 'Local Ollama',
          kind: 'none',
          enabled: true,
          canRefresh: false,
        },
      ],
    })
    mount(
      <UpstreamCredentials
        id={4}
        active
        allowAnonymous
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Edit Local Ollama' })
    )
    expect(
      screen.queryByRole('switch', { name: 'Replace credential' })
    ).not.toBeInTheDocument()
    expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Local inference' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save credential' }))
    await waitFor(() =>
      expect(state.updateCredential).toHaveBeenCalledWith(5, {
        name: 'Local inference',
      })
    )
  })

  it('hides anonymous credentials for other providers', async () => {
    mount(<UpstreamCredentials id={4} active onDirtyChange={state.dirty} />)
    fireEvent.click(screen.getByRole('button', { name: 'Add credential' }))
    fireEvent.click(screen.getByRole('combobox', { name: 'Credential type' }))
    expect(await screen.findByRole('option', { name: 'API Key' })).toBeVisible()
    expect(
      screen.queryByRole('option', { name: 'No authentication' })
    ).not.toBeInTheDocument()
  })

  it('blocks incompatible grants after switching to an anonymous credential and allows removing the conflict', async () => {
    mount(
      <UpstreamGrantForm
        modelId={9}
        credentials={[
          {
            id: 5,
            name: 'API credential',
            kind: 'api_key',
            enabled: true,
            canRefresh: false,
          },
          {
            id: 6,
            name: 'Local Ollama',
            kind: 'none',
            enabled: true,
            canRefresh: false,
          },
        ]}
        availableProtocols={[2, 2097152]}
        anonymousProtocols={[2097152]}
        members={[]}
        onDirtyChange={state.dirty}
      />
    )
    await select('Credentials', 'API credential')
    fireEvent.click(screen.getByRole('checkbox', { name: 'Chat' }))
    await select('Credentials', 'Local Ollama')
    expect(await screen.findByText('Authentication required')).toBeVisible()
    expect(screen.getByRole('checkbox', { name: /^Chat/ })).toBeChecked()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Ollama' }))
    fireEvent.click(screen.getByRole('button', { name: 'Add access' }))
    expect(
      await screen.findByText(
        'Select only endpoints that require no authentication for this credential.'
      )
    ).toBeVisible()
    expect(state.grant).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('checkbox', { name: /^Chat/ }))
    expect(
      screen.queryByRole('checkbox', { name: /Chat/ })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Add access' }))
    await waitFor(() =>
      expect(state.grant).toHaveBeenCalledWith({
        modelId: 9,
        credentialId: 6,
        protocols: [2097152],
        enabled: true,
      })
    )
  })

  it('sorts Ollama conversation conversion and native video formats independently without dropping exact capabilities', async () => {
    const member: ImportedMember = {
      id: 7,
      groupId: 8,
      groupName: 'Native routes',
      routeId: 9,
      channelId: 4,
      modelName: 'native-model',
      credentialName: 'Plan key',
      credentialEnabled: true,
      protocols: 2 | 2097152 | 524288 | 1048576 | 4194304 | 8388608,
      protocolOrder: [2, 524288, 2097152, 1048576, 4194304, 8388608],
      mode: 'failover',
      activeItemId: 0,
      priority: 0,
      weight: 1,
    }
    mount(
      <UpstreamRouteBindings members={[member]} onDirtyChange={state.dirty} />
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit route Native routes' })
    )
    expect(
      screen.getByRole('list', { name: 'Conversation conversion order' })
    ).toHaveTextContent('Chat')
    expect(
      screen.getByRole('list', { name: 'Conversation conversion order' })
    ).not.toHaveTextContent('Seedance')
    expect(
      screen.getByRole('list', { name: 'Video format order' })
    ).not.toHaveTextContent('Ollama')
    expect(
      screen.queryByRole('button', { name: 'Move System One up' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Move Alpha Search up' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Move Ollama up' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Move ZenMux video up' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save route' }))
    await waitFor(() =>
      expect(state.member).toHaveBeenCalledWith(7, {
        protocolOrder: [1048576, 524288, 2097152, 2, 4194304, 8388608],
      })
    )
  })
})
