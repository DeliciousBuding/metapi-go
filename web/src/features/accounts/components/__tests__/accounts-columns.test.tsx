// Behavior tests for the inline enable/disable button added to each account
// row's actions cell. The inline `Power` button surfaces the status-toggle
// action as one click (before the `MoreHorizontal` dropdown trigger). These
// tests assert only user-visible behavior: the button's accessible name, its
// click payload, its per-row disabled state while a toggle is pending, and
// the aria-label flip between enable/disable. Mocks are limited to the
// mutation boundary (`../api`) so the component path under test stays real.
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
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

import type { Account, AccountRowActions } from '../../types'
import { AccountsRowActions } from '../accounts-columns'

// vi.hoisted ensures `mockToggle` exists before the `vi.mock` factory runs.
// `pendingState` is a mutable holder so individual tests can flip the
// mocked mutation's pending state (isPending + variables) before re-render,
// mirroring how the live page derives `pendingStatusId` from the TanStack
// Query mutation's `variables`. The component under test receives
// `pendingStatusId` as a prop, so the mock is defensive — it stands in for
// the real hook in case a future refactor pulls the hook into the cell.
const { mockToggle, pendingState } = vi.hoisted(() => ({
  mockToggle: vi.fn(),
  pendingState: {
    isPending: false,
    variables: undefined as { id: number; status: string } | undefined,
  },
}))

vi.mock('../../api', () => ({
  useToggleAccountStatus: () => ({
    mutate: mockToggle,
    isPending: pendingState.isPending,
    variables: pendingState.variables,
  }),
}))

beforeAll(() => {
  // jsdom does not ship matchMedia; Base UI components read it on mount.
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
})

beforeEach(() => {
  mockToggle.mockReset()
  pendingState.isPending = false
  pendingState.variables = undefined
})

afterEach(() => cleanup())

// A known account row used across tests. Status starts 'active' so the
// inline button's aria-label is "Disable" (the action that will be taken).
// Cast as Account — the component reads only a handful of fields directly
// (id, status, isPinned, checkinEnabled, capabilities?.canCheckin), and the
// schema parse happens upstream in the columns hook, not in this component.
const baseAccount = {
  id: 42,
  siteId: 1,
  username: 'smoke-account',
  status: 'active',
  isPinned: false,
  checkinEnabled: false,
  capabilities: { canCheckin: false, canRefreshBalance: false, proxyOnly: false },
} as unknown as Account

// Actions object that mirrors the page wiring: `onToggleStatus` forwards the
// computed `{ id, status }` payload to `mockToggle` (the mocked mutate fn),
// so tests can assert on the exact payload the live page would send.
function buildActions(): AccountRowActions {
  return {
    onEdit: vi.fn(),
    onDelete: vi.fn(),
    onRefresh: vi.fn(),
    onViewDetail: vi.fn(),
    onTogglePin: vi.fn(),
    onToggleCheckin: vi.fn(),
    onToggleStatus: (account) =>
      mockToggle({
        id: account.id,
        status: account.status === 'active' ? 'disabled' : 'active',
      }),
  }
}

describe('AccountsRowActions inline enable/disable', () => {
  it('renders the inline toggle button and the dropdown trigger', async () => {
    render(
      <AccountsRowActions
        account={baseAccount}
        actions={buildActions()}
        pendingStatusId={null}
      />
    )

    // Inline button has the "Disable" accessible name (account is active →
    // clicking would disable it). The existing dropdown ⋮ trigger still
    // renders alongside it under the "Account actions" label.
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Disable' })
      ).toBeInTheDocument()
    })
    expect(
      screen.getByRole('button', { name: 'Account actions' })
    ).toBeInTheDocument()
  })

  it('calls the toggle mutate with { id, status: "disabled" } for an active account', async () => {
    const actions = buildActions()
    render(
      <AccountsRowActions
        account={baseAccount}
        actions={actions}
        pendingStatusId={null}
      />
    )

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Disable' })
      ).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Disable' }))

    expect(mockToggle).toHaveBeenCalledTimes(1)
    expect(mockToggle).toHaveBeenCalledWith({ id: 42, status: 'disabled' })
  })

  it('disables only the inline button for the pending row, leaving the dropdown trigger enabled', async () => {
    // Mimic the page's pending state: the TanStack Query mutation is in flight
    // for this row's account id, so the inline button shows a spinner and is
    // disabled. The dropdown ⋮ trigger stays clickable — no global lock.
    pendingState.isPending = true
    pendingState.variables = { id: 42, status: 'disabled' }
    render(
      <AccountsRowActions
        account={baseAccount}
        actions={buildActions()}
        pendingStatusId={42}
      />
    )

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Disable' })
      ).toBeDisabled()
    })
    expect(
      screen.getByRole('button', { name: 'Account actions' })
    ).not.toBeDisabled()
  })

  it('flips the aria-label to "Enable" when the account status is "disabled"', async () => {
    const disabledAccount = {
      ...baseAccount,
      status: 'disabled' as const,
    }
    render(
      <AccountsRowActions
        account={disabledAccount}
        actions={buildActions()}
        pendingStatusId={null}
      />
    )

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Enable' })
      ).toBeInTheDocument()
    })
    // The "Disable" label (for an active account) must not also be present.
    expect(
      screen.queryByRole('button', { name: 'Disable' })
    ).not.toBeInTheDocument()
  })

  it('calls the toggle mutate with { id, status: "active" } for a disabled account', async () => {
    const disabledAccount = {
      ...baseAccount,
      status: 'disabled' as const,
    }
    render(
      <AccountsRowActions
        account={disabledAccount}
        actions={buildActions()}
        pendingStatusId={null}
      />
    )

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Enable' })
      ).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Enable' }))

    expect(mockToggle).toHaveBeenCalledTimes(1)
    expect(mockToggle).toHaveBeenCalledWith({ id: 42, status: 'active' })
  })
})
