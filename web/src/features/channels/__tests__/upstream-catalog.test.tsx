import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type {
  ImportedMember,
  ImportedUpstreamDetail,
} from '@/lib/api/imported-upstreams'
import type { UpstreamModel } from '@/lib/api/upstream-catalog'

import { UpstreamCredentials } from '../components/upstream-credentials'
import { UpstreamDetailSheet } from '../components/upstream-detail-sheet'
import {
  UpstreamGrantForm,
  UpstreamModelCreateForm,
  UpstreamModelEditForm,
} from '../components/upstream-model-forms'
import { UpstreamModels } from '../components/upstream-models'

const state = vi.hoisted(() => ({
  models: vi.fn(),
  detail: vi.fn(),
  createModels: vi.fn(),
  updateModel: vi.fn(),
  createGrant: vi.fn(),
  updateGrant: vi.fn(),
  credentials: vi.fn(),
  createCredential: vi.fn(),
  updateCredential: vi.fn(),
  preview: vi.fn(),
  remove: vi.fn(),
  dirty: vi.fn(),
  message: vi.fn(),
  dismiss: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    getUpstreamModels: state.models,
    getImportedUpstream: state.detail,
    createUpstreamModels: state.createModels,
    updateUpstreamModel: state.updateModel,
    createUpstreamGrant: state.createGrant,
    updateUpstreamGrant: state.updateGrant,
    getImportedCredentials: state.credentials,
    createUpstreamCredential: state.createCredential,
    updateImportedCredential: state.updateCredential,
    previewUpstreamDeletion: state.preview,
    deleteUpstreamEntity: state.remove,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    message: state.message,
    dismiss: state.dismiss,
  },
}))

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
const credential = {
  id: 5,
  name: 'Plan key',
  enabled: true,
  kind: 'api_key' as const,
  canRefresh: false,
}
const model: UpstreamModel = {
  id: 3,
  name: 'qwen3-coder-plus',
  enabled: true,
  ownership: 'imported',
  grants: [],
}
const detail: ImportedUpstreamDetail = {
  id: 4,
  name: 'Coding Plan',
  provider: 'bailian',
  originKey: 'fixture',
  dialect: 'generic',
  enabled: true,
  baseUrl: 'https://example.com',
  endpointConfig: { chat: { url: 'https://example.com/chat', auth: 'bearer' } },
  useSystemProxy: false,
  hasChannelProxy: false,
  hasCustomHeaders: false,
  hasParamOverride: false,
  channelProxyDisplay: '',
  openaiChatCompletionPath: '',
  openaiResponsePath: '',
  anthropicMessagePath: '',
}
beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  state.models.mockResolvedValue({ items: [model] })
  state.detail.mockResolvedValue(detail)
  state.credentials.mockResolvedValue({ items: [credential] })
  state.createModels.mockResolvedValue({ items: [] })
  state.updateModel.mockResolvedValue({ success: true })
  state.createGrant.mockResolvedValue({ id: 7 })
  state.updateGrant.mockResolvedValue({ id: 7 })
  state.createCredential.mockResolvedValue({ id: 8 })
  state.remove.mockResolvedValue({ success: true })
  state.message.mockReturnValue('undo')
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('upstream catalog management', () => {
  it('keeps a credential submission locked when a background refresh resets RHF state', async () => {
    let finish!: (value: { success: boolean }) => void
    state.updateCredential.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        })
    )
    mount(<UpstreamCredentials id={4} active onDirtyChange={state.dirty} />)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Edit Plan key' })
    )
    fireEvent.click(screen.getByRole('switch', { name: 'Replace credential' }))
    fireEvent.change(screen.getByLabelText('API Key'), {
      target: { value: 'synthetic-pending-value' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save credential' }))
    await waitFor(() => expect(state.updateCredential).toHaveBeenCalledTimes(1))
    await act(async () => {
      client.setQueryData(['imported-upstreams', 4, 'credentials'], {
        items: [{ ...credential, enabled: false }],
      })
    })
    expect(
      screen.getByRole('button', { name: 'Save credential' })
    ).toBeDisabled()
    expect(screen.getByLabelText('API Key')).toBeDisabled()
    await act(async () => {
      finish({ success: true })
    })
    expect(state.updateCredential).toHaveBeenCalledTimes(1)
  })
  it('retains the refreshed untouched fields when a draft is manually reverted', async () => {
    const view = mount(
      <UpstreamModelEditForm model={model} onDirtyChange={state.dirty} />
    )
    const input = screen.getByLabelText('Upstream model name')
    fireEvent.change(input, { target: { value: 'temporary-name' } })
    view.rerender(
      <QueryClientProvider client={client}>
        <UpstreamModelEditForm
          model={{ ...model, enabled: false }}
          onDirtyChange={state.dirty}
        />
      </QueryClientProvider>
    )
    expect(input).toHaveValue('temporary-name')
    fireEvent.change(input, { target: { value: model.name } })
    expect(screen.getByRole('switch', { name: 'Enabled' })).not.toBeChecked()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save model' })).toBeDisabled()
    )
  })
  it('refreshes pristine model fields, preserves a draft and only patches its changed fields', async () => {
    const view = mount(
      <UpstreamModelEditForm model={model} onDirtyChange={state.dirty} />
    )
    client.setQueryDefaults(['routes'], { gcTime: Infinity })
    client.setQueryData(['routes', 'summary'], { rows: [] })
    const renderModel = (next: UpstreamModel) => (
      <QueryClientProvider client={client}>
        <UpstreamModelEditForm model={next} onDirtyChange={state.dirty} />
      </QueryClientProvider>
    )
    view.rerender(
      renderModel({ ...model, name: 'server-model', enabled: false })
    )
    expect(screen.getByLabelText('Upstream model name')).toHaveValue(
      'server-model'
    )
    expect(screen.getByRole('switch', { name: 'Enabled' })).not.toBeChecked()
    fireEvent.change(screen.getByLabelText('Upstream model name'), {
      target: { value: 'draft-model' },
    })
    view.rerender(
      renderModel({ ...model, name: 'external-model', enabled: true })
    )
    expect(screen.getByLabelText('Upstream model name')).toHaveValue(
      'draft-model'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save model' }))
    await waitFor(() =>
      expect(state.updateModel).toHaveBeenCalledWith(3, { name: 'draft-model' })
    )
    await waitFor(() =>
      expect(client.getQueryState(['routes', 'summary'])?.isInvalidated).toBe(
        true
      )
    )
    expect(screen.getByRole('switch', { name: 'Enabled' })).toBeChecked()
  })
  it('keeps an unsaved model draft when deletion is cancelled before preview', async () => {
    mount(
      <UpstreamDetailSheet
        item={{ ...detail, modelCount: 1, credentialCount: 1 }}
        members={[]}
        onClose={vi.fn()}
      />
    )
    fireEvent.click(await screen.findByRole('tab', { name: 'Models' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'Edit model qwen3-coder-plus' })
    )
    const input = screen.getByLabelText('Upstream model name')
    fireEvent.change(input, { target: { value: 'unsaved-model-name' } })
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete model qwen3-coder-plus' })
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Keep editing' }))
    expect(input).toHaveValue('unsaved-model-name')
    expect(state.preview).not.toHaveBeenCalled()
    expect(state.remove).not.toHaveBeenCalled()
  })
  it('adds distinct model names without silently granting credential access', async () => {
    mount(
      <UpstreamModelCreateForm
        channelId={4}
        onCreated={vi.fn()}
        onDirtyChange={state.dirty}
      />
    )
    const input = screen.getByLabelText('Upstream model names (one per line)')
    fireEvent.change(input, {
      target: { value: 'qwen3-coder-plus\nqwen3-coder-plus' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add models' }))
    expect(await screen.findByText('Model names must be unique')).toBeVisible()
    expect(state.createModels).not.toHaveBeenCalled()
    fireEvent.change(input, {
      target: { value: ' qwen3-coder-plus \n\n deepseek-chat ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add models' }))
    await waitFor(() =>
      expect(state.createModels).toHaveBeenCalledWith(4, {
        names: ['qwen3-coder-plus', 'deepseek-chat'],
      })
    )
    expect(state.createGrant).not.toHaveBeenCalled()
    expect(input).toHaveValue('')
  })
  it('edits the actual model while retaining failed edits and clearing dirty state on unmount', async () => {
    state.updateModel.mockRejectedValueOnce(new Error('conflict'))
    const view = mount(
      <UpstreamModelEditForm model={model} onDirtyChange={state.dirty} />
    )
    const input = screen.getByLabelText('Upstream model name')
    fireEvent.change(input, { target: { value: 'qwen3-coder-next' } })
    fireEvent.click(screen.getByRole('switch', { name: 'Enabled' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save model' }))
    await waitFor(() =>
      expect(state.updateModel).toHaveBeenCalledWith(3, {
        name: 'qwen3-coder-next',
        enabled: false,
      })
    )
    expect(input).toHaveValue('qwen3-coder-next')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save model' })).toBeEnabled()
    )
    view.unmount()
    expect(state.dirty).toHaveBeenLastCalledWith('model-3', false)
  })
  it('requires an explicit credential and protocol, and only offers executable protocols', async () => {
    mount(
      <UpstreamGrantForm
        modelId={3}
        credentials={[credential]}
        availableProtocols={[2]}
        members={[]}
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Add access' }))
    await waitFor(() =>
      expect(screen.getAllByText('Select a credential').length).toBeGreaterThan(
        0
      )
    )
    expect(state.createGrant).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('checkbox', { name: 'Messages' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('Select at least one protocol')).toBeVisible()
    fireEvent.click(screen.getByRole('combobox'))
    fireEvent.click(await screen.findByRole('option', { name: 'Plan key' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Chat' }))
    fireEvent.click(screen.getByRole('button', { name: 'Add access' }))
    await waitFor(() =>
      expect(state.createGrant).toHaveBeenCalledWith({
        modelId: 3,
        credentialId: 5,
        protocols: [2],
        enabled: true,
      })
    )
  })
  it('keeps a rejected protocol reduction and names the routes that still use it', async () => {
    state.updateGrant.mockRejectedValueOnce({
      isAxiosError: true,
      response: { status: 409, data: { conflictingMemberIds: [9] } },
    })
    mount(
      <UpstreamGrantForm
        modelId={3}
        credentials={[credential]}
        availableProtocols={[2, 8]}
        grant={{
          id: 7,
          modelId: 3,
          credentialId: 5,
          credentialName: 'Plan key',
          protocols: 10,
          enabled: true,
          memberCount: 1,
          ownership: 'native',
        }}
        members={[{ id: 9, groupName: 'Coding route' } as ImportedMember]}
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Messages' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save access' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Coding route')
    expect(screen.getByRole('checkbox', { name: 'Messages' })).not.toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Chat' })).toBeChecked()
    expect(state.updateGrant).toHaveBeenCalledExactlyOnceWith(7, {
      protocols: [2],
    })
  })
  it('retains a failed new key only in the form, then clears it after creation', async () => {
    state.createCredential.mockRejectedValueOnce(new Error('conflict'))
    mount(<UpstreamCredentials id={4} active onDirtyChange={state.dirty} />)
    await screen.findByText('Plan key')
    fireEvent.click(screen.getByRole('button', { name: 'Add credential' }))
    fireEvent.change(screen.getAllByLabelText('Name')[0], {
      target: { value: 'Second plan' },
    })
    const input = screen.getByLabelText('API Key')
    fireEvent.change(input, { target: { value: 'synthetic-catalog-value' } })
    const buttons = screen.getAllByRole('button', { name: 'Add credential' })
    fireEvent.click(buttons[1])
    await waitFor(() =>
      expect(state.createCredential).toHaveBeenCalledWith(4, {
        name: 'Second plan',
        enabled: false,
        apiKey: 'synthetic-catalog-value',
      })
    )
    expect(input).toHaveValue('synthetic-catalog-value')
    await waitFor(() => expect(buttons[1]).toBeEnabled())
    fireEvent.click(buttons[1])
    await waitFor(() =>
      expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    )
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((query) => query.state.data)
      )
    ).not.toContain('synthetic-catalog-value')
    expect(client.getMutationCache().getAll()).toHaveLength(0)
  })
  it('previews deletion, defers a leaf delete, and undo restores the model without a request', async () => {
    state.preview.mockResolvedValue({
      kind: 'model',
      id: 3,
      counts: { models: 1 },
      affectedRouteIds: [],
      revision: 'revision-a',
      requiresCascade: false,
    })
    mount(
      <UpstreamModels
        detail={detail}
        active
        members={[]}
        onDirtyChange={state.dirty}
      />
    )
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Delete model qwen3-coder-plus',
      })
    )
    await waitFor(() => expect(state.message).toHaveBeenCalled())
    expect(state.preview).toHaveBeenCalledWith('model', 3)
    expect(state.remove).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('button', { name: 'Delete model qwen3-coder-plus' })
    ).not.toBeInTheDocument()
    state.message.mock.calls[0][1].action.onClick()
    expect(
      await screen.findByRole('button', {
        name: 'Delete model qwen3-coder-plus',
      })
    ).toBeVisible()
    expect(state.remove).not.toHaveBeenCalled()
  })
})
