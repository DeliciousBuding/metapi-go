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
import type { ChannelPlugin, ProviderBinding } from '@/lib/api/channel-plugins'
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
