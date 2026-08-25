// Regression coverage for GitHub #996: Sites pagination is URL-controlled.
// The test renders the real shared data-table and asserts both the committed
// search string and the visible page rows after selecting page 2.
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

import { SitesPage } from '../sites-page'

type RouterLocation = {
  pathname: string
  searchStr: string
}

const routerState = vi.hoisted(() => {
  let location: RouterLocation = { pathname: '/sites', searchStr: '' }
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
  sites: [] as Array<Record<string, unknown>>,
  columns: [
    { accessorKey: 'name', header: 'Name' },
    { accessorKey: 'status', header: 'Status' },
  ],
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

vi.mock('@/features/accounts', () => ({
  useAccounts: () => ({ data: { accounts: [] }, isLoading: false }),
}))

vi.mock('@/features/import', () => ({
  ImportWizardDialog: () => null,
}))

vi.mock('../../api', () => ({
  useSites: () => ({
    data: testState.sites,
    error: null,
    isLoading: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
  useDeleteSite: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSite: () => ({
    mutate: vi.fn(),
    isPending: false,
    variables: undefined,
  }),
  useBatchUpdateSites: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../site-created-modal', () => ({
  SiteCreatedModal: () => null,
}))

vi.mock('../site-detail-sheet', () => ({
  SiteDetailSheet: () => null,
}))

vi.mock('../site-form-dialog', () => ({
  SiteFormDialog: () => null,
}))

vi.mock('../sites-columns', () => ({
  SITES_STATUS_FILTER_OPTIONS: [],
  useSitesColumns: () => testState.columns,
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
  routerState.reset('/sites?pageSize=10')
  testState.sites = Array.from({ length: 25 }, (_, index) => ({
    id: index + 1,
    name: `Site ${String(index + 1).padStart(2, '0')}`,
    url: `https://site-${index + 1}.example.com`,
    status: 'active',
  }))
})

afterEach(() => cleanup())

describe('SitesPage URL-controlled pagination', () => {
  it('keeps page 2 selected and renders rows 11-20', async () => {
    render(<SitesPage />)

    expect(screen.getByText('Site 01')).toBeInTheDocument()
    expect(screen.getByText('Site 10')).toBeInTheDocument()
    expect(screen.queryByText('Site 11')).not.toBeInTheDocument()

    // TanStack arms auto-reset after the initial render. A real user click
    // necessarily happens after that microtask; flush it so the regression is
    // observable under jsdom too.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0))
    })

    fireEvent.click(screen.getByRole('button', { name: /Go to page 2/ }))

    // TanStack schedules automatic page-index resets in a microtask. Flush the
    // queued reset before asserting the stable URL and rendered row model.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20))
    })

    const search = new URLSearchParams(window.location.search)
    expect(search.get('page')).toBe('1')
    expect(search.get('pageSize')).toBe('10')
    for (let index = 11; index <= 20; index += 1) {
      expect(
        screen.getByText(`Site ${String(index).padStart(2, '0')}`)
      ).toBeInTheDocument()
    }
    for (let index = 1; index <= 10; index += 1) {
      expect(
        screen.queryByText(`Site ${String(index).padStart(2, '0')}`)
      ).not.toBeInTheDocument()
    }
  })
})
