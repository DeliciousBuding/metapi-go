import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { useCallback, useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ReauthProvider } from '@/components/common/reauth-provider'
import { DataTablePage, useDataTable } from '@/components/data-table'
import '@/i18n/config'
import { api } from '@/lib/api'

import type { Account, AccountRowActions } from '../../types'
import { AccountInlineDetails } from '../account-inline-details'
import { useAccountsColumns } from '../accounts-columns'

const navigateMock = vi.hoisted(() => vi.fn())

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
  }
})

const accountA: Account = {
  id: 101,
  siteId: 10,
  username: 'alice-relay',
  status: 'active',
  accessToken: '',
  apiToken: null,
  accessTokenMasked: '',
  apiTokenMasked: '',
  balance: 42.5,
  balanceUsed: 12.25,
  quota: 100,
  unitCost: null,
  valueScore: 0,
  isPinned: false,
  sortOrder: 0,
  checkinEnabled: true,
  lastCheckinAt: '2026-11-26T08:00:00Z',
  lastBalanceRefresh: '2026-11-26T09:00:00Z',
  credentialMode: 'session',
  capabilities: {
    canCheckin: true,
    canRefreshBalance: true,
    proxyOnly: false,
  },
  runtimeHealth: { state: 'healthy', reason: '' },
  site: {
    id: 10,
    name: 'Relay Hub',
    url: 'https://relay.example.com',
    platform: 'new-api',
    status: 'active',
  },
  todayReward: 1.5,
  todayRewardStatus: '',
  todaySpend: 0.75,
  todaySpendStatus: '',
  tags: ['prod'],
  extraConfig: null,
  createdAt: '',
  updatedAt: '',
}

const accountB: Account = {
  ...accountA,
  id: 102,
  username: 'bob-key',
  credentialMode: 'apikey',
  todayReward: 0,
  todaySpend: 0,
}

const actions: AccountRowActions = {
  onEdit: vi.fn(),
  onDelete: vi.fn(),
  onRefresh: vi.fn(),
  onViewDetail: vi.fn(),
  onTogglePin: vi.fn(),
  onToggleCheckin: vi.fn(),
  onTriggerCheckin: vi.fn(),
  onToggleStatus: vi.fn(),
}

let mobile = false

beforeEach(() => {
  navigateMock.mockReset()
  vi.spyOn(api, 'getAccountTokens').mockResolvedValue([
    {
      id: 501,
      accountId: 101,
      name: 'default-token',
      token: '',
      tokenMasked: 'sk-****abcd',
      tokenGroup: 'default',
      valueStatus: 'normal',
      source: 'synced',
      enabled: true,
      isDefault: true,
      createdAt: '',
      updatedAt: '',
    },
  ])
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn((query: string) => ({
      matches: mobile,
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
  vi.stubGlobal('ResizeObserver', ResizeObserverStub)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  mobile = false
})

function Harness() {
  const [expanded, setExpanded] = useState<ReadonlySet<number>>(() => new Set())
  const toggle = useCallback(
    (id: number) =>
      setExpanded((prev) => {
        const next = new Set(prev)
        if (next.has(id)) next.delete(id)
        else next.add(id)
        return next
      }),
    []
  )
  const contentId = useCallback((id: number) => `account-details-${id}`, [])
  const columns = useAccountsColumns(
    actions,
    null,
    null,
    null,
    null,
    undefined,
    {
      expandedIds: expanded,
      toggle,
      getContentId: contentId,
    }
  )
  const { table } = useDataTable({
    data: [accountA, accountB],
    columns,
    getRowId: (row) => String(row.id),
    enableRowSelection: true,
  })
  return (
    <DataTablePage
      table={table}
      rowDisclosure={{
        isExpanded: (row) => expanded.has(row.original.id),
        onToggle: (row) => toggle(row.original.id),
        getContentId: (row) => contentId(row.original.id),
        getLabel: (row) => row.original.username ?? `#${row.original.id}`,
        renderContent: (row) => (
          <AccountInlineDetails
            account={row.original}
            onViewDetail={actions.onViewDetail}
            onRefresh={actions.onRefresh}
          />
        ),
      }}
    />
  )
}

function renderHarness() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ReauthProvider>
        <Harness />
      </ReauthProvider>
    </QueryClientProvider>
  )
}

describe('accounts inline row disclosure', () => {
  it('lazily loads tokens only when a row expands and navigates to routes with account+token context', async () => {
    renderHarness()
    expect(api.getAccountTokens).not.toHaveBeenCalled()

    // Balance cell displays both today reward and today spend concisely.
    expect(screen.getByText('+$1.50')).toBeInTheDocument()
    expect(screen.getByText('-$0.75')).toBeInTheDocument()

    const trigger = screen.getByRole('button', { name: 'alice-relay' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(trigger).toHaveAttribute('aria-controls', 'account-details-101')

    fireEvent.click(trigger)
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'alice-relay' })
      ).toHaveAttribute('aria-expanded', 'true')
    )

    const region = await screen.findByRole('region', { name: 'alice-relay' })
    expect(await within(region).findByText('default-token')).toBeInTheDocument()
    expect(api.getAccountTokens).toHaveBeenCalledTimes(1)

    const tokenList = within(region).getByRole('list')
    fireEvent.click(
      within(tokenList).getByRole('button', { name: 'Use this key in routes' })
    )
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/token-routes',
      search: { accountId: 101, tokenId: 501, siteId: 10 },
    })
  })

  it('allows multiple rows expanded at once and surfaces retry when token loading fails', async () => {
    vi.mocked(api.getAccountTokens).mockRejectedValueOnce(
      new Error('tokens offline')
    )
    renderHarness()

    fireEvent.click(screen.getByRole('button', { name: 'alice-relay' }))
    const regionA = await screen.findByRole('region', { name: 'alice-relay' })
    expect(await within(regionA).findByRole('alert')).toHaveTextContent(
      /Could not load API keys/
    )

    fireEvent.click(screen.getByRole('button', { name: 'bob-key' }))
    expect(screen.getAllByRole('region')).toHaveLength(2)

    fireEvent.click(within(regionA).getByRole('button', { name: 'Retry' }))
    await waitFor(() =>
      expect(within(regionA).getByText('default-token')).toBeInTheDocument()
    )
  })
})
