// Behavior tests for the route form dialog's channel-drafts section
// (2026-08-18 multi-perspective review journey break: the section used to
// vanish when no account options existed yet, hiding the guided chain's
// preselected account). Asserts the empty-state hint + inline rebuild
// affordance, the populated checkbox list, and the chain seed visibility.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import { RouteFormDialog } from '../components/route-form-dialog'

const {
  mockRebuildMutate,
  mockCreateMutate,
  mockUpdateMutate,
  mockBatchMutate,
} = vi.hoisted(() => ({
  mockRebuildMutate: vi.fn(),
  mockCreateMutate: vi.fn(),
  mockUpdateMutate: vi.fn(),
  mockBatchMutate: vi.fn(),
}))

vi.mock('../api', () => ({
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
  useRebuildRoutes: () => ({
    mutate: mockRebuildMutate,
    isPending: false,
  }),
  resolveCreatedRouteId: () => 1,
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

beforeEach(() => {
  mockRebuildMutate.mockReset()
  mockCreateMutate.mockReset()
  mockUpdateMutate.mockReset()
  mockBatchMutate.mockReset()
})

afterEach(() => cleanup())

function renderDialog(props?: {
  accountOptions?: { id: number; label: string }[]
  chainContext?: { accountId?: number; siteId?: number }
}) {
  return render(
    <RouteFormDialog
      open
      onOpenChange={() => {}}
      mode='create'
      availableRoutes={[]}
      accountOptions={props?.accountOptions ?? []}
      chainContext={props?.chainContext}
    />
  )
}

describe('RouteFormDialog channel drafts', () => {
  it('shows the empty-state hint and rebuild action before model discovery', async () => {
    renderDialog()

    const hint = await screen.findByText(
      /accounts are discovered during the model scan/
    )
    expect(hint).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Auto-rebuild' })
    ).toBeInTheDocument()
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0)
  })

  it('triggers a model-refresh rebuild from the empty state', async () => {
    renderDialog()

    fireEvent.click(await screen.findByRole('button', { name: 'Auto-rebuild' }))

    expect(mockRebuildMutate).toHaveBeenCalledTimes(1)
    expect(mockRebuildMutate).toHaveBeenCalledWith({ refreshModels: true })
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
    expect(screen.getAllByRole('checkbox')).toHaveLength(2)
    expect(
      screen.queryByRole('button', { name: 'Auto-rebuild' })
    ).not.toBeInTheDocument()
  })

  it('shows the guided chain seed as a checked channel candidate', async () => {
    renderDialog({
      accountOptions: [{ id: 7, label: 'account-seven' }],
      chainContext: { accountId: 7, siteId: 3 },
    })

    const checkbox = (await screen.findByRole('checkbox')) as HTMLButtonElement
    expect(checkbox).toBeChecked()
  })
})
