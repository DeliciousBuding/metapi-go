import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'
import { request } from '@/lib/api/transport'

import { availableQuotaPreview, multiplierPreview } from '../quota-preview'
import { ReadinessDialog } from '../readiness'

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
const network = vi.mocked(request)
const responses = () => ({
  '/api/admin/users': { items: [user], limit: 1000 },
  '/api/admin/user-groups': { items: [group], limit: 1000 },
  '/api/admin/user-groups/2/routes': {
    items: [{ groupId: 2, routeId: 15, createdAt: user.createdAt }],
  },
  '/api/routes/summary': [
    {
      id: 15,
      modelPattern: 'example-*',
      displayName: 'Example',
      enabled: true,
    },
  ],
  '/api/admin/users/1/budget': {
    limit: 10,
    used: 3,
    reserved: 2,
    unlimited: false,
    frozen: false,
    unknownRequests: 0,
  },
})
let facts: Record<string, unknown>
function mount() {
  const actions = {
    close: vi.fn(),
    editUser: vi.fn(),
    editGroup: vi.fn(),
    editAccess: vi.fn(),
    editBudget: vi.fn(),
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ReadinessDialog userId={1} {...actions} />
    </QueryClientProvider>
  )
  return { actions, client }
}
function check(name: string) {
  const section = screen.getByRole('heading', { name }).closest('section')
  if (!section) throw new Error(`Missing check section ${name}`)
  return within(section)
}

beforeEach(async () => {
  await i18n.changeLanguage('en')
  facts = responses()
  network.mockReset()
  network.mockImplementation(async (path) => {
    if (!(path in facts)) throw new Error('Unexpected fixture path')
    return facts[path]
  })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('on-demand administrator provisioning facts', () => {
  it.each([0, 1])(
    'accepts SQLite route-summary enabled=%s without truthy coercion',
    async (enabled) => {
      facts['/api/routes/summary'] = [
        { id: 15, modelPattern: 'public-model', displayName: null, enabled },
      ]
      mount()
      await waitFor(() =>
        expect(
          check('Enabled authorized public route').getByText(
            enabled === 1 ? 'Passed' : 'Needs attention'
          )
        ).toBeVisible()
      )
    }
  )

  it.each([2, 'false', null])(
    'rejects an invalid route-summary enabled value %s',
    async (enabled) => {
      facts['/api/routes/summary'] = [
        { id: 15, modelPattern: 'public-model', displayName: null, enabled },
      ]
      mount()
      await waitFor(() =>
        expect(
          check('Enabled authorized public route').getByText('Unknown')
        ).toBeVisible()
      )
      expect(
        screen.queryByText('Configuration checks passed')
      ).not.toBeInTheDocument()
    }
  )

  it('passes configuration only while explicitly leaving activation and client results unknown', async () => {
    mount()
    expect(await screen.findByText('Configuration checks passed')).toBeVisible()
    for (const name of [
      'Account enabled',
      'Current group enabled',
      'Enabled authorized public route',
      'User quota',
    ]) {
      expect(check(name).getByText('Passed')).toBeVisible()
    }
    expect(screen.getByText(/Current available quota: 5 units/)).toBeVisible()
    expect(screen.getByText(/Activation has not been confirmed/)).toBeVisible()
    expect(
      screen.getByText(/does not test live upstream availability/)
    ).toBeVisible()
    expect(network).toHaveBeenCalledTimes(5)
    expect(
      network.mock.calls.every(
        ([, options]) => !options?.method || options.method === 'GET'
      )
    ).toBe(true)
  })

  it('marks disabled accounts, disabled groups, empty grants and zero quotas as needing attention', async () => {
    facts['/api/admin/users'] = {
      items: [{ ...user, enabled: false }],
      limit: 1000,
    }
    facts['/api/admin/user-groups'] = {
      items: [{ ...group, enabled: false }],
      limit: 1000,
    }
    facts['/api/admin/user-groups/2/routes'] = { items: [] }
    facts['/api/admin/users/1/budget'] = {
      ...responses()['/api/admin/users/1/budget'],
      limit: 0,
      used: 0,
      reserved: 0,
    }
    mount()
    await waitFor(() =>
      expect(screen.getAllByText('Needs attention')).toHaveLength(4)
    )
    expect(
      screen.queryByText('Configuration checks passed')
    ).not.toBeInTheDocument()
    expect(screen.getByText(/No enabled published route matches/)).toBeVisible()
  })

  it('opens the existing configuration editor for each failed fact', async () => {
    facts['/api/admin/users'] = {
      items: [{ ...user, enabled: false }],
      limit: 1000,
    }
    facts['/api/admin/user-groups'] = {
      items: [{ ...group, enabled: false }],
      limit: 1000,
    }
    facts['/api/admin/user-groups/2/routes'] = { items: [] }
    facts['/api/admin/users/1/budget'] = {
      ...responses()['/api/admin/users/1/budget'],
      limit: 0,
    }
    const { actions } = mount()
    await waitFor(() =>
      expect(screen.getAllByText('Needs attention')).toHaveLength(4)
    )
    fireEvent.click(
      check('Account enabled').getByRole('button', {
        name: 'Edit / change group',
      })
    )
    expect(actions.editUser).toHaveBeenCalledWith({ ...user, enabled: false })
    fireEvent.click(
      check('Current group enabled').getByRole('button', { name: 'Edit group' })
    )
    expect(actions.editGroup).toHaveBeenCalledWith({ ...group, enabled: false })
    fireEvent.click(
      check('Enabled authorized public route').getByRole('button', {
        name: 'Routes / pricing',
      })
    )
    expect(actions.editAccess).toHaveBeenCalledWith({
      ...group,
      enabled: false,
    })
    fireEvent.click(check('User quota').getByRole('button', { name: 'Quota' }))
    expect(actions.editBudget).toHaveBeenCalledWith({ ...user, enabled: false })
    expect(actions.close).toHaveBeenCalledTimes(4)
  })

  it('does not count disabled routes or unpublished source IDs as enabled authorized public routes', async () => {
    facts['/api/routes/summary'] = [
      { id: 15, modelPattern: '*', displayName: null, enabled: false },
    ]
    facts['/api/admin/user-groups/2/routes'] = {
      items: [
        { groupId: 2, routeId: 15, createdAt: user.createdAt },
        { groupId: 2, routeId: 99, createdAt: user.createdAt },
      ],
    }
    mount()
    await waitFor(() =>
      expect(
        check('Enabled authorized public route').getByText('Needs attention')
      ).toBeVisible()
    )
    expect(
      screen.queryByText('Configuration checks passed')
    ).not.toBeInTheDocument()
  })

  it('keeps unlimited quotas frozen when usage is unknown', async () => {
    facts['/api/admin/users/1/budget'] = {
      ...responses()['/api/admin/users/1/budget'],
      unlimited: true,
      frozen: true,
      unknownRequests: 1,
    }
    mount()
    await waitFor(() =>
      expect(check('User quota').getByText('Needs attention')).toBeVisible()
    )
    expect(
      check('User quota').getByText(
        /Raising the limit or enabling unlimited quota does not resolve unknown usage/
      )
    ).toBeVisible()
    expect(
      screen.queryByText('Configuration checks passed')
    ).not.toBeInTheDocument()
  })

  it('accepts an explicit unfrozen unlimited quota even with a finite zero cap', async () => {
    facts['/api/admin/users/1/budget'] = {
      ...responses()['/api/admin/users/1/budget'],
      limit: 0,
      unlimited: true,
    }
    mount()
    expect(await screen.findByText('Configuration checks passed')).toBeVisible()
    expect(screen.getByText(/Current available quota: Unlimited/)).toBeVisible()
  })

  it('treats malformed route data as an error, not a passing or empty result', async () => {
    facts['/api/routes/summary'] = [{ id: 15, enabled: 'yes' }]
    mount()
    await screen.findByRole('alert')
    await waitFor(() =>
      expect(
        check('Enabled authorized public route').getByText('Unknown')
      ).toBeVisible()
    )
    expect(
      screen.queryByText('Configuration checks passed')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText(/No enabled published route matches/)
    ).not.toBeInTheDocument()
  })

  it('refreshes the true current group and loads its grants rather than retaining old permissions', async () => {
    mount()
    await screen.findByText('Configuration checks passed')
    facts['/api/admin/users'] = {
      items: [{ ...user, groupId: 3 }],
      limit: 1000,
    }
    facts['/api/admin/user-groups'] = {
      items: [group, { ...group, id: 3, name: 'New group' }],
      limit: 1000,
    }
    facts['/api/admin/user-groups/3/routes'] = { items: [] }
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() =>
      expect(
        check('Enabled authorized public route').getByText('Needs attention')
      ).toBeVisible()
    )
    expect(network.mock.calls.map(([path]) => path)).toContain(
      '/api/admin/user-groups/3/routes'
    )
    expect(screen.getByText(/Current group: New group/)).toBeVisible()
    expect(
      screen.queryByText('Configuration checks passed')
    ).not.toBeInTheDocument()
  })

  it('allows keyboard Escape to dismiss the named dialog', async () => {
    const { actions } = mount()
    await screen.findByText('Configuration checks passed')
    const dialog = screen.getByRole('dialog', {
      name: 'Provisioning check · Alice',
    })
    fireEvent.keyDown(dialog, { key: 'Escape' })
    await waitFor(() => expect(actions.close).toHaveBeenCalledOnce())
  })

  it.each([
    '/api/admin/users',
    '/api/admin/user-groups',
    '/api/admin/user-groups/2/routes',
    '/api/routes/summary',
    '/api/admin/users/1/budget',
  ])(
    'does not pass when %s fails and recovers on refresh',
    async (failedPath) => {
      let failed = true
      network.mockImplementation(async (path) => {
        if (path === failedPath && failed)
          throw new Error('synthetic read failure')
        return facts[path]
      })
      mount()
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Could not load management data'
      )
      expect(
        screen.queryByText('Configuration checks passed')
      ).not.toBeInTheDocument()
      await waitFor(() =>
        expect(screen.getAllByText('Unknown').length).toBeGreaterThan(0)
      )
      failed = false
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      expect(
        await screen.findByText('Configuration checks passed')
      ).toBeVisible()
    }
  )

  it('invalidates a previously passing view on refresh failure instead of displaying stale success', async () => {
    mount()
    await screen.findByText('Configuration checks passed')
    network.mockImplementation(async (path) => {
      if (path === '/api/routes/summary')
        throw new Error('synthetic read failure')
      return facts[path]
    })
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await screen.findByRole('alert')
    expect(
      check('Enabled authorized public route').getByText('Unknown')
    ).toBeVisible()
    expect(
      screen.queryByText('Configuration checks passed')
    ).not.toBeInTheDocument()
  })

  it('keeps a missing capped-directory identity unknown and does not query a guessed group', async () => {
    facts['/api/admin/users'] = { items: [], limit: 1 }
    mount()
    await waitFor(() =>
      expect(check('Account enabled').getByText('Unknown')).toBeVisible()
    )
    expect(check('Current group enabled').getByText('Unknown')).toBeVisible()
    expect(network.mock.calls.map(([path]) => path)).not.toContain(
      '/api/admin/user-groups/0/routes'
    )
    expect(
      screen.queryByText('Configuration checks passed')
    ).not.toBeInTheDocument()
  })

  it('shows loading rather than pass while facts are still being read', async () => {
    let resolve: (value: unknown) => void = () => {}
    network.mockImplementation((path) =>
      path.endsWith('/budget')
        ? new Promise((done) => {
            resolve = done
          })
        : Promise.resolve(facts[path])
    )
    mount()
    expect(screen.getAllByText('Checking…')).toHaveLength(4)
    expect(
      screen.queryByText('Configuration checks passed')
    ).not.toBeInTheDocument()
    resolve(responses()['/api/admin/users/1/budget'])
    expect(await screen.findByText('Configuration checks passed')).toBeVisible()
  })

  it('switches all readiness copy into Chinese', async () => {
    await i18n.changeLanguage('zhCN')
    mount()
    expect(await screen.findByText('配置检查通过')).toBeVisible()
    expect(
      screen.getByRole('heading', { name: '激活 / 客户端接通：未知' })
    ).toBeVisible()
    expect(screen.getByRole('button', { name: '刷新' })).toBeVisible()
  })
})

describe('explanatory quota arithmetic', () => {
  it('retains exact micro-units and negative available amounts', () => {
    expect(availableQuotaPreview('0.3', 0.1, 0.2)).toBe('0')
    expect(availableQuotaPreview('0.300001', 0.1, 0.2)).toBe('0.000001')
    expect(availableQuotaPreview('1', 2, 0.000001)).toBe('-1.000001')
    expect(availableQuotaPreview('-1', 0, 0)).toBeNull()
    expect(availableQuotaPreview('1.0000001', 0, 0)).toBeNull()
    expect(availableQuotaPreview('1000000001', 0, 0)).toBeNull()
  })

  it('rounds synthetic request deductions up once at the minimum unit', () => {
    expect(multiplierPreview('2')).toEqual({
      fourteen: '0.000028',
      one: '0.000002',
    })
    expect(multiplierPreview('0.5')).toEqual({
      fourteen: '0.000007',
      one: '0.000001',
    })
    expect(multiplierPreview('0.000001')).toEqual({
      fourteen: '0.000001',
      one: '0.000001',
    })
    expect(multiplierPreview('1000000000')).toEqual({
      fourteen: '14000',
      one: '1000',
    })
    expect(multiplierPreview('0')).toBeNull()
    expect(multiplierPreview('1e2')).toBeNull()
  })
})
