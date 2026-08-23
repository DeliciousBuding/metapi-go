// Behavior tests for the inline check-in toggle badge in the accounts list
// (Wave 7 L4 finding: the check-in badge looked clickable but was a static
// <Badge> — dead click). The cell must now render a real button that keeps
// the badge look, calls actions.onToggleCheckin with the account, shows a
// per-row spinner while the mutation is pending, and stays a plain muted
// "Not supported" text when the account cannot check in.
import '@testing-library/jest-dom/vitest'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
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

import i18n from '@/i18n/config'

import type { Account, AccountRowActions } from '../../types'
import { useAccountsColumns } from '../accounts-columns'

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 42,
    siteId: 1,
    username: 'smoke-account',
    status: 'active',
    isPinned: false,
    checkinEnabled: true,
    credentialMode: 'session',
    capabilities: {
      canCheckin: true,
      canRefreshBalance: false,
      proxyOnly: false,
    },
    ...overrides,
  } as Account
}

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

function Harness({
  account,
  actions,
  pendingCheckinId = null,
}: {
  account: Account
  actions: AccountRowActions
  pendingCheckinId?: number | null
}) {
  const columns = useAccountsColumns(actions, null, pendingCheckinId)
  const table = useReactTable({
    data: [account],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  const row = table.getRowModel().rows[0]
  return (
    <div>
      {row
        .getVisibleCells()
        .map((cell) =>
          flexRender(cell.column.columnDef.cell, cell.getContext())
        )}
    </div>
  )
}

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

beforeEach(async () => {
  // i18next resolves the zh variant to the registered 'zhCN' language id
  // (see web/src/i18n/languages.ts convertDetectedLanguage).
  await i18n.changeLanguage('zhCN')
})

afterEach(() => cleanup())

describe('accounts check-in badge toggle', () => {
  it('renders a real button labeled with the on state and the turn-off action', async () => {
    render(
      <Harness
        account={makeAccount({ checkinEnabled: true })}
        actions={buildActions()}
      />
    )
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: '关闭签到' })
      ).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: '关闭签到' })).toHaveTextContent(
      '已开启'
    )
  })

  it('renders the turn-on action label when check-in is off', async () => {
    render(
      <Harness
        account={makeAccount({ checkinEnabled: false })}
        actions={buildActions()}
      />
    )
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: '开启签到' })
      ).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: '开启签到' })).toHaveTextContent(
      '未开启'
    )
  })

  it('clicking the badge calls onToggleCheckin with the account', async () => {
    const actions = buildActions()
    const account = makeAccount({ checkinEnabled: true })
    render(<Harness account={account} actions={actions} />)
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: '关闭签到' })
      ).toBeInTheDocument()
    })
    fireEvent.click(screen.getByRole('button', { name: '关闭签到' }))
    expect(actions.onToggleCheckin).toHaveBeenCalledTimes(1)
    expect(actions.onToggleCheckin).toHaveBeenCalledWith(account)
  })

  it('disables only the pending row badge (per-row spinner contract)', async () => {
    const account = makeAccount({ checkinEnabled: false })
    render(
      <Harness
        account={account}
        actions={buildActions()}
        pendingCheckinId={42}
      />
    )
    await waitFor(() => {
      expect(screen.getByRole('button', { name: '开启签到' })).toBeDisabled()
    })
  })

  it('keeps the plain muted "unsupported" text (no button) when canCheckin is false', async () => {
    render(
      <Harness
        account={makeAccount({
          capabilities: {
            canCheckin: false,
            canRefreshBalance: false,
            proxyOnly: false,
          },
        })}
        actions={buildActions()}
      />
    )
    await waitFor(() => {
      expect(screen.getByText('不支持')).toBeInTheDocument()
    })
    // No check-in toggle button (the harness also renders the row-actions
    // Power/⋮ buttons, so scope the negative assertion to check-in labels).
    expect(
      screen.queryByRole('button', { name: '关闭签到' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: '开启签到' })
    ).not.toBeInTheDocument()
  })

  it('localizes the credential mode badge from the i18n resources (en)', async () => {
    await i18n.changeLanguage('en')
    render(
      <Harness
        account={makeAccount({ credentialMode: 'apikey' })}
        actions={buildActions()}
      />
    )
    await waitFor(() => {
      expect(
        screen.getByText(i18n.t('accounts.columns.credentialModeApiKey'))
      ).toBeInTheDocument()
    })
  })
})
