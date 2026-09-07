import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
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

import { accountSchema } from '@/features/accounts/types'
import i18n from '@/i18n/config'

import { RoutesPage } from '../routes-page'

const boundary = vi.hoisted(() => ({
  accounts: vi.fn(),
  sites: vi.fn(),
  candidates: vi.fn(),
  createRoute: vi.fn(),
  batchAddChannels: vi.fn(),
  rebuild: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    getRoutesSummary: vi.fn().mockResolvedValue([]),
    getModelTokenCandidates: boundary.candidates,
    getChannels: vi.fn().mockResolvedValue({ items: [] }),
    getAccountsSnapshot: boundary.accounts,
    getSites: boundary.sites,
    addRoute: boundary.createRoute,
    batchAddChannels: boundary.batchAddChannels,
    rebuildRoutes: boundary.rebuild,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), info: vi.fn(), error: vi.fn(), warning: vi.fn() },
}))

const activeAccount = accountSchema.parse({
  id: 7,
  siteId: 3,
  username: 'relay-only-account',
  status: 'active',
  credentialMode: 'apikey',
  apiTokenMasked: 'masked-relay-key',
  accessTokenMasked: '',
  capabilities: { proxyOnly: true },
  runtimeHealth: { state: 'unknown' },
})
const activeSite = {
  id: 3,
  name: 'Relay-only site',
  url: 'https://relay.example',
  platform: 'openai',
  status: 'active',
}
const clients: QueryClient[] = []

async function openCreateDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute({ component: Outlet })
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: Outlet,
  })
  const route = createRoute({
    getParentRoute: () => authenticated,
    path: 'token-routes',
    component: RoutesPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([route])]),
    history: createMemoryHistory({ initialEntries: ['/token-routes'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('heading', { name: 'Route management' })
  fireEvent.click(screen.getAllByRole('button', { name: 'Add route' })[0])
  return within(await screen.findByRole('dialog', { name: 'Add route' }))
}

beforeAll(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn((query: string) => ({
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
  await i18n.changeLanguage('en')
  window.localStorage.clear()
  boundary.accounts
    .mockReset()
    .mockResolvedValue({ accounts: [activeAccount], sites: [activeSite] })
  boundary.sites.mockReset().mockResolvedValue([activeSite])
  boundary.candidates.mockReset().mockResolvedValue({ models: {} })
  boundary.createRoute.mockReset().mockResolvedValue({ id: 42 })
  boundary.batchAddChannels
    .mockReset()
    .mockResolvedValue({ success: true, created: 1, errors: [] })
  boundary.rebuild.mockReset()
})
afterEach(() => {
  cleanup()
  for (const client of clients.splice(0)) client.clear()
})

describe('RoutesPage manual account binding from the account snapshot', () => {
  it('binds an active API-key-only account when discovery is empty, from page entry through select-all and submit', async () => {
    const dialog = await openCreateDialog()

    const account = await dialog.findByRole('checkbox', {
      name: 'relay-only-account @ Relay-only site',
    })
    expect(account).toBeEnabled()
    fireEvent.change(dialog.getByLabelText('Model match rule'), {
      target: { value: 'manual-only-model' },
    })
    fireEvent.click(
      dialog.getByRole('checkbox', { name: 'Select all accounts' })
    )
    expect(account).toBeChecked()
    fireEvent.click(dialog.getByRole('button', { name: 'Add route' }))

    await waitFor(() =>
      expect(boundary.batchAddChannels).toHaveBeenCalledWith(42, [
        { accountId: 7 },
      ])
    )
    expect(boundary.createRoute).toHaveBeenCalledWith(
      expect.objectContaining({
        routeMode: 'pattern',
        modelPattern: 'manual-only-model',
      })
    )
    expect(boundary.accounts).toHaveBeenCalledTimes(1)
    expect(boundary.rebuild).not.toHaveBeenCalled()
  })

  it('ignores model-candidate ghosts and excludes inactive or credential-less accounts from select-all', async () => {
    const inactive = {
      ...activeAccount,
      id: 8,
      username: 'disabled-account',
      status: 'disabled',
    }
    const expired = {
      ...activeAccount,
      id: 9,
      username: 'expired-account',
      status: 'expired',
    }
    const noCredentials = {
      ...activeAccount,
      id: 10,
      username: 'no-credentials',
      apiTokenMasked: '',
    }
    const managementOnly = {
      ...noCredentials,
      id: 11,
      username: 'management-session-only',
      credentialMode: 'session',
      accessTokenMasked: 'masked-management-session',
    }
    const disabledSite = {
      ...activeSite,
      id: 4,
      name: 'Disabled site',
      status: 'disabled',
    }
    const disabledSiteAccount = {
      ...activeAccount,
      id: 12,
      siteId: 4,
      username: 'disabled-site-account',
    }
    const oauthAccount = {
      ...activeAccount,
      id: 13,
      username: 'oauth-account',
      credentialMode: 'session',
      apiTokenMasked: '',
      accessTokenMasked: 'masked-oauth-access',
      oauthProvider: 'openai',
    }
    boundary.accounts.mockResolvedValue({
      accounts: [
        activeAccount,
        inactive,
        expired,
        noCredentials,
        managementOnly,
        disabledSiteAccount,
        oauthAccount,
      ],
      sites: [activeSite, disabledSite],
    })
    boundary.sites.mockResolvedValue([activeSite, disabledSite])
    boundary.candidates.mockResolvedValue({
      models: {
        'discovered-model': [
          {
            accountId: 99,
            username: 'candidate-ghost',
            siteName: 'Stale discovery',
          },
        ],
      },
    })
    const dialog = await openCreateDialog()
    await dialog.findByRole('checkbox', {
      name: 'relay-only-account @ Relay-only site',
    })
    expect(
      dialog.getByRole('checkbox', { name: 'oauth-account @ Relay-only site' })
    ).toBeEnabled()
    for (const name of [
      'disabled-account',
      'expired-account',
      'no-credentials',
      'management-session-only',
      'disabled-site-account',
      'candidate-ghost',
    ]) {
      expect(
        dialog.queryByRole('checkbox', { name: new RegExp(name) })
      ).not.toBeInTheDocument()
    }
    fireEvent.change(dialog.getByLabelText('Model match rule'), {
      target: { value: 'manual-only-model' },
    })
    fireEvent.click(
      dialog.getByRole('checkbox', { name: 'Select all accounts' })
    )
    expect(dialog.getByRole('status')).toHaveTextContent('2 / 2 selected')
    fireEvent.click(dialog.getByRole('button', { name: 'Add route' }))

    await waitFor(() =>
      expect(boundary.batchAddChannels).toHaveBeenCalledWith(42, [
        { accountId: 13 },
        { accountId: 7 },
      ])
    )
  })

  it('keeps account snapshot bindings available even when the model-candidates query fails', async () => {
    boundary.candidates.mockRejectedValue(
      new Error('model discovery unavailable')
    )
    const dialog = await openCreateDialog()

    const account = await dialog.findByRole('checkbox', {
      name: 'relay-only-account @ Relay-only site',
    })
    fireEvent.change(dialog.getByLabelText('Model match rule'), {
      target: { value: 'not-discovered' },
    })
    fireEvent.click(
      dialog.getByRole('checkbox', { name: 'Select all accounts' })
    )
    expect(account).toBeChecked()
    fireEvent.click(dialog.getByRole('button', { name: 'Add route' }))

    await waitFor(() =>
      expect(boundary.batchAddChannels).toHaveBeenCalledWith(42, [
        { accountId: 7 },
      ])
    )
    expect(boundary.rebuild).not.toHaveBeenCalled()
  })

  it('preserves the site label for an unnamed API-key account from the snapshot', async () => {
    boundary.accounts.mockResolvedValue({
      accounts: [{ ...activeAccount, username: null }],
      sites: [activeSite],
    })
    const dialog = await openCreateDialog()

    expect(
      await dialog.findByRole('checkbox', {
        name: 'account-7 @ Relay-only site',
      })
    ).toBeEnabled()
  })

  it('does not offer discovery as a prerequisite when no account has relay credentials', async () => {
    boundary.accounts.mockResolvedValue({
      accounts: [{ ...activeAccount, apiTokenMasked: '' }],
      sites: [activeSite],
    })
    const dialog = await openCreateDialog()

    expect(
      await dialog.findByText(/No eligible accounts are loaded/)
    ).toBeInTheDocument()
    expect(
      dialog.queryByRole('button', { name: 'Auto-rebuild' })
    ).not.toBeInTheDocument()
    expect(dialog.queryByRole('checkbox')).not.toBeInTheDocument()
  })
})
