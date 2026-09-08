// Real form/checkbox interactions for manual channel binding: empty eligibility,
// guided seeds, bulk selection, submission and preservation of operator edits.
// Only the route API and toast boundaries are mocked.

import '@testing-library/jest-dom/vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ComponentProps } from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import i18n from '@/i18n/config'

import { RouteFormDialog } from '../components/route-form-dialog'
import type { RouteChannel, RouteSummaryRow } from '../types'

const {
  mockCreateMutate,
  mockUpdateMutate,
  mockBatchMutate,
  mockUpdateChannel,
  mockDeleteChannel,
  channels,
} = vi.hoisted(() => ({
  mockCreateMutate: vi.fn(),
  mockUpdateMutate: vi.fn(),
  mockBatchMutate: vi.fn(),
  mockUpdateChannel: vi.fn(),
  mockDeleteChannel: vi.fn(),
  channels: [] as RouteChannel[],
}))

vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  useCreateRoute: () => ({
    mutateAsync: mockCreateMutate,
    isPending: false,
  }),
  useUpdateRoute: () => ({
    mutateAsync: mockUpdateMutate,
    isPending: false,
  }),
  useBatchAddChannels: () => ({
    mutateAsync: mockBatchMutate,
    isPending: false,
  }),
  useRouteChannels: () => ({
    data: channels,
    isLoading: false,
    isFetching: false,
  }),
  useUpdateChannel: () => ({
    mutate: mockUpdateChannel,
    isPending: false,
  }),
  useDeleteChannel: () => ({
    mutate: mockDeleteChannel,
    isPending: false,
  }),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

beforeEach(async () => {
  await i18n.changeLanguage('en')
  mockCreateMutate.mockReset().mockResolvedValue({ id: 1 })
  mockUpdateMutate.mockReset().mockResolvedValue({ success: true })
  mockBatchMutate.mockReset().mockResolvedValue({ created: 2, errors: [] })
  mockUpdateChannel.mockReset()
  mockDeleteChannel.mockReset()
  channels.length = 0
})

afterEach(() => cleanup())

function renderDialog(
  props: Partial<ComponentProps<typeof RouteFormDialog>> = {}
) {
  return render(
    <RouteFormDialog
      open
      onOpenChange={() => {}}
      mode='create'
      availableRoutes={[]}
      accountOptions={[]}
      {...props}
    />
  )
}

describe('RouteFormDialog channel drafts', () => {
  it('explains account eligibility without requiring model discovery', async () => {
    renderDialog()

    expect(
      await screen.findByText(/No eligible accounts are loaded/)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Auto-rebuild' })
    ).not.toBeInTheDocument()
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0)
  })

  it('keeps edit-mode guidance separate from the existing channel editor', async () => {
    renderDialog({ mode: 'edit', route: editableRoute })

    expect(
      await screen.findByText(
        /No eligible accounts are loaded for manual binding/
      )
    ).toBeInTheDocument()
    expect(screen.getByText('Route channels')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Auto-rebuild' })
    ).not.toBeInTheDocument()
  })

  it('keeps the checkbox list when account options exist', async () => {
    renderDialog({
      accountOptions: [
        { id: 7, label: 'account-seven' },
        { id: 8, label: 'account-eight' },
      ],
    })

    await screen.findByText('account-seven')
    expect(screen.getByText('account-eight')).toBeInTheDocument()
    expect(
      screen.getByRole('checkbox', { name: 'account-seven' })
    ).not.toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: 'account-eight' })
    ).not.toBeChecked()
    expect(
      screen.queryByRole('button', { name: 'Auto-rebuild' })
    ).not.toBeInTheDocument()
  })

  it('shows the guided chain seed as a checked channel candidate', async () => {
    renderDialog({
      accountOptions: [{ id: 7, label: 'account-seven' }],
      chainContext: { accountId: 7, siteId: 3 },
    })

    const checkbox = await screen.findByRole('checkbox', {
      name: 'account-seven',
    })
    expect(checkbox).toBeChecked()
  })
})

const accountOptions = [
  { id: 7, label: 'account-seven' },
  { id: 8, label: 'account-eight' },
]

const editableRoute: RouteSummaryRow = {
  id: 42,
  routeMode: 'pattern',
  modelPattern: 'gpt-5.5',
  displayName: null,
  displayIcon: null,
  modelMapping: null,
  enabled: true,
  channelCount: 1,
  enabledChannelCount: 0,
  siteNames: [],
  decisionSnapshot: null,
  decisionRefreshedAt: null,
}

describe('RouteFormDialog bulk channel selection', () => {
  it('selects 120 accounts in one action and submits each account once', async () => {
    const accounts = Array.from({ length: 120 }, (_, index) => ({
      id: index + 1,
      label: `account-${index + 1}`,
    }))
    renderDialog({
      accountOptions: accounts,
      chainContext: { accountId: 1 },
    })

    const selectAll = await screen.findByRole('checkbox', {
      name: 'Select all accounts',
    })
    expect(selectAll).toBePartiallyChecked()
    expect(screen.getByRole('status')).toHaveTextContent('1 / 120 selected')

    fireEvent.click(selectAll)

    expect(
      screen.getByRole('checkbox', { name: 'Deselect all accounts' })
    ).toBeChecked()
    expect(screen.getByRole('status')).toHaveTextContent('120 / 120 selected')
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes).toHaveLength(121)
    for (const checkbox of checkboxes) {
      expect(checkbox).toBeChecked()
    }
    expect(mockCreateMutate).not.toHaveBeenCalled()
    expect(mockBatchMutate).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('checkbox', { name: 'account-2' }))
    const partialSelection = screen.getByRole('checkbox', {
      name: 'Select all accounts',
    })
    expect(partialSelection).toBePartiallyChecked()
    expect(screen.getByRole('status')).toHaveTextContent('119 / 120 selected')
    fireEvent.click(partialSelection)
    fireEvent.change(screen.getByLabelText('Model match rule'), {
      target: { value: 'gpt-5.5' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add route' }))

    await waitFor(() => {
      expect(mockBatchMutate).toHaveBeenCalledWith({
        routeId: 1,
        channels: expect.arrayContaining(
          accounts.map((account) => ({ accountId: account.id }))
        ),
      })
    })
    expect(mockBatchMutate.mock.calls[0][0].channels).toHaveLength(120)
  })

  it('deselects every candidate and submits without manual bindings', async () => {
    const onOpenChange = vi.fn()
    renderDialog({ accountOptions, onOpenChange })
    fireEvent.click(
      await screen.findByRole('checkbox', { name: 'Select all accounts' })
    )
    fireEvent.click(
      screen.getByRole('checkbox', { name: 'Deselect all accounts' })
    )

    const selectAll = screen.getByRole('checkbox', {
      name: 'Select all accounts',
    })
    expect(selectAll).not.toBeChecked()
    expect(selectAll).not.toBePartiallyChecked()
    expect(screen.getByRole('status')).toHaveTextContent('0 / 2 selected')
    for (const checkbox of screen.getAllByRole('checkbox')) {
      expect(checkbox).not.toBeChecked()
    }
    fireEvent.change(screen.getByLabelText('Model match rule'), {
      target: { value: 'gpt-5.5' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add route' }))

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(mockCreateMutate).toHaveBeenCalled()
    expect(mockBatchMutate).not.toHaveBeenCalled()
  })

  it.each([
    {
      language: 'en',
      select: 'Select all accounts',
      deselect: 'Deselect all accounts',
    },
    { language: 'zhCN', select: '全选账号', deselect: '取消全选账号' },
  ])(
    'supports labeled keyboard selection for a single account in $language',
    async ({ language, select, deselect }) => {
      await i18n.changeLanguage(language)
      renderDialog({ accountOptions: [accountOptions[0]] })
      const selectAll = await screen.findByRole('checkbox', { name: select })
      act(() => selectAll.focus())
      expect(selectAll).toHaveFocus()
      fireEvent.keyDown(selectAll, { key: ' ', code: 'Space' })
      fireEvent.keyUp(selectAll, { key: ' ', code: 'Space' })

      expect(screen.getByRole('checkbox', { name: deselect })).toBeChecked()
      const account = screen.getByRole('checkbox', { name: 'account-seven' })
      expect(account).toBeChecked()
      fireEvent.click(account)
      expect(screen.getByRole('checkbox', { name: select })).not.toBeChecked()
      expect(
        screen.getByRole('checkbox', { name: select })
      ).not.toBePartiallyChecked()
      fireEvent.click(screen.getByText(select))
      expect(account).toBeChecked()
    }
  )

  it('becomes partially selected when new candidates arrive without clearing current choices', async () => {
    const view = renderDialog({ accountOptions })
    fireEvent.click(
      await screen.findByRole('checkbox', { name: 'Select all accounts' })
    )
    view.rerender(
      <RouteFormDialog
        open
        onOpenChange={() => {}}
        mode='create'
        availableRoutes={[]}
        accountOptions={[...accountOptions, { id: 9, label: 'account-nine' }]}
      />
    )

    const selectAll = screen.getByRole('checkbox', {
      name: 'Select all accounts',
    })
    expect(selectAll).toBePartiallyChecked()
    expect(screen.getByRole('status')).toHaveTextContent('2 / 3 selected')
    expect(
      screen.getByRole('checkbox', { name: 'account-seven' })
    ).toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: 'account-eight' })
    ).toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: 'account-nine' })
    ).not.toBeChecked()
    fireEvent.click(selectAll)
    expect(screen.getByRole('status')).toHaveTextContent('3 / 3 selected')
    expect(screen.getByRole('checkbox', { name: 'account-nine' })).toBeChecked()
  })

  it('preserves route fields and existing channel edits through bulk selection in edit mode', async () => {
    channels.push({
      id: 50,
      routeId: editableRoute.id,
      accountId: 99,
      tokenId: 990,
      sourceModel: 'upstream-model',
      weight: 17,
      priority: 4,
      enabled: false,
      manualOverride: true,
      successCount: 0,
      failCount: 0,
      account: { username: 'bound-account' },
    })
    renderDialog({ mode: 'edit', route: editableRoute, accountOptions })
    const weight = await screen.findByRole('spinbutton', { name: 'Weight' })
    fireEvent.change(weight, { target: { value: '23' } })
    fireEvent.change(screen.getByLabelText('Context length (optional)'), {
      target: { value: '8192' },
    })
    const mapping = '{"gpt-5.5":"upstream-model"}'
    fireEvent.change(screen.getByLabelText('Model mapping (optional)'), {
      target: { value: mapping },
    })

    fireEvent.click(
      screen.getByRole('checkbox', { name: 'Select all accounts' })
    )
    fireEvent.click(
      screen.getByRole('checkbox', { name: 'Deselect all accounts' })
    )
    fireEvent.click(
      screen.getByRole('checkbox', { name: 'Select all accounts' })
    )

    expect(weight).toHaveValue(23)
    expect(screen.getByRole('spinbutton', { name: 'Priority' })).toHaveValue(4)
    expect(screen.getByRole('switch', { name: 'Enable' })).not.toBeChecked()
    expect(screen.getByText('upstream-model')).toBeInTheDocument()
    expect(screen.getByLabelText('Context length (optional)')).toHaveValue(8192)
    expect(screen.getByLabelText('Model mapping (optional)')).toHaveValue(
      mapping
    )
    expect(mockUpdateChannel).not.toHaveBeenCalled()
    expect(mockDeleteChannel).not.toHaveBeenCalled()
    fireEvent.blur(weight)
    expect(mockUpdateChannel).toHaveBeenCalledWith(
      { id: 50, data: { weight: 23 } },
      expect.any(Object)
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(mockBatchMutate).toHaveBeenCalledWith({
        routeId: editableRoute.id,
        channels: [{ accountId: 7 }, { accountId: 8 }],
      })
    })
    expect(mockUpdateMutate).toHaveBeenCalledWith({
      id: editableRoute.id,
      payload: expect.objectContaining({
        modelPattern: 'gpt-5.5',
        modelMapping: mapping,
        contextLength: 8192,
      }),
    })
    expect(mockCreateMutate).not.toHaveBeenCalled()
  })

  it('asks before discarding a bulk-only selection when the dialog is canceled', async () => {
    const onOpenChange = vi.fn()
    renderDialog({ accountOptions, onOpenChange })
    fireEvent.click(
      await screen.findByRole('checkbox', { name: 'Select all accounts' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(
      await screen.findByText('Discard unsaved changes?')
    ).toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    expect(mockBatchMutate).not.toHaveBeenCalled()
  })
})
