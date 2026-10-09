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
  ImportedMember,
  ImportedUpstreamInventory,
} from '@/lib/api/imported-upstreams'
import type { UpstreamGroup, UpstreamModel } from '@/lib/api/upstream-catalog'
import type { UpstreamDeletionPreview } from '@/lib/api/upstream-lifecycle'

import { UpstreamRouteBindings } from '../components/upstream-route-bindings'
import { upstreamKeys } from '../lib/upstream-config'

const state = vi.hoisted(() => ({
  models: vi.fn(),
  credentials: vi.fn(),
  groups: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  bind: vi.fn(),
  preview: vi.fn(),
  remove: vi.fn(),
  member: vi.fn(),
  clear: vi.fn(),
  dirty: vi.fn(),
  message: vi.fn(),
  error: vi.fn(),
  success: vi.fn(),
  dismiss: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    getUpstreamModels: state.models,
    getImportedCredentials: state.credentials,
    getUpstreamGroups: state.groups,
    createUpstreamGroup: state.create,
    updateUpstreamGroup: state.update,
    createUpstreamMember: state.bind,
    previewUpstreamDeletion: state.preview,
    deleteUpstreamEntity: state.remove,
    updateImportedMember: state.member,
    clearImportedMemberCooldown: state.clear,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: {
    message: state.message,
    error: state.error,
    success: state.success,
    dismiss: state.dismiss,
  },
}))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children: React.ReactNode }) => (
    <a href='/token-routes'>{props.children}</a>
  ),
}))

const member: ImportedMember = {
  id: 7,
  grantId: 3,
  groupId: 8,
  groupName: 'Shared group',
  routeId: 9,
  channelId: 4,
  modelName: 'local-model',
  credentialName: 'Plan key',
  credentialEnabled: true,
  effectiveEnabled: true,
  protocols: 10,
  protocolOrder: [8, 2],
  mode: 'manual',
  activeItemId: 7,
  priority: 0,
  weight: 1,
}
const models: UpstreamModel[] = [
  {
    id: 2,
    name: 'local-model',
    enabled: true,
    ownership: 'native',
    grants: [
      {
        id: 3,
        modelId: 2,
        credentialId: 5,
        credentialName: 'Plan key',
        enabled: true,
        protocols: 10,
        memberCount: 1,
        ownership: 'native',
      },
      {
        id: 4,
        modelId: 2,
        credentialId: 6,
        credentialName: 'Spare key',
        enabled: true,
        protocols: 2,
        memberCount: 0,
        ownership: 'native',
      },
    ],
  },
]
const group: UpstreamGroup = {
  id: 8,
  name: 'Shared group',
  mode: 'manual',
  enabled: true,
  activeMemberId: 7,
  ownership: 'native',
  routeId: 9,
  modelPattern: 'public-model',
  displayName: '',
  members: [
    {
      id: 7,
      groupId: 8,
      grantId: 3,
      priority: 0,
      weight: 1,
      protocolOrder: [8, 2],
      ownership: 'native',
      modelName: 'local-model',
      credentialName: 'Plan key',
      channelId: 4,
    },
    {
      id: 202,
      groupId: 8,
      grantId: 42,
      priority: 0,
      weight: 1,
      protocolOrder: [],
      ownership: 'imported',
      modelName: 'foreign-model',
      credentialName: 'Foreign key',
      channelId: 99,
    },
  ],
}
const otherGroup: UpstreamGroup = {
  ...group,
  id: 30,
  name: 'Other group',
  modelPattern: 'other-public',
  members: [{ ...group.members[1], groupId: 30 }],
  activeMemberId: 202,
}
const preview: UpstreamDeletionPreview = {
  kind: 'member',
  id: 7,
  counts: { members: 1 },
  affectedRouteIds: [9],
  revision: 'revision-1',
  requiresCascade: false,
}
let client: QueryClient
const originalScrollIntoView = Object.getOwnPropertyDescriptor(
  Element.prototype,
  'scrollIntoView'
)
function mount(active = true, beforeDelete?: (action: () => void) => void) {
  client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  })
  client.setQueryData<ImportedUpstreamInventory>(upstreamKeys.all, {
    items: [],
    members: [member],
  })
  return render(
    <QueryClientProvider client={client}>
      <UpstreamRouteBindings
        channelId={4}
        active={active}
        members={[member]}
        onDirtyChange={state.dirty}
        beforeDelete={beforeDelete}
      />
    </QueryClientProvider>
  )
}
async function ready() {
  await screen.findByRole('button', { name: 'Edit group Shared group' })
}
async function choose(
  label: string,
  option: string | RegExp,
  scope: Pick<typeof screen, 'getByRole'> = screen
) {
  const trigger = scope.getByRole('combobox', { name: label })
  fireEvent.mouseDown(trigger)
  const item = await screen.findByRole('option', { name: option })
  fireEvent.pointerDown(item)
  fireEvent.click(item)
  await waitFor(() => expect(trigger).toHaveTextContent(option))
}
async function createForm() {
  await ready()
  fireEvent.click(screen.getByRole('button', { name: 'Create group' }))
  return within(screen.getByRole('form', { name: 'Create group' }))
}

beforeEach(() => {
  Object.defineProperty(Element.prototype, 'scrollIntoView', {
    configurable: true,
    writable: true,
    value: vi.fn(),
  })
  vi.clearAllMocks()
  state.models.mockReset().mockResolvedValue({ items: models })
  state.credentials.mockReset().mockResolvedValue({
    items: [
      { id: 5, name: 'Plan key', enabled: true },
      { id: 6, name: 'Spare key', enabled: true },
    ],
  })
  state.groups.mockReset().mockResolvedValue({ items: [group, otherGroup] })
  state.create.mockReset().mockResolvedValue(group)
  state.update.mockReset().mockResolvedValue(group)
  state.bind.mockReset().mockResolvedValue(group.members[0])
  state.preview.mockReset().mockResolvedValue(preview)
  state.remove.mockReset().mockResolvedValue({ ...preview, success: true })
  state.member.mockReset().mockResolvedValue({ success: true })
  state.message.mockReturnValue(91)
})
afterEach(() => {
  cleanup()
  if (originalScrollIntoView) {
    Object.defineProperty(
      Element.prototype,
      'scrollIntoView',
      originalScrollIntoView
    )
  } else {
    Reflect.deleteProperty(Element.prototype, 'scrollIntoView')
  }
  client?.clear()
  vi.useRealTimers()
})

describe('upstream route group management', () => {
  it.each([
    { button: 'Delete member local-model', kind: 'member', id: 7 },
    { button: 'Delete group Shared group', kind: 'group', id: 8 },
  ])(
    'defers $kind deletion until the shared dirty owner approves',
    async ({ button, kind, id }) => {
      const guard = vi.fn<(action: () => void) => void>()
      mount(true, guard)
      await ready()
      fireEvent.click(screen.getByRole('button', { name: button }))
      expect(guard).toHaveBeenCalledTimes(1)
      expect(state.preview).not.toHaveBeenCalled()
      expect(state.remove).not.toHaveBeenCalled()
      await act(async () => {
        guard.mock.calls[0][0]()
      })
      await waitFor(() => expect(state.preview).toHaveBeenCalledWith(kind, id))
    }
  )
  it('does not load catalog data while the route tab is inactive', () => {
    mount(false)
    expect(state.models).not.toHaveBeenCalled()
    expect(state.credentials).not.toHaveBeenCalled()
    expect(state.groups).not.toHaveBeenCalled()
  })
  it('creates a group using only explicitly selected authorized pairs and an exact public model', async () => {
    mount()
    const form = await createForm()
    fireEvent.change(form.getByLabelText('Group name'), {
      target: { value: 'New group' },
    })
    fireEvent.change(form.getByLabelText('Public model name'), {
      target: { value: 'new-public' },
    })
    fireEvent.click(
      form.getByRole('checkbox', { name: 'local-model · Spare key' })
    )
    fireEvent.click(form.getByRole('button', { name: 'Create group' }))
    await waitFor(() =>
      expect(state.create).toHaveBeenCalledWith({
        name: 'New group',
        mode: 'failover',
        enabled: false,
        route: { modelPattern: 'new-public' },
        members: [{ grantId: 4 }],
      })
    )
    await waitFor(() =>
      expect(state.dirty).toHaveBeenCalledWith('group-create', false)
    )
  })
  it('rejects wildcard aliases and requires a manually enabled group to select its own active grant', async () => {
    mount()
    const form = await createForm()
    fireEvent.change(form.getByLabelText('Group name'), {
      target: { value: 'New group' },
    })
    fireEvent.change(form.getByLabelText('Public model name'), {
      target: { value: 'model-*' },
    })
    fireEvent.click(
      form.getByRole('checkbox', { name: 'local-model · Plan key' })
    )
    fireEvent.click(form.getByRole('button', { name: 'Create group' }))
    expect(await form.findByText(/exact public model name/)).toBeVisible()
    expect(state.create).not.toHaveBeenCalled()
    fireEvent.change(form.getByLabelText('Public model name'), {
      target: { value: 'public-new' },
    })
    await choose('Selection mode', 'Manual', form)
    fireEvent.click(form.getByRole('switch', { name: 'Enabled' }))
    fireEvent.click(form.getByRole('button', { name: 'Create group' }))
    expect(await form.findByText(/Select an active member/)).toBeVisible()
    expect(state.create).not.toHaveBeenCalled()
    await choose('Active member', 'local-model · Plan key', form)
    fireEvent.click(form.getByRole('button', { name: 'Create group' }))
    await waitFor(() =>
      expect(state.create).toHaveBeenCalledWith(
        expect.objectContaining({
          activeGrantId: 3,
          mode: 'manual',
          enabled: true,
        })
      )
    )
  })
  it('uses the complete group member list when switching a manual group to a member on another upstream', async () => {
    mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit group Shared group' })
    )
    const form = within(screen.getByRole('form', { name: 'Edit route group' }))
    await choose(
      'Active member',
      /foreign-model · Foreign key · Channel 99/,
      form
    )
    fireEvent.click(form.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(8, {
        activeMemberId: 202,
      })
    )
  })
  it('binds a current-channel authorization to a group that currently contains only another upstream', async () => {
    mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Add to existing group' })
    )
    const form = within(
      screen.getByRole('form', { name: 'Add to existing group' })
    )
    await choose('Target group', 'Other group · other-public', form)
    await choose('Model and credential', 'local-model · Spare key', form)
    fireEvent.click(form.getByRole('button', { name: 'Add to existing group' }))
    await waitFor(() =>
      expect(state.bind).toHaveBeenCalledWith(30, { grantId: 4 })
    )
  })
  it('keeps a dirty group draft while collapsed and when catalog data refreshes', async () => {
    mount()
    await ready()
    const toggle = screen.getByRole('button', {
      name: 'Edit group Shared group',
    })
    fireEvent.click(toggle)
    fireEvent.change(
      within(
        screen.getByRole('form', { name: 'Edit route group' })
      ).getByLabelText('Group name'),
      {
        target: { value: 'Unsaved name' },
      }
    )
    fireEvent.click(toggle)
    await act(async () => {
      client.setQueryData(upstreamKeys.groups, {
        items: [{ ...group, name: 'Remote name' }, otherGroup],
      })
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Edit group Remote name' })
    )
    expect(
      within(
        screen.getByRole('form', { name: 'Edit route group' })
      ).getByLabelText('Group name')
    ).toHaveValue('Unsaved name')
    expect(state.dirty).toHaveBeenCalledWith('group-8', true)
    expect(state.update).not.toHaveBeenCalled()
  })
  it('shows a failed group save and retains the draft for retry', async () => {
    state.update.mockRejectedValueOnce(new Error('Concurrent group update'))
    mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit group Shared group' })
    )
    fireEvent.change(
      within(
        screen.getByRole('form', { name: 'Edit route group' })
      ).getByLabelText('Group name'),
      {
        target: { value: 'Draft' },
      }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Concurrent group update'
    )
    expect(
      within(
        screen.getByRole('form', { name: 'Edit route group' })
      ).getByLabelText('Group name')
    ).toHaveValue('Draft')
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  })
  it('refreshes pristine group fields and sends only edited fields after a later concurrent change', async () => {
    mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit group Shared group' })
    )
    await act(async () => {
      client.setQueryData(upstreamKeys.groups, {
        items: [
          { ...group, name: 'Refreshed', mode: 'failover', enabled: false },
          otherGroup,
        ],
      })
    })
    const form = within(screen.getByRole('form', { name: 'Edit route group' }))
    await waitFor(() =>
      expect(form.getByLabelText('Group name')).toHaveValue('Refreshed')
    )
    expect(
      form.getByRole('combobox', { name: 'Selection mode' })
    ).toHaveTextContent('Failover')
    expect(form.getByRole('switch', { name: 'Enabled' })).not.toBeChecked()
    fireEvent.change(form.getByLabelText('Group name'), {
      target: { value: 'Only name' },
    })
    state.update.mockResolvedValue({
      ...group,
      name: 'Only name',
      mode: 'manual',
      enabled: true,
    })
    fireEvent.click(form.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(state.update).toHaveBeenCalledWith(8, { name: 'Only name' })
    )
    await waitFor(() =>
      expect(
        form.getByRole('combobox', { name: 'Selection mode' })
      ).toHaveTextContent('Manual')
    )
    expect(form.getByRole('switch', { name: 'Enabled' })).toBeChecked()
  })
  it('refreshes untouched member fields without replacing its draft and patches only that draft', async () => {
    const view = mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit route Shared group' })
    )
    const replace = (next: ImportedMember) =>
      view.rerender(
        <QueryClientProvider client={client}>
          <UpstreamRouteBindings
            channelId={4}
            active
            members={[next]}
            onDirtyChange={state.dirty}
          />
        </QueryClientProvider>
      )
    replace({ ...member, weight: 3 })
    await waitFor(() => expect(screen.getByLabelText('Weight')).toHaveValue(3))
    fireEvent.change(screen.getByLabelText('Weight'), {
      target: { value: '5' },
    })
    replace({ ...member, weight: 9, priority: 6 })
    await waitFor(() =>
      expect(screen.getByLabelText('Priority')).toHaveValue(6)
    )
    expect(screen.getByLabelText('Weight')).toHaveValue(5)
    fireEvent.click(screen.getByRole('button', { name: 'Save route' }))
    await waitFor(() =>
      expect(state.member).toHaveBeenCalledWith(7, { weight: 5 })
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save route' })).toBeDisabled()
    )
    expect(screen.getByLabelText('Priority')).toHaveValue(6)
  })
  it('offers undo for a leaf member and clears its dirty marker only after the deferred delete commits', async () => {
    mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit route Shared group' })
    )
    fireEvent.change(screen.getByLabelText('Weight'), {
      target: { value: '3' },
    })
    await waitFor(() =>
      expect(state.dirty).toHaveBeenCalledWith('member-7', true)
    )
    state.dirty.mockClear()
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete member local-model' })
    )
    await waitFor(() => expect(state.message).toHaveBeenCalled())
    expect(state.remove).not.toHaveBeenCalled()
    expect(state.dirty).not.toHaveBeenCalledWith('member-7', false)
    await act(async () => {
      state.message.mock.calls[0][1].onAutoClose()
    })
    await waitFor(() =>
      expect(state.remove).toHaveBeenCalledWith('member', 7, {
        cascade: false,
        expectedRevision: 'revision-1',
      })
    )
    expect(state.dirty).toHaveBeenCalledWith('member-7', false)
  })
  it('restores a leaf member and opens explicit review when its deletion preview changes during the undo window', async () => {
    const changed = {
      ...preview,
      requiresCascade: true,
      revision: 'revision-2',
      counts: { members: 1, groups: 1 },
    }
    state.remove.mockRejectedValueOnce({
      isAxiosError: true,
      response: { status: 409, data: { preview: changed } },
    })
    mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete member local-model' })
    )
    await waitFor(() => expect(state.message).toHaveBeenCalled())
    await act(async () => {
      state.message.mock.calls[0][1].onAutoClose()
    })
    expect(await screen.findByRole('dialog')).toBeVisible()
    expect(screen.getByRole('alert')).toHaveTextContent(/changed/)
    expect(
      client.getQueryData<ImportedUpstreamInventory>(upstreamKeys.all)?.members
    ).toHaveLength(1)
    expect(state.remove).toHaveBeenCalledTimes(1)
    expect(screen.getByPlaceholderText('DELETE')).toHaveValue('')
  })
  it('requires explicit cascade review to delete a group', async () => {
    state.preview.mockResolvedValue({
      ...preview,
      kind: 'group',
      id: 8,
      requiresCascade: true,
      counts: { groups: 1, members: 2, routes: 1 },
    })
    mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete group Shared group' })
    )
    expect(await screen.findByRole('dialog')).toBeVisible()
    expect(screen.getByText('Groups deleted')).toBeVisible()
    expect(state.message).not.toHaveBeenCalled()
    expect(state.remove).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })
  it('clears the group and its member drafts after confirmed cascade deletion', async () => {
    const cascade = {
      ...preview,
      kind: 'group' as const,
      id: 8,
      requiresCascade: true,
      counts: { groups: 1, members: 2, routes: 1 },
    }
    state.preview.mockResolvedValue(cascade)
    state.remove.mockResolvedValue({ ...cascade, success: true })
    mount()
    await ready()
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit group Shared group' })
    )
    fireEvent.change(
      within(
        screen.getByRole('form', { name: 'Edit route group' })
      ).getByLabelText('Group name'),
      { target: { value: 'Draft group' } }
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Edit route Shared group' })
    )
    fireEvent.change(screen.getByLabelText('Weight'), {
      target: { value: '3' },
    })
    await waitFor(() =>
      expect(state.dirty).toHaveBeenCalledWith('member-7', true)
    )
    state.dirty.mockClear()
    vi.useFakeTimers()
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete group Shared group' })
    )
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    fireEvent.change(screen.getByPlaceholderText('DELETE'), {
      target: { value: 'DELETE' },
    })
    for (let i = 0; i < 3; i++) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000)
      })
    }
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete these records' })
    )
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(state.remove).toHaveBeenCalledWith('group', 8, {
      cascade: true,
      expectedRevision: 'revision-1',
    })
    expect(state.dirty).toHaveBeenCalledWith('group-8', false)
    expect(state.dirty).toHaveBeenCalledWith('member-7', false)
  })
  it('keeps creation explicit when no authorized pairs exist and shows a retry on catalog failure', async () => {
    state.models.mockResolvedValue({ items: [] })
    mount()
    const form = await createForm()
    expect(
      form.getByText('Add a model authorization before creating a group.')
    ).toBeVisible()
    fireEvent.change(form.getByLabelText('Group name'), {
      target: { value: 'Empty' },
    })
    fireEvent.change(form.getByLabelText('Public model name'), {
      target: { value: 'empty-public' },
    })
    fireEvent.click(form.getByRole('button', { name: 'Create group' }))
    expect(
      await form.findByText('Select at least one authorized pair.')
    ).toBeVisible()
    expect(state.create).not.toHaveBeenCalled()
    state.models.mockRejectedValueOnce(new Error('catalog unavailable'))
    await act(async () => {
      await client.invalidateQueries({ queryKey: upstreamKeys.models(4) })
    })
    expect(
      await screen.findByText(
        'Failed to load route groups: catalog unavailable'
      )
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  })
})
