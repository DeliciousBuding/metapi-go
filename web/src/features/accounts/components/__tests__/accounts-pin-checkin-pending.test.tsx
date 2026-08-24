// Behavior tests for the per-row pending state on the pin / check-in
// dropdown toggles (Wave 11 feedback loops). While a row's pin or check-in
// toggle is in flight, that row's dropdown item is disabled and shows the
// canonical Spinner (role=status); the two toggles keep independent pending
// ids (no cross-talk), and other rows stay fully interactive. Mirrors the
// sites actions-cell pending tests (#889).
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import {
  afterAll,
  afterEach,
  beforeAll,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import type { Account, AccountRowActions } from '../../types'
import { AccountsRowActions } from '../accounts-columns'

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

afterEach(() => cleanup())

afterAll(() => {
  vi.restoreAllMocks()
})

// Check-in capable row: the dropdown only renders the check-in item when
// capabilities.canCheckin is true.
const baseAccount = {
  id: 42,
  siteId: 1,
  username: 'smoke-account',
  status: 'active',
  isPinned: false,
  checkinEnabled: false,
  capabilities: {
    canCheckin: true,
    canRefreshBalance: false,
    proxyOnly: false,
  },
} as unknown as Account

function buildActions(): AccountRowActions {
  return {
    onEdit: vi.fn(),
    onDelete: vi.fn(),
    onRefresh: vi.fn(),
    onViewDetail: vi.fn(),
    onTogglePin: vi.fn(),
    onToggleCheckin: vi.fn(),
    onToggleStatus: vi.fn(),
  }
}

async function openRowMenu() {
  fireEvent.click(screen.getByRole('button', { name: 'Account actions' }))
  return screen.findByRole('menuitem', { name: /Pin/ })
}

describe('AccountsRowActions pin/check-in pending', () => {
  it('disables the pin item and shows a spinner while that pin toggle is pending', async () => {
    render(
      <AccountsRowActions
        account={baseAccount}
        actions={buildActions()}
        pendingPinId={42}
      />
    )

    await openRowMenu()
    const pinItem = screen.getByRole('menuitem', { name: /Pin/ })
    expect(pinItem).toHaveAttribute('aria-disabled', 'true')
    // The canonical Spinner (role=status) renders inside the pending item.
    expect(within(pinItem).getByRole('status')).toBeInTheDocument()
    // Non-mutating actions stay enabled — no global lock.
    expect(
      screen.getByRole('menuitem', { name: 'View details' })
    ).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('disables the check-in item and shows a spinner while that check-in toggle is pending', async () => {
    render(
      <AccountsRowActions
        account={baseAccount}
        actions={buildActions()}
        pendingCheckinId={42}
      />
    )

    await openRowMenu()
    const checkinItem = screen.getByRole('menuitem', { name: /check-in/ })
    expect(checkinItem).toHaveAttribute('aria-disabled', 'true')
    expect(within(checkinItem).getByRole('status')).toBeInTheDocument()
    expect(
      screen.getByRole('menuitem', { name: 'View details' })
    ).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('keeps pin and check-in pending ids independent (no cross-talk)', async () => {
    // Pin pending → the check-in item stays enabled (and vice versa).
    const { unmount } = render(
      <AccountsRowActions
        account={baseAccount}
        actions={buildActions()}
        pendingPinId={42}
      />
    )
    await openRowMenu()
    expect(
      screen.getByRole('menuitem', { name: /check-in/ })
    ).not.toHaveAttribute('aria-disabled', 'true')
    unmount()

    render(
      <AccountsRowActions
        account={baseAccount}
        actions={buildActions()}
        pendingCheckinId={42}
      />
    )
    await openRowMenu()
    expect(screen.getByRole('menuitem', { name: /Pin/ })).not.toHaveAttribute(
      'aria-disabled',
      'true'
    )
  })

  it('keeps every toggle enabled for rows that are not pending', async () => {
    render(
      <AccountsRowActions
        account={baseAccount}
        actions={buildActions()}
        pendingPinId={999}
        pendingCheckinId={999}
      />
    )

    await openRowMenu()
    expect(screen.getByRole('menuitem', { name: /Pin/ })).not.toHaveAttribute(
      'aria-disabled',
      'true'
    )
    expect(
      screen.getByRole('menuitem', { name: /check-in/ })
    ).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('does not fire the pin toggle when the pending item is clicked', async () => {
    const actions = buildActions()
    render(
      <AccountsRowActions
        account={baseAccount}
        actions={actions}
        pendingPinId={42}
      />
    )

    await openRowMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: /Pin/ }))
    expect(actions.onTogglePin).not.toHaveBeenCalled()
  })

  it('fires the pin toggle when the enabled item is clicked', async () => {
    const actions = buildActions()
    render(<AccountsRowActions account={baseAccount} actions={actions} />)

    await openRowMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Pin' }))
    expect(actions.onTogglePin).toHaveBeenCalledTimes(1)
    expect(actions.onTogglePin).toHaveBeenCalledWith(baseAccount)
  })
})
