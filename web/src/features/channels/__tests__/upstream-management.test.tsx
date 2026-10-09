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

import { ImportedUpstreamsPanel } from '../components/imported-upstreams-panel'
import { UpstreamConnectionForm } from '../components/upstream-connection-form'
import { UpstreamCredentials } from '../components/upstream-credentials'
import { UpstreamRouteBindings } from '../components/upstream-route-bindings'

const state = vi.hoisted(() => ({
  requestConfig: vi.fn(),
  update: vi.fn(),
  credentials: vi.fn(),
  replace: vi.fn(),
  member: vi.fn(),
  clear: vi.fn(),
  inventory: vi.fn(),
  dirty: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    getImportedRequestConfig: state.requestConfig,
    updateImportedUpstream: state.update,
    getImportedCredentials: state.credentials,
    updateImportedCredential: state.replace,
    updateImportedMember: state.member,
    clearImportedMemberCooldown: state.clear,
    getImportedUpstreams: state.inventory,
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
  name: '百炼 Coding Plan',
  originKey: 'fixture',
  provider: 'bailian',
  dialect: 'openai',
  baseUrl: 'https://upstream.example/v1',
  enabled: true,
  useSystemProxy: false,
  endpointConfig: {
    chat: { url: 'https://upstream.example/chat', auth: 'bearer' },
  },
  hasChannelProxy: true,
  hasCustomHeaders: true,
  hasParamOverride: true,
  channelProxyDisplay: '',
  openaiChatCompletionPath: '',
  openaiResponsePath: '',
  anthropicMessagePath: '',
}
const member: ImportedMember = {
  id: 7,
  grantId: 3,
  groupId: 8,
  groupName: 'coding-model',
  routeId: 9,
  channelId: 4,
  modelName: 'qwen3-coder-plus',
  credentialName: 'Plan key',
  credentialEnabled: true,
  effectiveEnabled: true,
  protocols: 10,
  protocolOrder: [8, 2],
  mode: 'failover',
  activeItemId: 7,
  priority: 0,
  weight: 1,
}
let client: QueryClient
function mount(element: React.ReactNode) {
  client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={client}>{element}</QueryClientProvider>
  )
}
beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  state.update.mockResolvedValue({ success: true, id: 4, enabled: true })
  state.replace.mockResolvedValue({ success: true, id: 2 })
  state.member.mockResolvedValue({ success: true, id: 7 })
  state.clear.mockResolvedValue({ success: true, id: 7, grantId: 3 })
  state.credentials.mockResolvedValue({
    items: [
      {
        id: 2,
        name: 'Plan key',
        enabled: true,
        kind: 'api_key',
        canRefresh: false,
      },
    ],
  })
  state.requestConfig.mockResolvedValue({
    channelProxy: 'http://proxy.example',
    customHeaders:
      '[{"header_key":"X-Fixture","header_value":"synthetic-only"}]',
    paramOverride: '{"temperature":0.5}',
  })
  state.inventory.mockResolvedValue({
    items: [{ ...detail, credentialCount: 1, modelCount: 1 }],
    members: [member],
  })
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('upstream maintenance', () => {
  it('edits a name without reading or sending hidden request settings', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={state.dirty} />
    )
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'New name' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, { name: 'New name' })
    )
    expect(state.requestConfig).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Save connection' })
    ).toBeDisabled()
  })
  it('loads request settings on demand without caching them or losing another draft', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={state.dirty} />
    )
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Draft' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit request settings' })
    )
    expect(await screen.findByDisplayValue('synthetic-only')).toBeVisible()
    expect(screen.getByLabelText('Name')).toHaveValue('Draft')
    fireEvent.change(screen.getByDisplayValue('synthetic-only'), {
      target: { value: 'synthetic-updated' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(4, {
        name: 'Draft',
        customHeaders:
          '[{"header_key":"X-Fixture","header_value":"synthetic-updated"}]',
      })
    )
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((entry) => entry.state.data)
      )
    ).not.toContain('synthetic-only')
  })
  it('reports invalid endpoint edits without sending them', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={state.dirty} />
    )
    fireEvent.change(screen.getByLabelText('Endpoint URL'), {
      target: { value: 'invalid' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    expect(
      await screen.findByText(
        'Check endpoint URLs and authentication settings.'
      )
    ).toBeVisible()
    expect(state.update).not.toHaveBeenCalled()
  })
  it('preserves failed credential replacement input and clears it after confirmed success', async () => {
    state.replace.mockRejectedValueOnce(new Error('fixture failure'))
    mount(<UpstreamCredentials id={4} active onDirtyChange={state.dirty} />)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Edit Plan key' })
    )
    fireEvent.click(screen.getByRole('switch', { name: 'Replace credential' }))
    fireEvent.change(screen.getByLabelText('API Key'), {
      target: { value: 'synthetic-replacement' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save credential' }))
    await waitFor(() =>
      expect(state.replace).toHaveBeenCalledWith(2, {
        apiKey: 'synthetic-replacement',
      })
    )
    expect(screen.getByLabelText('API Key')).toHaveValue(
      'synthetic-replacement'
    )
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save credential' })
      ).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save credential' }))
    await waitFor(() =>
      expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    )
    fireEvent.click(screen.getByRole('switch', { name: 'Replace credential' }))
    expect(screen.getByLabelText('API Key')).toHaveValue('')
    expect(client.getMutationCache().getAll()).toHaveLength(0)
  })
  it('reorders only granted protocols and explicitly restores inheritance', async () => {
    mount(
      <UpstreamRouteBindings members={[member]} onDirtyChange={state.dirty} />
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit route coding-model' })
    )
    expect(
      screen.queryByRole('checkbox', { name: 'Responses' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Move Chat up' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save route' }))
    await waitFor(() =>
      expect(state.member).toHaveBeenCalledWith(7, {
        priority: 0,
        weight: 1,
        protocolOrder: [2, 8],
      })
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save route' })).toBeDisabled()
    )
    await waitFor(() =>
      expect(
        screen.getByRole('switch', { name: 'Inherit authorized protocols' })
      ).toBeEnabled()
    )
    fireEvent.click(
      screen.getByRole('switch', { name: 'Inherit authorized protocols' })
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save route' })).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save route' }))
    await waitFor(() =>
      expect(state.member).toHaveBeenLastCalledWith(7, {
        priority: 0,
        weight: 1,
        protocolOrder: [],
      })
    )
  })
  it('does not send an empty update when a replacement draft is switched off', async () => {
    mount(<UpstreamCredentials id={4} active onDirtyChange={state.dirty} />)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Edit Plan key' })
    )
    fireEvent.click(screen.getByRole('switch', { name: 'Replace credential' }))
    fireEvent.change(screen.getByLabelText('API Key'), {
      target: { value: 'synthetic-draft' },
    })
    fireEvent.click(screen.getByRole('switch', { name: 'Replace credential' }))
    expect(
      screen.getByRole('button', { name: 'Save credential' })
    ).toBeDisabled()
    fireEvent.click(screen.getByRole('switch', { name: 'Replace credential' }))
    expect(screen.getByLabelText('API Key')).toHaveValue('synthetic-draft')
    expect(state.replace).not.toHaveBeenCalled()
  })
  it('states the shared cooldown scope and refreshes all upstream projections', async () => {
    const invalidate = vi.spyOn(QueryClient.prototype, 'invalidateQueries')
    mount(
      <UpstreamRouteBindings
        members={[
          {
            ...member,
            cooldownUntil: new Date(Date.now() + 60_000).toISOString(),
          },
        ]}
        onDirtyChange={state.dirty}
      />
    )
    expect(
      screen.getByText('Shared by routes using this credential and model.')
    ).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Clear cooldown' }))
    await waitFor(() => expect(state.clear).toHaveBeenCalledWith(7))
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith({
        queryKey: ['imported-upstreams'],
      })
    )
    invalidate.mockRestore()
  })
  it('keeps account channels primary and offers an uncluttered upstream view', async () => {
    mount(
      <ImportedUpstreamsPanel accountCount={2}>
        <p>Account rows</p>
      </ImportedUpstreamsPanel>
    )
    expect(
      await screen.findByRole('tab', { name: /Account channels/ })
    ).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByText('Account rows')).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: /Independent upstreams/ }))
    expect(
      await screen.findByRole('button', { name: '百炼 Coding Plan' })
    ).toBeVisible()
    expect(screen.queryByText('Account rows')).not.toBeInTheDocument()
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Search upstreams or models…' }),
      { target: { value: 'not-present' } }
    )
    expect(
      within(screen.getByRole('region')).getByText('No matching upstreams')
    ).toBeVisible()
  })
})
