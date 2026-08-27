// Regression coverage for GitHub #1008: Accounts pagination is URL-controlled.
// The accounts page uses a fresh URL-derived filter array on every render; the
// table must not let TanStack reset an explicit page click back to page 1.
import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
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

import { AccountsPage } from '../accounts-page'

type RouterLocation = {
  pathname: string
  searchStr: string
}

const routerState = vi.hoisted(() => {
  let location: RouterLocation = { pathname: '/accounts', searchStr: '' }
  const listeners = new Set<() => void>()

  const commitHref = (href: string) => {
    const url = new URL(href, window.location.origin)
    location = { pathname: url.pathname, searchStr: url.search }
    window.history.replaceState({}, '', `${url.pathname}${url.search}`)
    for (const listener of listeners) listener()
  }

  return {
    getLocation: () => location,
    subscribe: (listener: () => void) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    navigate: vi.fn((options: { href?: string }) => {
      if (options.href) commitHref(options.href)
    }),
    reset: (href: string) => commitHref(href),
  }
})

const testState = vi.hoisted(() => ({
  accounts: [] as Array<Record<string, unknown>>,
  columns: [{ accessorKey: 'username', header: 'Username' }],
}))

vi.mock('@tanstack/react-router', async () => {
  const React = await import('react')

  return {
    useLocation: <T,>({
      select,
    }: {
      select: (location: RouterLocation) => T
    }) =>
      React.useSyncExternalStore(
        routerState.subscribe,
        () => select(routerState.getLocation()),
        () => select(routerState.getLocation())
      ),
    useNavigate: () => routerState.navigate,
    useSearch: () => ({}),
  }
})

vi.mock('@/features/import', () => ({
  ImportWizardDialog: () => null,
}))

vi.mock('../../api', () => ({
  useAccounts: () => ({
    data: {
      accounts: testState.accounts,
      sites: [
        {
          id: 1,
          name: 'Site 01',
          url: 'https://site.example.com',
          platform: 'newapi',
          status: 'active',
        },
      ],
      generatedAt: '',
    },
    error: null,
    isLoading: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
  useDeleteAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRefreshAccount: () => ({ mutate: vi.fn(), isPending: false }),
  useToggleAccountPin: () => ({
    mutate: vi.fn(),
    isPending: false,
    variables: undefined,
  }),
  useToggleAccountStatus: () => ({
    mutate: vi.fn(),
    isPending: false,
    variables: undefined,
  }),
  useToggleAccountCheckin: () => ({
    mutate: vi.fn(),
    isPending: false,
    variables: undefined,
  }),
  useBatchUpdateAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../accounts-columns', () => ({
  useAccountsColumns: () => testState.columns,
}))

vi.mock('../account-created-toast', () => ({
  showAccountCreatedToast: vi.fn(),
  showAccountLoginToast: vi.fn(),
}))

vi.mock('../account-detail-sheet', () => ({
  AccountDetailSheet: () => null,
}))

vi.mock('../account-form-dialog', () => ({
  AccountFormDialog: () => null,
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
})

beforeEach(() => {
  localStorage.clear()
  routerState.navigate.mockClear()
  routerState.reset('/accounts?pageSize=10')
  testState.accounts = Array.from({ length: 25 }, (_, index) => ({
    id: index + 1,
    siteId: 1,
    username: `Account ${String(index + 1).padStart(2, '0')}`,
    status: 'active',
    credentialMode: 'session',
    site: {
      id: 1,
      name: 'Site 01',
      url: 'https://site.example.com',
      platform: 'newapi',
      status: 'active',
    },
  }))
})

afterEach(() => cleanup())

describe('AccountsPage URL-controlled pagination', () => {
  it('keeps page 2 selected and renders rows 11-20', async () => {
    render(<AccountsPage />)

    expect(screen.getByText('Account 01')).toBeInTheDocument()
    expect(screen.getByText('Account 10')).toBeInTheDocument()
    expect(screen.queryByText('Account 11')).not.toBeInTheDocument()

    // TanStack arms auto-reset after the initial render. A real user click
    // necessarily happens after that microtask; flush it so the regression is
    // observable under jsdom too.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0))
    })

    fireEvent.click(screen.getByRole('button', { name: /Go to page 2/ }))

    // Flush the queued reset before asserting the URL and rendered row model.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20))
    })

    const search = new URLSearchParams(window.location.search)
    expect(search.get('page')).toBe('2')
    expect(search.get('pageSize')).toBe('10')
    for (let index = 11; index <= 20; index += 1) {
      expect(
        screen.getByText(`Account ${String(index).padStart(2, '0')}`)
      ).toBeInTheDocument()
    }
    for (let index = 1; index <= 10; index += 1) {
      expect(
        screen.queryByText(`Account ${String(index).padStart(2, '0')}`)
      ).not.toBeInTheDocument()
    }
  })
})
