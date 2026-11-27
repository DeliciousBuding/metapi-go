import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ReauthProvider } from '@/components/common/reauth-provider'
import i18n from '@/i18n/config'
import { request } from '@/lib/api/transport'

import { BudgetHolds } from '../budget-holds'
import { BudgetEditor } from '../editors'
import { ReadinessDialog } from '../readiness'
import { userAdminSearchSchema } from '../search'
import { UserAdministrationPage } from '../user-administration-page'

vi.mock('@/lib/api/transport', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/transport')>()),
  request: vi.fn(),
}))

const network = vi.mocked(request)

const activeUser = {
  id: 1,
  username: 'alice',
  displayName: 'Alice',
  groupId: 2,
  enabled: true,
  createdAt: '2026-10-21T00:00:00Z',
  updatedAt: '2026-10-21T00:00:00Z',
}

const disabledUser = {
  id: 2,
  username: 'bob',
  displayName: 'Bob',
  groupId: 3,
  enabled: false,
  createdAt: '2026-10-21T00:00:00Z',
  updatedAt: '2026-10-21T00:00:00Z',
}

const activeGroup = {
  id: 2,
  slug: 'default',
  name: 'Default',
  enabled: true,
  createdAt: activeUser.createdAt,
  updatedAt: activeUser.updatedAt,
}

const disabledGroup = {
  id: 3,
  slug: 'suspended',
  name: 'Suspended',
  enabled: false,
  createdAt: activeUser.createdAt,
  updatedAt: activeUser.updatedAt,
}

function mount(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ReauthProvider>{ui}</ReauthProvider>
    </QueryClientProvider>
  )
}

beforeEach(async () => {
  await i18n.changeLanguage('en')
  network.mockReset()
  network.mockImplementation(async (path) => {
    if (path === '/api/admin/users')
      return { items: [activeUser, disabledUser], limit: 1000 }
    if (path === '/api/admin/user-groups')
      return { items: [activeGroup, disabledGroup], limit: 1000 }
    if (path === '/api/admin/user-groups/2/routes')
      return {
        items: [{ groupId: 2, routeId: 15, createdAt: activeUser.createdAt }],
      }
    if (path === '/api/routes/summary')
      return [
        {
          id: 15,
          modelPattern: 'example-*',
          displayName: 'Example',
          enabled: true,
        },
      ]
    if (path === '/api/admin/users/1/budget')
      return {
        limit: 10,
        used: 3,
        reserved: 2,
        unlimited: false,
        frozen: false,
        unknownRequests: 0,
      }
    if (path.includes('/budget/holds'))
      return {
        items: [
          {
            requestId: 'req_pending',
            state: 'pending',
            reserved: 0.25,
            totalTokens: null,
            createdAt: activeUser.createdAt,
            multiplier: 1,
          },
          {
            requestId: 'req_unknown',
            state: 'unknown',
            reserved: 0.5,
            totalTokens: null,
            createdAt: activeUser.createdAt,
            multiplier: 2,
          },
        ],
        limit: 100,
        offset: 0,
      }
    throw new Error(`Unexpected path ${path}`)
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('user administration SoftToneBadge & quiet refresh', () => {
  it('renders dual-channel status & group badges and QuietRefreshStatus on the directory page', async () => {
    const root = createRootRoute()
    const authenticated = createRoute({
      getParentRoute: () => root,
      id: '_authenticated',
    })
    const page = createRoute({
      getParentRoute: () => authenticated,
      path: '/users',
      validateSearch: userAdminSearchSchema,
      component: UserAdministrationPage,
    })
    const history = createMemoryHistory({ initialEntries: ['/users'] })
    const router = createRouter({
      routeTree: root.addChildren([authenticated.addChildren([page])]),
      history,
    })

    const { container } = mount(<RouterProvider router={router} />)

    await screen.findByRole('heading', { name: 'Alice' })
    expect(screen.getByTestId('quiet-refresh-status')).toBeInTheDocument()
    expect(
      container.querySelector('[data-tone="success"][data-tone]')
    ).not.toBeNull()
    expect(
      container.querySelector('[data-tone-marker="success"]')
    ).not.toBeNull()
    expect(
      container.querySelector('[data-tone-marker="neutral"]')
    ).not.toBeNull()
    expect(container.querySelector('[data-tone="info"]')).toHaveTextContent(
      'Default'
    )
    expect(container.querySelector('[data-tone="warning"]')).toHaveTextContent(
      'Suspended'
    )
  })

  it('renders SoftToneBadge health tones in BudgetEditor and BudgetHolds', async () => {
    const { container, rerender } = mount(
      <BudgetEditor
        userId={1}
        budget={{
          limit: 10,
          used: 3,
          reserved: 2,
          unlimited: false,
          frozen: false,
          unknownRequests: 0,
        }}
      />
    )
    expect(container.querySelector('[data-tone="success"]')).not.toBeNull()

    rerender(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <ReauthProvider>
          <BudgetEditor
            userId={1}
            budget={{
              limit: 10,
              used: 3,
              reserved: 2,
              unlimited: false,
              frozen: true,
              unknownRequests: 1,
            }}
          />
        </ReauthProvider>
      </QueryClientProvider>
    )
    expect(container.querySelector('[data-tone="destructive"]')).not.toBeNull()

    const holds = mount(<BudgetHolds userId={1} />)
    await screen.findByText('req_pending')
    expect(holds.container.querySelector('[data-tone="info"]')).not.toBeNull()
    expect(
      holds.container.querySelector('[data-tone="warning"]')
    ).not.toBeNull()
  })

  it('renders SoftToneBadge states and QuietRefreshStatus inside ReadinessDialog', async () => {
    mount(
      <ReadinessDialog
        userId={1}
        close={vi.fn()}
        editUser={vi.fn()}
        editGroup={vi.fn()}
        editAccess={vi.fn()}
        editBudget={vi.fn()}
      />
    )

    await screen.findByText('Configuration checks passed')
    expect(
      document.querySelectorAll('[data-tone="success"]').length
    ).toBeGreaterThanOrEqual(4)
    expect(screen.getByTestId('quiet-refresh-status')).toBeInTheDocument()
  })
})
