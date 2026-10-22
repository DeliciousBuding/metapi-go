import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createRootRoute,
  createRoute,
  createRouter,
  createMemoryHistory,
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
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ReauthProvider } from '@/components/common/reauth-provider'
import i18n from '@/i18n/config'
import { gatewayUsersApi } from '@/lib/api/gateway-users'
import { request, type RequestOptions } from '@/lib/api/transport'
import { useAuthStore } from '@/stores/auth-store'

import { BudgetHolds } from '../budget-holds'
import {
  ActivationDialog,
  BudgetEditor,
  IdentityEditor,
  PricingEditor,
} from '../editors'
import { GroupAccessDialog } from '../group-access'
import { userAdminSearchSchema } from '../search'
import { UserAdministrationPage } from '../user-administration-page'

vi.mock('@/lib/api/transport', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/transport')>()),
  request: vi.fn(),
}))

const user = {
  id: 1,
  username: 'alice',
  displayName: 'Alice',
  groupId: 2,
  enabled: true,
  createdAt: '2026-10-21T00:00:00Z',
  updatedAt: '2026-10-21T00:00:00Z',
}
const group = {
  id: 2,
  slug: 'default',
  name: 'Default',
  enabled: true,
  createdAt: user.createdAt,
  updatedAt: user.updatedAt,
}
const secondGroup = { ...group, id: 3, slug: 'premium', name: 'Premium' }
const budget = {
  limit: 0,
  used: 0,
  reserved: 0,
  unlimited: false,
  frozen: false,
  unknownRequests: 0,
}
const network = vi.mocked(request)
const writes: Array<{
  path: string
  options: RequestOptions
  body: Record<string, unknown> | undefined
}> = []

function mount(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const view = render(
    <QueryClientProvider client={client}>
      <ReauthProvider>{ui}</ReauthProvider>
    </QueryClientProvider>
  )
  return { ...view, client }
}

beforeEach(async () => {
  await i18n.changeLanguage('en')
  writes.length = 0
  network.mockReset()
  network.mockImplementation(async (path, options = {}) => {
    const body = options.body
      ? (JSON.parse(options.body) as Record<string, unknown>)
      : undefined
    if (options.method && options.method !== 'GET') {
      writes.push({ path, options, body })
      if (path === '/api/admin/users' && options.method === 'POST')
        return { ...user, ...body, groupId: body?.groupId || group.id }
      if (path === '/api/admin/user-groups' && options.method === 'POST')
        return { ...group, ...body }
      if (path.endsWith('/budget') || path.endsWith('/reconcile'))
        return { ...budget, ...body }
      if (path.endsWith('/pricing')) return body
      if (path.endsWith('/activation'))
        return {
          code: 'synthetic-one-time-code',
          expiresAt: '2026-10-22T00:00:00Z',
        }
      return { success: true }
    }
    if (path === '/api/admin/users') return { items: [user], limit: 1000 }
    if (path === '/api/admin/user-groups')
      return { items: [group, secondGroup], limit: 1000 }
    if (path.endsWith('/budget')) return budget
    if (path.includes('/budget/holds'))
      return {
        items: [
          {
            requestId: 'req_synthetic',
            state: 'unknown',
            reserved: 0.5,
            totalTokens: null,
            createdAt: user.createdAt,
            multiplier: 2,
          },
        ],
        limit: 100,
        offset: 0,
      }
    if (path.endsWith('/pricing')) return { multiplier: 1 }
    if (path.endsWith('/routes')) return { items: [] }
    if (path === '/api/routes/summary')
      return [
        {
          id: 15,
          modelPattern: 'example-*',
          displayName: 'Published composite',
          enabled: true,
          routeMode: 'composite',
          sourceRouteIds: [7, 8],
        },
      ]
    throw new Error(`Unexpected test request ${path}`)
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('administrator user workflows', () => {
  it('creates a real user with an explicit finite default group selector and no invented credentials', async () => {
    const close = vi.fn()
    mount(<IdentityEditor kind='user' groups={[group]} close={close} />)
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'new-user' },
    })
    fireEvent.change(screen.getByLabelText('Display name'), {
      target: { value: 'New user' },
    })
    expect(screen.getByText(/default group grants no routes/i)).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
    expect(writes[0]?.body).toEqual({
      username: 'new-user',
      displayName: 'New user',
      groupId: 0,
      enabled: true,
    })
  })

  it('changes the true current group and disables the user without deleting it or changing username', async () => {
    const close = vi.fn()
    mount(
      <IdentityEditor
        kind='user'
        user={user}
        groups={[group, secondGroup]}
        close={close}
      />
    )
    expect(screen.getByLabelText('Username')).toBeDisabled()
    fireEvent.click(screen.getByRole('combobox', { name: 'Current group' }))
    const option = await screen.findByRole('option', { name: 'Premium' })
    fireEvent.pointerDown(option)
    fireEvent.click(option)
    fireEvent.click(screen.getByRole('switch', { name: 'Enabled' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
    expect(writes[0]?.body).toEqual({
      displayName: 'Alice',
      groupId: 3,
      enabled: false,
    })
    expect(writes[0]?.options.method).toBe('PUT')
  })

  it('keeps zero finite and requires an explicit unlimited switch', async () => {
    mount(<BudgetEditor userId={1} budget={budget} />)
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(writes).toHaveLength(1))
    expect(writes[0]?.body).toEqual({ limit: 0, unlimited: false })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
    )
    fireEvent.click(
      screen.getByRole('switch', { name: 'Unlimited quota (explicit)' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(writes).toHaveLength(2))
    expect(writes[1]?.body).toEqual({ limit: 0, unlimited: true })
    expect(screen.getByText(/not USD or upstream cost/i)).toBeVisible()
  })

  it('previews a limit change without resetting used or held quota or claiming a top-up', async () => {
    mount(
      <BudgetEditor
        userId={1}
        budget={{ ...budget, limit: 10, used: 3, reserved: 2 }}
      />
    )
    expect(screen.getByText('Available quota: 5 units')).toBeVisible()
    fireEvent.change(screen.getByLabelText('Total quota limit (units)'), {
      target: { value: '20' },
    })
    expect(screen.getByText('Available quota: 15 units')).toBeVisible()
    expect(
      screen.getByText(/20 limit − 3 used − 2 reserved = 15/)
    ).toBeVisible()
    expect(screen.getByText(/Changing the limit is not a top-up/)).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(writes[0]?.body).toEqual({ limit: 20, unlimited: false })
    )
    expect(screen.getByText('Available quota: 15 units')).toBeVisible()
    fireEvent.change(screen.getByLabelText('Total quota limit (units)'), {
      target: { value: '4' },
    })
    expect(screen.getByText('Available quota: -1 units')).toBeVisible()
  })

  it('previews invalid and unlimited limits without hiding a frozen quota', async () => {
    mount(
      <BudgetEditor
        userId={1}
        budget={{ ...budget, frozen: true, unknownRequests: 1 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Total quota limit (units)'), {
      target: { value: '-1' },
    })
    expect(screen.getByText(/Available quota: Unknown/)).toBeVisible()
    fireEvent.click(
      screen.getByRole('switch', { name: 'Unlimited quota (explicit)' })
    )
    expect(screen.getByText(/No quota cap after this save/)).toBeVisible()
    expect(
      screen.getByText(/Changing its cap does not clear unknown usage/)
    ).toBeVisible()
  })

  it('explains exact multiplier examples and minimum-unit rounding from the current form', () => {
    mount(<PricingEditor groupId={2} pricing={{ multiplier: 2 }} />)
    expect(
      screen.getByText(/14 reported total tokens → 0.000028/)
    ).toBeVisible()
    fireEvent.change(screen.getByLabelText('Token quota multiplier'), {
      target: { value: '0.5' },
    })
    expect(
      screen.getByText(
        /14 reported total tokens → 0.000007.*1 reported total token → 0.000001/
      )
    ).toBeVisible()
    expect(screen.getByText(/each request rounds up/)).toBeVisible()
    fireEvent.change(screen.getByLabelText('Token quota multiplier'), {
      target: { value: '0' },
    })
    expect(
      screen.getByText(
        'Enter a valid positive multiplier to preview deductions.'
      )
    ).toBeVisible()
    expect(writes).toHaveLength(0)
  })

  it('rejects invalid numeric quota values and explains held unknown usage', async () => {
    mount(
      <BudgetEditor
        userId={1}
        budget={{ ...budget, reserved: 0.2, frozen: true, unknownRequests: 1 }}
      />
    )
    expect(screen.getByText(/Quota is frozen/)).toBeVisible()
    fireEvent.change(screen.getByLabelText('Total quota limit (units)'), {
      target: { value: '-1' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText(/Enter 0–1,000,000,000/)
    expect(writes).toHaveLength(0)
    expect(screen.getByLabelText('Total quota limit (units)')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
  })

  it('shows failed writes and permits retry without reporting success', async () => {
    const normal = network.getMockImplementation()
    let fail = true
    network.mockImplementation(async (path, options) => {
      if (path.endsWith('/budget') && options?.method === 'PUT' && fail) {
        fail = false
        throw new Error('offline')
      }
      return normal?.(path, options)
    })
    mount(<BudgetEditor userId={1} budget={budget} />)
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/change was not saved/)).toBeVisible()
    expect(screen.queryByText('Saved', { exact: true })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Saved', { exact: true })).toBeVisible()
  })

  it('replays reauthenticated writes without retaining the master token in Query mutation variables', async () => {
    const normal = network.getMockImplementation()
    network.mockImplementation(async (path, options) => {
      if (
        options?.method === 'PUT' &&
        !options.headers?.['X-Admin-Confirm-Token']
      )
        throw { response: { status: 403, data: { reauthRequired: true } } }
      return normal?.(path, options)
    })
    const { client } = mount(<BudgetEditor userId={1} budget={budget} />)
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    const label = i18n.t('reauth.tokenLabel')
    fireEvent.change(await screen.findByLabelText(label), {
      target: { value: 'synthetic-master-confirm' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: i18n.t('reauth.submit') })
    )
    expect(await screen.findByText('Saved', { exact: true })).toBeVisible()
    expect(writes[0]?.options.headers).toEqual({
      'X-Admin-Confirm-Token': 'synthetic-master-confirm',
    })
    expect(
      JSON.stringify(
        client
          .getMutationCache()
          .getAll()
          .map((item) => item.state)
      )
    ).not.toContain('synthetic-master-confirm')
  })

  it('shows activation secrets only in component memory and clears them on close', async () => {
    const close = vi.fn()
    const { client, unmount } = mount(
      <ActivationDialog user={user} close={close} />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Issue one-time code' }))
    expect(
      await screen.findByLabelText('One-time activation code')
    ).toHaveValue('synthetic-one-time-code')
    expect(writes[0]?.body).toEqual({})
    expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(
      'synthetic-one-time-code'
    )
    expect(client.getMutationCache().getAll()).toHaveLength(0)
    expect(JSON.stringify(localStorage)).not.toContain(
      'synthetic-one-time-code'
    )
    expect(window.location.href).not.toContain('synthetic-one-time-code')
    fireEvent.click(
      screen.getAllByRole('button', { name: 'Close' })[0] as HTMLElement
    )
    expect(close).toHaveBeenCalledOnce()
    unmount()
    mount(<ActivationDialog user={user} close={vi.fn()} />)
    expect(
      screen.queryByDisplayValue('synthetic-one-time-code')
    ).not.toBeInTheDocument()
  })

  it('copies a complete private activation package without adding the code to its URL', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    const { client } = mount(<ActivationDialog user={user} close={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Issue one-time code' }))
    const instructions = await screen.findByLabelText(
      'Complete activation instructions'
    )
    const text = (instructions as HTMLTextAreaElement).value
    expect(text).toContain(`User portal: ${window.location.origin}/user`)
    expect(text).toContain('Username: alice')
    expect(text).toContain('synthetic-one-time-code')
    expect(text).toContain('2026-10-22T00:00:00Z')
    fireEvent.click(
      screen.getByRole('button', { name: 'Copy complete instructions' })
    )
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        (instructions as HTMLTextAreaElement).value
      )
    )
    expect(await screen.findByText('Copied')).toBeVisible()
    expect(client.getMutationCache().getAll()).toHaveLength(0)
    expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(
      'synthetic-one-time-code'
    )
  })

  it('clears an issued code when the administrator signs out, even before the surface unmounts', async () => {
    useAuthStore.getState().auth.setSession(Date.now() + 60_000)
    const close = vi.fn()
    mount(<ActivationDialog user={user} close={close} />)
    fireEvent.click(screen.getByRole('button', { name: 'Issue one-time code' }))
    await screen.findByDisplayValue('synthetic-one-time-code')
    useAuthStore.getState().auth.reset()
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
    expect(
      screen.queryByDisplayValue('synthetic-one-time-code')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByLabelText('Complete activation instructions')
    ).not.toBeInTheDocument()
  })

  it('keeps the activation package absent on issuance failure and permits retry', async () => {
    const normal = network.getMockImplementation()
    let failed = true
    network.mockImplementation(async (path, options) => {
      if (path.endsWith('/activation') && failed)
        throw new Error('synthetic issuance failure')
      return normal?.(path, options)
    })
    mount(<ActivationDialog user={user} close={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Issue one-time code' }))
    await screen.findByRole('alert')
    expect(
      screen.queryByLabelText('Complete activation instructions')
    ).not.toBeInTheDocument()
    failed = false
    fireEvent.click(screen.getByRole('button', { name: 'Issue one-time code' }))
    expect(
      await screen.findByLabelText('Complete activation instructions')
    ).toBeVisible()
  })

  it('requires administrator reauthentication before issuing a code without caching it', async () => {
    const normal = network.getMockImplementation()
    network.mockImplementation(async (path, options) => {
      if (
        path.endsWith('/activation') &&
        !options?.headers?.['X-Admin-Confirm-Token']
      )
        throw { response: { status: 403, data: { reauthRequired: true } } }
      return normal?.(path, options)
    })
    const { client } = mount(<ActivationDialog user={user} close={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Issue one-time code' }))
    fireEvent.change(
      await screen.findByLabelText(i18n.t('reauth.tokenLabel')),
      { target: { value: 'synthetic-confirm-only' } }
    )
    fireEvent.click(
      screen.getByRole('button', { name: i18n.t('reauth.submit') })
    )
    expect(
      await screen.findByLabelText('Complete activation instructions')
    ).toBeVisible()
    expect(client.getMutationCache().getAll()).toHaveLength(0)
    expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(
      'synthetic-one-time-code'
    )
    expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(
      'synthetic-confirm-only'
    )
  })

  it('does not retain a late activation response after signing out', async () => {
    useAuthStore.getState().auth.setSession(Date.now() + 60_000)
    const normal = network.getMockImplementation()
    let resolve: (value: unknown) => void = () => {}
    network.mockImplementation((path, options) =>
      path.endsWith('/activation')
        ? new Promise((done) => {
            resolve = done
          })
        : Promise.resolve(normal?.(path, options))
    )
    const close = vi.fn()
    mount(<ActivationDialog user={user} close={close} />)
    fireEvent.click(screen.getByRole('button', { name: 'Issue one-time code' }))
    useAuthStore.getState().auth.reset()
    resolve({ code: 'synthetic-late-code', expiresAt: '2026-10-22T00:00:00Z' })
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
    expect(
      screen.queryByDisplayValue('synthetic-late-code')
    ).not.toBeInTheDocument()
  })

  it('grants only the public outer route ID, not composite source IDs', async () => {
    mount(<GroupAccessDialog group={group} close={vi.fn()} />)
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Grant route Published composite',
      })
    )
    await waitFor(() => expect(writes).toHaveLength(1))
    expect(writes[0]?.body).toEqual({ routeId: 15 })
  })

  it('does not render failed route candidates as a successful empty authorization list', async () => {
    const normal = network.getMockImplementation()
    network.mockImplementation(async (path, options) => {
      if (path === '/api/routes/summary') throw new Error('routes unavailable')
      return normal?.(path, options)
    })
    mount(<GroupAccessDialog group={group} close={vi.fn()} />)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not load management data'
    )
    expect(screen.queryByText(/No routes are granted/)).not.toBeInTheDocument()
    expect(
      screen.queryByText(/No published routes available/)
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
    expect(
      screen.queryByRole('button', { name: /^Grant route/ })
    ).not.toBeInTheDocument()
  })

  it('rejects malformed directory responses instead of returning empty users', async () => {
    network.mockResolvedValue({ items: null, limit: 1000 })
    await expect(gatewayUsersApi.users()).rejects.toThrow()
  })

  it('creates a group with no inferred grants and saves the token pricing multiplier', async () => {
    const close = vi.fn()
    const created = mount(
      <IdentityEditor kind='group' groups={[]} close={close} />
    )
    fireEvent.change(screen.getByLabelText('Group identifier'), {
      target: { value: 'limited' },
    })
    fireEvent.change(screen.getByLabelText('Group name'), {
      target: { value: 'Limited group' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
    expect(writes[0]?.body).toEqual({
      slug: 'limited',
      name: 'Limited group',
      enabled: true,
    })
    created.unmount()
    mount(<PricingEditor groupId={2} pricing={{ multiplier: 1 }} />)
    fireEvent.change(screen.getByLabelText('Token quota multiplier'), {
      target: { value: '0.5' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Saved')).toBeVisible()
    expect(writes[1]?.body).toEqual({ multiplier: 0.5 })
  })

  it('requires evidence confirmation and explicit terminated acknowledgement before manual settlement', async () => {
    mount(<BudgetHolds userId={1} />)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Reconcile this request' })
    )
    expect(screen.getByText(/Never guess zero/)).toBeVisible()
    fireEvent.change(
      screen.getByLabelText('Verified total tokens (all attempts)'),
      { target: { value: '1250000' } }
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Confirm verified settlement' })
    )
    expect(
      await screen.findByText(
        'Type VERIFIED after checking request and provider evidence.'
      )
    ).toBeVisible()
    expect(writes).toHaveLength(0)
    fireEvent.change(
      screen.getByLabelText(
        'Type VERIFIED to confirm the evidence was checked'
      ),
      { target: { value: 'VERIFIED' } }
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Confirm verified settlement' })
    )
    await waitFor(() => expect(writes).toHaveLength(1))
    expect(writes[0]?.body).toEqual({
      requestId: 'req_synthetic',
      totalTokens: 1250000,
      confirmTerminated: true,
    })
  })

  it('renders an issued activation code in Chinese and handles clipboard failure', async () => {
    await i18n.changeLanguage('zhCN')
    mount(<ActivationDialog user={user} close={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '签发一次性激活码' }))
    expect(await screen.findByLabelText('一次性激活码')).toHaveValue(
      'synthetic-one-time-code'
    )
    expect(screen.getByText(/过期时间/)).toBeVisible()
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: undefined,
    })
    fireEvent.click(screen.getByRole('button', { name: '复制激活码' }))
    expect(
      await screen.findByText('剪贴板不可用，请选中激活码手动复制。')
    ).toBeVisible()
    expect(
      (screen.getByLabelText('完整激活说明') as HTMLTextAreaElement).value
    ).toContain('用户名：alice')
    fireEvent.click(screen.getByRole('button', { name: '复制完整说明' }))
    expect(
      await screen.findByText('剪贴板不可用，请选中完整说明手动复制。')
    ).toBeVisible()
  })

  it('renders the actual management page, filters through URL state and performs reversible disable', async () => {
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
    mount(<RouterProvider router={router} />)
    const alice = await screen.findByRole('heading', { name: 'Alice' })
    const row = alice.closest('article')
    expect(row).not.toBeNull()
    expect(network.mock.calls.map(([path]) => path)).not.toContain(
      '/api/routes/summary'
    )
    expect(network.mock.calls.map(([path]) => path)).not.toContain(
      '/api/admin/users/1/budget'
    )
    fireEvent.click(
      within(row as HTMLElement).getByRole('button', {
        name: 'Provisioning check',
      })
    )
    expect(
      await screen.findByRole('heading', { name: 'Provisioning check · Alice' })
    ).toBeVisible()
    await waitFor(() =>
      expect(network.mock.calls.map(([path]) => path)).toContain(
        '/api/admin/user-groups/2/routes'
      )
    )
    fireEvent.click(
      screen.getAllByRole('button', { name: 'Close' })[0] as HTMLElement
    )
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    fireEvent.click(
      within(row as HTMLElement).getByRole('button', { name: 'Disable' })
    )
    await waitFor(() =>
      expect(writes[0]?.body).toEqual({
        displayName: 'Alice',
        groupId: 2,
        enabled: false,
      })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Search names or identifiers' }),
      { target: { value: 'missing' } }
    )
    expect(await screen.findByText(/No matching entries/)).toBeVisible()
    expect(history.location.search).toContain('q=missing')
    fireEvent.click(screen.getByRole('button', { name: 'Reset filters' }))
    expect(await screen.findByRole('heading', { name: 'Alice' })).toBeVisible()
  })
})
