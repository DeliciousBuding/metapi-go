import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import i18n from 'i18next'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import {
  channelPluginQueryKey,
  type ChannelPlugin,
  type ProviderBinding,
} from '@/lib/api/channel-plugins'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import { ChannelPluginBindings } from '../components/channel-plugin-bindings'
import { ChannelProviderBinding } from '../components/channel-provider-binding'
import { UpstreamConnectionForm } from '../components/upstream-connection-form'
import { UpstreamCreateSheet } from '../components/upstream-create-sheet'

const mocks = vi.hoisted(() => ({
  plugins: vi.fn(),
  config: vi.fn(),
  update: vi.fn(),
  presets: vi.fn(),
  resolve: vi.fn(),
  connect: vi.fn(),
}))
vi.mock('@/lib/api/channel-plugins', async (original) => ({
  ...(await original<typeof import('@/lib/api/channel-plugins')>()),
  channelPluginsApi: { list: mocks.plugins },
}))
vi.mock('@/lib/api', () => ({
  api: {
    getImportedRequestConfig: mocks.config,
    updateImportedUpstream: mocks.update,
    connectUpstream: mocks.connect,
  },
}))
vi.mock('@/lib/api/upstream-presets', () => ({
  upstreamPresetsApi: {
    getUpstreamPresets: mocks.presets,
    resolveUpstreamPreset: mocks.resolve,
  },
}))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn(), info: vi.fn() } }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

const provider: ChannelPlugin = {
  key: 'fixture-provider',
  enabled: true,
  revision: 1,
  manifest: {
    apiVersion: 2,
    kind: 'provider',
    key: 'fixture-provider',
    name: 'Fixture provider',
    version: '1.0.0',
    protocols: ['chat', 'responses'],
    credentialKinds: ['api_key'],
    hooks: ['request', 'response', 'modelsRequest', 'modelsResponse'],
    options: [
      { key: 'mode', label: 'Provider mode', type: 'string', default: 'safe' },
    ],
    defaults: { catalog: { path: '/inventory', auth: 'bearer' } },
  },
}
const wire: ChannelPlugin = {
  key: 'fixture-wire',
  enabled: true,
  revision: 1,
  manifest: {
    apiVersion: 1,
    key: 'fixture-wire',
    name: 'Fixture wire',
    version: '1',
    protocols: ['chat'],
    hooks: ['request'],
  },
}
const binding: ProviderBinding = {
  key: provider.key,
  protocols: ['chat'],
  options: { mode: 'saved' },
  catalog: {
    url: 'https://fixture.invalid/custom-models',
    auth: 'x-api-key',
    cursorParameter: 'page',
  },
}
const imageProvider: ChannelPlugin = {
  ...provider,
  manifest: {
    apiVersion: 2,
    kind: 'provider',
    key: provider.key,
    name: 'Fixture images',
    version: '1',
    protocols: ['imageGeneration', 'imageEdit'],
    credentialKinds: ['api_key'],
    hooks: ['submit', 'query', 'taskResult'],
    imageTasks: {
      imageGeneration: {
        query: { path: '/generation/{taskId}', auth: 'bearer' },
      },
      imageEdit: { query: { path: '/editing/{taskId}', auth: 'none' } },
    },
    observations: {
      imageGeneration: { response: { taskIdPath: '/id' } },
      imageEdit: { response: { taskIdPath: '/id' } },
    },
  },
}
const imageBinding: ProviderBinding = {
  key: provider.key,
  protocols: ['imageGeneration', 'imageEdit'],
  imageQueries: {
    imageGeneration: {
      url: 'https://fixture.invalid/saved-generate/{taskId}',
      auth: 'x-api-key',
    },
    imageEdit: {
      url: 'https://fixture.invalid/saved-edit/{taskId}',
      auth: 'none',
    },
  },
}
const detail: ImportedUpstreamDetail = {
  id: 7,
  name: 'Fixture channel',
  originKey: '',
  provider: 'custom',
  dialect: 'generic',
  enabled: true,
  baseUrl: 'https://fixture.invalid',
  endpointConfig: {
    chat: { url: 'https://fixture.invalid/chat', auth: 'bearer' },
  },
  openaiChatCompletionPath: '',
  openaiResponsePath: '',
  anthropicMessagePath: '',
  useSystemProxy: false,
  hasChannelProxy: false,
  hasCustomHeaders: false,
  hasParamOverride: false,
  channelProxyDisplay: '',
}
const preset: UpstreamPreset = {
  id: 'new-api-connection',
  name: 'New API',
  label: 'New API',
  provider: 'new-api',
  platform: 'new-api',
  group: 'gateway',
  credentialMode: 'apiKey',
  defaultUrl: 'https://fixture.invalid',
  requiresBaseUrl: true,
  protocols: ['chat'],
  recommendedModels: [],
}
let client: QueryClient
function mount(node: React.ReactNode) {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}
async function chooseProvider() {
  const select = screen.getByRole('combobox', { name: 'Protocol adapter' })
  await waitFor(() => expect(select).not.toBeDisabled())
  fireEvent.click(select)
  activateOption(
    await screen.findByRole('option', { name: 'Fixture provider' })
  )
  await screen.findByLabelText('Provider mode')
}
function activateOption(option: HTMLElement) {
  fireEvent.pointerDown(option, { pointerType: 'mouse', button: 0 })
  fireEvent.click(option)
}
beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage('en')
  Element.prototype.scrollIntoView = vi.fn()
  mocks.plugins.mockResolvedValue({ items: [provider, wire] })
  mocks.config.mockResolvedValue({
    channelProxy: '',
    customHeaders: '',
    paramOverride: '',
    pluginBindings: [],
    providerBinding: binding,
  })
  mocks.update.mockResolvedValue({ success: true })
  mocks.presets.mockResolvedValue({ items: [preset] })
  mocks.connect.mockResolvedValue({
    id: 7,
    modelCount: 1,
    routeCount: 1,
    discovery: { status: 'discovered' },
  })
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('independent provider configuration', () => {
  it('edits independent image query targets and removes only a deselected slot', async () => {
    mocks.plugins.mockResolvedValue({ items: [imageProvider] })
    const changed = vi.fn()
    function Editor() {
      const [value, setValue] = useState<ProviderBinding | null>(imageBinding)
      return (
        <ChannelProviderBinding
          value={value}
          onChange={(next) => {
            changed(next)
            setValue(next)
          }}
          protocols={['imageGeneration', 'imageEdit']}
        />
      )
    }
    mount(<Editor />)
    const generate = await screen.findByLabelText(
      'Image generation task query URL template'
    )
    expect(
      screen.queryByText(/The catalog must share an origin/)
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(/declared asynchronous image submit\/query\/result/)
    ).toBeVisible()
    await waitFor(() => expect(generate).not.toBeDisabled())
    expect(generate).toHaveValue(
      imageBinding.imageQueries!.imageGeneration!.url
    )
    expect(
      screen.getByLabelText('Image edits task query URL template')
    ).toHaveValue(imageBinding.imageQueries!.imageEdit!.url)
    const auth = screen.getAllByRole('combobox', {
      name: 'Task query authentication',
    })
    expect(auth[0]).toHaveTextContent('x-api-key')
    expect(auth[1]).toHaveTextContent('none')
    expect(changed).not.toHaveBeenCalled()
    fireEvent.change(generate, {
      target: { value: 'https://fixture.invalid/custom/{taskId}' },
    })
    fireEvent.click(auth[0]!)
    activateOption(await screen.findByRole('option', { name: 'none' }))
    expect(changed).toHaveBeenLastCalledWith({
      ...imageBinding,
      imageQueries: {
        ...imageBinding.imageQueries,
        imageGeneration: {
          url: 'https://fixture.invalid/custom/{taskId}',
          auth: 'none',
        },
      },
    })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Image edits' }))
    expect(changed).toHaveBeenLastCalledWith({
      key: provider.key,
      protocols: ['imageGeneration'],
      imageQueries: {
        imageGeneration: {
          url: 'https://fixture.invalid/custom/{taskId}',
          auth: 'none',
        },
      },
    })
    expect(
      screen.queryByLabelText('Image edits task query URL template')
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('checkbox', { name: 'Image generation' })
    ).toHaveAttribute('aria-disabled', 'true')
    changed.mockClear()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Image generation' }))
    expect(changed).not.toHaveBeenCalled()
  })
  it('shows creation defaults without persisting them in the draft or replacing saved targets on upgrade', async () => {
    mocks.plugins.mockResolvedValue({ items: [imageProvider] })
    const changed = vi.fn()
    mount(
      <ChannelProviderBinding
        value={{
          key: provider.key,
          protocols: ['imageGeneration', 'imageEdit'],
        }}
        onChange={changed}
        protocols={['imageGeneration', 'imageEdit']}
        newChannel
      />
    )
    const generate = await screen.findByLabelText(
      'Image generation task query URL template'
    )
    expect(generate).toHaveValue('')
    expect(
      screen.getByText(/On creation, \/generation\/\{taskId\} is saved/)
    ).toBeVisible()
    expect(
      screen.getAllByRole('combobox', { name: 'Task query authentication' })[1]
    ).toHaveTextContent('none')
    expect(changed).not.toHaveBeenCalled()
    cleanup()
    client.clear()
    mount(
      <ChannelProviderBinding
        value={imageBinding}
        onChange={changed}
        protocols={['imageGeneration', 'imageEdit']}
      />
    )
    await screen.findByLabelText('Image generation task query URL template')
    if (imageProvider.manifest?.apiVersion !== 2)
      throw new Error('fixture manifest')
    const manifest = imageProvider.manifest
    act(() =>
      client.setQueryData(channelPluginQueryKey, {
        items: [
          {
            ...imageProvider,
            manifest: {
              ...manifest,
              version: '2',
              imageTasks: {
                imageGeneration: {
                  query: { path: '/new-default/{taskId}', auth: 'bearer' },
                },
                imageEdit: manifest.imageTasks!.imageEdit,
              },
            },
          },
        ],
      })
    )
    expect(
      screen.getByLabelText('Image generation task query URL template')
    ).toHaveValue(imageBinding.imageQueries!.imageGeneration!.url)
    expect(changed).not.toHaveBeenCalled()
    await act(async () => {
      await i18n.changeLanguage('zhCN')
    })
    expect(
      screen.getAllByRole('combobox', { name: '任务查询认证' })
    ).toHaveLength(2)
    expect(screen.getAllByText(/查询须与本图片端点同 origin/)).toHaveLength(2)
  })
  it('keeps saved image targets visible when the plugin is missing and blocks editing on metadata failure', async () => {
    mocks.plugins.mockRejectedValueOnce(new Error('fixture metadata failed'))
    const changed = vi.fn()
    mount(
      <ChannelProviderBinding
        value={imageBinding}
        onChange={changed}
        protocols={['imageGeneration', 'imageEdit']}
      />
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not load plugins'
    )
    expect(
      screen.getByLabelText('Image generation task query URL template')
    ).toBeDisabled()
    mocks.plugins.mockResolvedValueOnce({ items: [wire] })
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByText(/saved provider is unavailable/)
    expect(
      screen.getByLabelText('Image generation task query URL template')
    ).toHaveValue(imageBinding.imageQueries!.imageGeneration!.url)
    expect(changed).not.toHaveBeenCalled()
  })
  it('validates the matching image origin before PATCH and preserves a failed-save draft', async () => {
    mocks.plugins.mockResolvedValue({ items: [imageProvider] })
    mocks.config.mockResolvedValue({
      channelProxy: '',
      customHeaders: '',
      paramOverride: '',
      pluginBindings: [],
      providerBinding: imageBinding,
    })
    mocks.update.mockRejectedValueOnce(new Error('fixture save failed'))
    const imageDetail: ImportedUpstreamDetail = {
      ...detail,
      endpointConfig: {
        imageGeneration: {
          url: 'https://fixture.invalid/generate',
          auth: 'bearer',
        },
        imageEdit: { url: 'https://edits.invalid/edit', auth: 'bearer' },
      },
    }
    // The saved second slot is explicitly corrected to its own endpoint.
    mocks.config.mockResolvedValue({
      channelProxy: '',
      customHeaders: '',
      paramOverride: '',
      pluginBindings: [],
      providerBinding: {
        ...imageBinding,
        imageQueries: {
          ...imageBinding.imageQueries,
          imageEdit: {
            url: 'https://edits.invalid/tasks/{taskId}',
            auth: 'none',
          },
        },
      },
    })
    mount(
      <UpstreamConnectionForm detail={imageDetail} onDirtyChange={vi.fn()} />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit request settings' })
    )
    const generate = await screen.findByLabelText(
      'Image generation task query URL template'
    )
    fireEvent.change(generate, {
      target: { value: 'https://edits.invalid/tasks/{taskId}' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await screen.findByText(
      /Each image task query must share an origin with its own/
    )
    expect(mocks.update).not.toHaveBeenCalled()
    expect(generate).toHaveAttribute('aria-invalid', 'true')
    fireEvent.change(generate, {
      target: { value: 'https://fixture.invalid/new/{taskId}' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() => expect(mocks.update).toHaveBeenCalledOnce())
    expect(mocks.update).toHaveBeenLastCalledWith(7, {
      providerBinding: {
        ...imageBinding,
        imageQueries: {
          imageGeneration: {
            url: 'https://fixture.invalid/new/{taskId}',
            auth: 'x-api-key',
          },
          imageEdit: {
            url: 'https://edits.invalid/tasks/{taskId}',
            auth: 'none',
          },
        },
      },
    })
    expect(generate).toHaveValue('https://fixture.invalid/new/{taskId}')
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save connection' })
      ).not.toBeDisabled()
    )
  })
  it('shows immutable host authentication in both languages without a secret control', async () => {
    if (provider.manifest?.apiVersion !== 2) throw new Error('fixture manifest')
    mocks.plugins.mockResolvedValue({
      items: [
        {
          ...provider,
          manifest: {
            ...provider.manifest,
            authentication: {
              type: 'jwt_hs256',
              header: 'Authorization',
              prefix: 'Bearer ',
              jwt: {
                keySeparator: '|',
                issuerClaim: 'iss',
                ttlSeconds: 1800,
                timeUnit: 'seconds',
                notBeforeSkewSeconds: 5,
              },
            },
          },
        },
      ],
    })
    const changed = vi.fn()
    mount(
      <ChannelProviderBinding
        value={binding}
        onChange={changed}
        protocols={['chat', 'responses']}
        credentialKind='api_key'
      />
    )
    await screen.findByText('Host authentication: jwt_hs256 → Authorization')
    expect(screen.getByText(/id\|secret/)).toHaveTextContent('1800')
    expect(
      screen.getByText(/Explicit none remains credential-free/)
    ).toBeVisible()
    expect(
      screen.queryByLabelText(/secret|signing key/i)
    ).not.toBeInTheDocument()
    expect(changed).not.toHaveBeenCalled()
    await act(async () => {
      await i18n.changeLanguage('zhCN')
    })
    expect(i18n.language).toBe('zhCN')
    await screen.findByText('宿主认证：jwt_hs256 → Authorization')
    expect(screen.getByText(/id\|secret/)).toHaveTextContent('1800')
    expect(screen.getByText(/显式 none 仍不发送凭据/)).toBeVisible()
    expect(changed).not.toHaveBeenCalled()
  })
  it('allows an anonymous provider only for a compatible identity and preserves a no-auth catalog', async () => {
    if (provider.manifest?.apiVersion !== 2) throw new Error('fixture manifest')
    const anonymous: ChannelPlugin = {
      ...provider,
      manifest: {
        ...provider.manifest,
        credentialKinds: ['none'],
      },
    }
    mocks.plugins.mockResolvedValue({ items: [anonymous] })
    const changed = vi.fn()
    mount(
      <ChannelProviderBinding
        value={{
          ...binding,
          catalog: { url: binding.catalog!.url, auth: 'none' },
        }}
        onChange={changed}
        protocols={['chat']}
        credentialKind='none'
      />
    )
    const auth = await screen.findByRole('combobox', {
      name: 'Catalog authentication header',
    })
    await waitFor(() => expect(auth).not.toBeDisabled())
    expect(auth).toHaveTextContent('none')
    expect(
      screen.queryByText(/does not support the selected credential kind/)
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('combobox', { name: 'Protocol adapter' }))
    expect(
      screen.getByRole('option', { name: 'Fixture provider' })
    ).not.toHaveAttribute('aria-disabled', 'true')
    expect(changed).not.toHaveBeenCalled()
  })
  it('never offers providers as wire hooks, and derives only configured provider protocols', async () => {
    const changed = vi.fn()
    mount(
      <>
        <ChannelProviderBinding
          value={null}
          onChange={changed}
          protocols={['chat']}
          credentialKind='api_key'
        />
        <ChannelPluginBindings
          value={[]}
          onChange={vi.fn()}
          endpoints={detail.endpointConfig ?? {}}
        />
      </>
    )
    expect(
      await screen.findByRole('checkbox', { name: 'Fixture wire' })
    ).toBeVisible()
    expect(
      screen.queryByRole('checkbox', { name: 'Fixture provider' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('combobox', { name: 'Protocol adapter' }))
    expect(screen.getByRole('listbox')).toHaveAccessibleName('Protocol adapter')
    expect(
      screen.queryByRole('option', { name: 'Fixture wire' })
    ).not.toBeInTheDocument()
    activateOption(screen.getByRole('option', { name: 'Fixture provider' }))
    await waitFor(() =>
      expect(changed).toHaveBeenCalledWith({
        key: provider.key,
        protocols: ['chat'],
      })
    )
  })
  it('keeps an incompatible credential selection visible without silently changing the binding', async () => {
    const changed = vi.fn()
    mount(
      <ChannelProviderBinding
        value={binding}
        onChange={changed}
        protocols={['chat']}
        credentialKind='gcp_service_account'
      />
    )
    expect(
      await screen.findByText(/does not support the selected credential kind/)
    ).toBeVisible()
    fireEvent.click(screen.getByRole('combobox', { name: 'Protocol adapter' }))
    expect(
      screen.getByRole('option', { name: 'Fixture provider · Unavailable' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(changed).not.toHaveBeenCalled()
  })
  it('preserves missing bindings and custom values until explicit removal', async () => {
    mocks.plugins.mockResolvedValue({ items: [wire] })
    const changed = vi.fn()
    mount(
      <ChannelProviderBinding
        value={binding}
        onChange={changed}
        protocols={['chat']}
      />
    )
    expect(
      await screen.findByText(/saved provider is unavailable/)
    ).toBeVisible()
    expect(screen.getByLabelText('Model catalog URL')).toHaveValue(
      binding.catalog!.url
    )
    expect(changed).not.toHaveBeenCalled()
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Remove provider binding' })
      ).not.toBeDisabled()
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Remove provider binding' })
    )
    expect(changed).toHaveBeenCalledWith(null)
  })
  it('reports failed metadata, preserves the draft, and retries without replacing it', async () => {
    mocks.plugins.mockRejectedValueOnce(new Error('fixture metadata failed'))
    const changed = vi.fn()
    mount(
      <ChannelProviderBinding
        value={binding}
        onChange={changed}
        protocols={['chat']}
      />
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not load plugins'
    )
    expect(
      screen.getByRole('button', { name: 'Remove provider binding' })
    ).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() =>
      expect(screen.getByLabelText('Provider mode')).toHaveValue('saved')
    )
    expect(screen.getByLabelText('Model catalog URL')).toHaveValue(
      binding.catalog!.url
    )
    expect(changed).not.toHaveBeenCalled()
  })
  it('edits options without losing the custom catalog or binding order and can remove discovery separately', async () => {
    const changed = vi.fn()
    function Editor() {
      const [value, setValue] = useState<ProviderBinding | null>(binding)
      return (
        <ChannelProviderBinding
          value={value}
          onChange={(next) => {
            changed(next)
            setValue(next)
          }}
          protocols={['chat']}
        />
      )
    }
    mount(<Editor />)
    fireEvent.change(await screen.findByLabelText('Provider mode'), {
      target: { value: 'custom' },
    })
    expect(changed).toHaveBeenLastCalledWith({
      ...binding,
      options: { mode: 'custom' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Remove model discovery' })
    )
    expect(changed).toHaveBeenLastCalledWith({
      key: provider.key,
      protocols: ['chat'],
      options: { mode: 'custom' },
    })
  })
  it('reads the saved provider explicitly, PATCHes only its edits, and sends root null on removal', async () => {
    mount(<UpstreamConnectionForm detail={detail} onDirtyChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit request settings' })
    )
    fireEvent.change(await screen.findByLabelText('Provider mode'), {
      target: { value: 'edited' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenLastCalledWith(7, {
        providerBinding: { ...binding, options: { mode: 'edited' } },
      })
    )
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Remove provider binding' })
      ).not.toBeDisabled()
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Remove provider binding' })
    )
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save connection' })
      ).not.toBeDisabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenLastCalledWith(7, {
        providerBinding: null,
      })
    )
  })
  it('reveals invalid provider settings and keeps the draft after a failed save', async () => {
    mocks.update.mockRejectedValueOnce(new Error('fixture save failed'))
    mount(<UpstreamConnectionForm detail={detail} onDirtyChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit request settings' })
    )
    const catalog = await screen.findByLabelText('Model catalog URL')
    fireEvent.change(catalog, {
      target: { value: 'https://other.invalid/models' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    expect(
      await screen.findByText(/must share an origin with one of/)
    ).toBeVisible()
    expect(mocks.update).not.toHaveBeenCalled()
    fireEvent.change(catalog, {
      target: { value: 'https://fixture.invalid/new-catalog' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() => expect(mocks.update).toHaveBeenCalledOnce())
    expect(catalog).toHaveValue('https://fixture.invalid/new-catalog')
    expect(
      screen.getByRole('button', { name: 'Save connection' })
    ).not.toBeDisabled()
  })
  it('includes provider binding in the real quick-connect payload and never substitutes a wire hook', async () => {
    const created = vi.fn()
    mount(<UpstreamCreateSheet onClose={vi.fn()} onCreated={created} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Use New API' }))
    fireEvent.change(screen.getByLabelText('API Key'), {
      target: { value: 'fixture-connect-key' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    await chooseProvider()
    expect(
      screen.getByText(/driver's \/inventory catalog is saved/)
    ).toBeVisible()
    fireEvent.click(
      screen.getByRole('button', { name: 'Customize model discovery' })
    )
    fireEvent.change(screen.getByLabelText('Model catalog URL'), {
      target: { value: binding.catalog!.url },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    await waitFor(() =>
      expect(mocks.connect).toHaveBeenCalledWith({
        presetId: preset.id,
        protocols: ['chat'],
        apiKey: 'fixture-connect-key',
        baseUrl: detail.baseUrl,
        providerBinding: {
          key: provider.key,
          protocols: ['chat'],
          catalog: { url: binding.catalog!.url, auth: 'bearer' },
        },
      })
    )
    expect(created).toHaveBeenCalledWith(7)
  })
})
