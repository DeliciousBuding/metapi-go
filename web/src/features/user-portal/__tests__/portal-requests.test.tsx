import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import type { AxiosAdapter } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'
import { apiClient } from '@/lib/http-client'

import { userPortalApi } from '../api'
import { PortalRequests } from '../components/portal-requests'

const originalAdapter = apiClient.defaults.adapter
const originalMatchMedia = window.matchMedia
let compact = false
const request = {
  id: 'synthetic-request',
  modelRequested: 'public-model',
  terminalStatus: 'completed',
  latencyMs: 12,
  startedAt: '2026-10-22T00:00:00Z',
  endedAt: '2026-10-22T00:00:01Z',
  attemptCount: 2,
}
const quota = {
  unit: 'token_credits',
  state: 'settled',
  totalTokens: 14,
  groupMultiplier: 2,
  reserved: 0.006,
  charged: 0.000028,
}
const list = (item: unknown) => ({
  items: [item],
  total: 1,
  page: 1,
  pageSize: 20,
})
let client: QueryClient
let handler: (url: string) => unknown

beforeEach(async () => {
  compact = false
  window.matchMedia = vi.fn((query: string) => ({
    matches: compact && query === '(max-width: 640px)',
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(() => true),
  }))
  await i18n.changeLanguage('en')
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  handler = (url) =>
    url.includes('/requests?')
      ? list({ ...request, quota })
      : { ...request, quota }
  const adapter: AxiosAdapter = async (config) => {
    const data = await handler(config.url ?? '')
    if (data instanceof Error) throw data
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  }
  apiClient.defaults.adapter = adapter
})
afterEach(() => {
  cleanup()
  client.clear()
  apiClient.defaults.adapter = originalAdapter
  window.matchMedia = originalMatchMedia
})

async function mount(href = '/user?view=requests&requestId=synthetic-request') {
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: '/user',
    validateSearch: (search) => ({
      requestId:
        typeof search.requestId === 'string' ? search.requestId : undefined,
    }),
    component: () => {
      const search = route.useSearch()
      return (
        <PortalRequests
          userId={1}
          page={1}
          requestId={search.requestId}
          onCloseDetail={() =>
            void router.navigate({
              to: '/user',
              search: { view: 'requests', page: 1 },
            })
          }
        />
      )
    },
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: [href] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

function quotaValue(dialog: HTMLElement, label: string) {
  return within(dialog).getByText(label, { selector: 'dt' }).nextElementSibling
}

describe('personal request and usage DTOs', () => {
  it('shows model, outcome and timing together in the narrow-screen request card', async () => {
    compact = true
    await mount('/user?view=requests')
    const row = await screen.findByRole('listitem')
    expect(
      within(row).getByRole('link', { name: 'public-model' })
    ).toBeVisible()
    expect(within(row).getByText('Completed')).toBeVisible()
    expect(within(row).getByText('12 ms')).toBeVisible()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('refreshes a recent empty overview cache when opening request records', async () => {
    client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } })
    client.setQueryData(['userPortal', 1, 'requests', 1], {
      items: [],
      total: 0,
      page: 1,
      pageSize: 20,
    })
    await mount('/user?view=requests')
    expect(
      await screen.findByRole('link', { name: 'public-model' })
    ).toBeVisible()
  })

  it('sends URL search and quota filters to the server and clears them together', async () => {
    let queried = ''
    handler = (url) => {
      queried = url
      return { items: [], total: 0, page: 1, pageSize: 20 }
    }
    const router = await mount(
      '/user?view=requests&q=absent&status=completed&quotaState=unknown&page=9'
    )
    expect(await screen.findByText(/No retained requests match/)).toBeVisible()
    const params = new URLSearchParams(queried.split('?')[1])
    expect(params.get('search')).toBe('absent')
    expect(params.get('status')).toBe('completed')
    expect(params.get('quotaState')).toBe('unknown')
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    await waitFor(() =>
      expect(router.state.location.searchStr).not.toContain('quotaState')
    )
    expect(router.state.location.searchStr).not.toContain('status')
    expect(router.state.location.searchStr).toContain('page=1')
    expect(await screen.findByText(/No request records yet/)).toBeVisible()
  })

  it('preserves quota in list and detail, including explicit zero and null', async () => {
    const item = {
      ...request,
      quota: { ...quota, totalTokens: 0, charged: 0, reserved: 0 },
    }
    handler = (url) => (url.includes('?') ? list(item) : item)
    expect((await userPortalApi.requests(1)).items[0]?.quota).toEqual(
      item.quota
    )
    expect((await userPortalApi.request(request.id)).quota).toEqual(item.quota)
    handler = () => ({
      ...request,
      quota: { ...quota, totalTokens: null, charged: null },
    })
    expect((await userPortalApi.request(request.id)).quota).toMatchObject({
      totalTokens: null,
      charged: null,
    })
  })

  it('only permits missing quota for older APIs and explicit null for no record', async () => {
    handler = () => request
    expect((await userPortalApi.request(request.id)).quota).toBeUndefined()
    handler = () => ({ ...request, quota: null })
    expect((await userPortalApi.request(request.id)).quota).toBeNull()
  })

  it.each(
    [
      'not-an-object',
      [],
      {},
      { ...quota, unit: 'USD' },
      { ...quota, state: '<img src=x onerror=alert(1)>' },
      { ...quota, totalTokens: -1 },
      { ...quota, totalTokens: 1.5 },
      { ...quota, totalTokens: '14' },
      { ...quota, totalTokens: Number.MAX_SAFE_INTEGER + 1 },
      { ...quota, groupMultiplier: 0 },
      { ...quota, groupMultiplier: -1 },
      { ...quota, groupMultiplier: null },
      { ...quota, reserved: -0.1 },
      { ...quota, reserved: null },
      { ...quota, charged: -0.1 },
      { ...quota, charged: '0' },
      { ...quota, charged: Infinity },
      { ...quota, charged: NaN },
    ].map((invalid) => [invalid])
  )('rejects invalid present quota %# in both endpoints', async (invalid) => {
    const item = { ...request, quota: invalid }
    handler = (url) => (url.includes('?') ? list(item) : item)
    await expect(userPortalApi.requests(1)).rejects.toThrow()
    await expect(userPortalApi.request(request.id)).rejects.toThrow()
  })

  it('preserves recordedRequests independently of current-key counters', async () => {
    handler = () => ({ usedRequests: 2, keyCount: 0, recordedRequests: 14 })
    await expect(userPortalApi.usage()).resolves.toMatchObject({
      usedRequests: 2,
      keyCount: 0,
      recordedRequests: 14,
    })
    handler = () => ({ usedRequests: 20, keyCount: 1, recordedRequests: 0 })
    expect((await userPortalApi.usage()).recordedRequests).toBe(0)
    handler = () => ({ usedRequests: 20, keyCount: 1 })
    expect((await userPortalApi.usage()).recordedRequests).toBeUndefined()
  })

  it.each([null, -1, 1.5, '14', Infinity, NaN, Number.MAX_SAFE_INTEGER + 1])(
    'rejects invalid recordedRequests %s without falling back to usedRequests',
    async (recordedRequests) => {
      handler = () => ({ usedRequests: 20, keyCount: 1, recordedRequests })
      await expect(userPortalApi.usage()).rejects.toThrow()
    }
  )
})

describe('request quota details', () => {
  it('shows historical values and the exact rounding rule without recomputing a deduction', async () => {
    // Deliberately use a saved amount differing from the formula: the UI is not
    // a settlement authority and must not overwrite the ledger projection.
    handler = (url) =>
      url.includes('?')
        ? list({ ...request, quota })
        : { ...request, quota: { ...quota, charged: 0.000029 } }
    await mount()
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('Quota for this request')
    expect(quotaValue(dialog, 'Reported total tokens')).toHaveTextContent(
      /^14$/
    )
    expect(
      quotaValue(dialog, 'Group multiplier at request time')
    ).toHaveTextContent(/^2$/)
    expect(quotaValue(dialog, 'Reserved quota units')).toHaveTextContent(
      /^0.006$/
    )
    expect(
      quotaValue(dialog, 'Recorded deduction · quota units')
    ).toHaveTextContent(/^0.000029$/)
    expect(
      within(dialog).getByText(/ceil\(reported total tokens/)
    ).toBeVisible()
    expect(within(dialog).getByText(/1 token × 0.5 → 0.000001/)).toBeVisible()
    expect(
      within(dialog).getByText(/Later group or multiplier changes/)
    ).toBeVisible()
    expect(
      within(dialog).getByText(/Failed attempts are not guaranteed to be free/)
    ).toBeVisible()
    expect(
      within(dialog).getByText(/new request may consume additional quota/)
    ).toBeVisible()
    expect(within(dialog).getByText('Internal request ID')).toBeVisible()
    expect(within(dialog).getByText('Attempts')).toBeVisible()
  })

  it.each(['pending', 'settled', 'released', 'unknown'])(
    'renders %s with zero distinct from unknown',
    async (state) => {
      handler = (url) =>
        url.includes('?')
          ? list(request)
          : {
              ...request,
              quota: { ...quota, state, totalTokens: null, charged: 0 },
            }
      await mount()
      const dialog = await screen.findByRole('dialog')
      await within(dialog).findByText('Quota for this request')
      expect(quotaValue(dialog, 'Settlement status')).toHaveTextContent(
        i18n.t('userPortal.quotaDetails.states.' + state)
      )
      expect(quotaValue(dialog, 'Reported total tokens')).toHaveTextContent(
        /^Unknown$/
      )
      const label = ['pending', 'unknown'].includes(state)
        ? 'Recorded deduction · not finalized (quota units)'
        : 'Recorded deduction · quota units'
      expect(quotaValue(dialog, label)).toHaveTextContent(/^0$/)
      expect(
        within(dialog).getByText(
          i18n.t('userPortal.quotaDetails.guidance.' + state)
        )
      ).toBeVisible()
      expect(
        within(dialog).queryByRole('button', { name: /retry|resend/i })
      ).not.toBeInTheDocument()
    }
  )

  it('shows settled zero tokens and an unknown deduction as different facts', async () => {
    handler = (url) =>
      url.includes('?')
        ? list(request)
        : {
            ...request,
            quota: { ...quota, totalTokens: 0, charged: null },
          }
    await mount()
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('Quota for this request')
    expect(quotaValue(dialog, 'Reported total tokens')).toHaveTextContent(/^0$/)
    expect(
      quotaValue(dialog, 'Recorded deduction · quota units')
    ).toHaveTextContent(/^Unknown$/)
  })

  it.each([undefined, null])(
    'explains missing settlement %s without claiming free usage',
    async (missing) => {
      handler = (url) =>
        url.includes('?') ? list(request) : { ...request, quota: missing }
      await mount()
      const dialog = await screen.findByRole('dialog')
      expect(
        await within(dialog).findByText(
          i18n.t(
            missing === undefined
              ? 'userPortal.quotaDetails.legacyUnknown'
              : 'userPortal.quotaDetails.noRecord'
          )
        )
      ).toBeVisible()
      expect(
        within(dialog).queryByText('Recorded deduction · quota units')
      ).not.toBeInTheDocument()
    }
  )

  it('renders hostile request strings as text, and invalid quota as a retryable error', async () => {
    handler = (url) =>
      url.includes('?')
        ? list(request)
        : {
            ...request,
            modelRequested: '<img src=x onerror=alert(1)>',
            quota,
          }
    await mount()
    const dialog = await screen.findByRole('dialog')
    expect(
      await within(dialog).findByText('<img src=x onerror=alert(1)>')
    ).toBeVisible()
    expect(dialog.querySelector('img')).toBeNull()
    handler = (url) =>
      url.includes('?')
        ? list(request)
        : { ...request, quota: { ...quota, charged: -1 } }
    await act(async () => {
      await client.invalidateQueries({ queryKey: ['userPortal', 1, 'request'] })
    })
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      'Could not load'
    )
    expect(
      within(dialog).queryByText('Quota for this request')
    ).not.toBeInTheDocument()
    handler = (url) =>
      url.includes('?') ? list(request) : { ...request, quota }
    fireEvent.click(within(dialog).getByRole('button', { name: 'Retry' }))
    expect(
      await within(dialog).findByText('Quota for this request')
    ).toBeVisible()
  })

  it('shows loading and list errors rather than successful empty records', async () => {
    let resolve: (value: unknown) => void = () => undefined
    handler = () =>
      new Promise((done) => {
        resolve = done
      })
    await mount('/user?view=requests')
    expect(
      await screen.findByRole('status', { name: 'Loading…' })
    ).toBeVisible()
    await act(async () => {
      resolve(new Error('offline fixture'))
    })
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load')
    expect(screen.queryByText(/No request records yet/)).not.toBeInTheDocument()
    handler = () => ({ items: [], total: 0, page: 1, pageSize: 20 })
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText(/No request records yet/)).toBeVisible()
  })

  it('closes with Escape and returns focus to the original request entry', async () => {
    const router = await mount('/user?view=requests')
    const entry = await screen.findByRole('link', { name: 'public-model' })
    entry.focus()
    fireEvent.click(entry)
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('Quota for this request')
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' })
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await waitFor(() => expect(entry).toHaveFocus())
    expect(router.state.location.searchStr).not.toContain('requestId')
  })

  it('updates every quota label and explanation when switching to Chinese', async () => {
    await mount()
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('Quota for this request')
    await act(async () => {
      await i18n.changeLanguage('zhCN')
    })
    expect(within(dialog).getByText('本次配额')).toBeVisible()
    expect(quotaValue(dialog, '请求时的组倍率')).toHaveTextContent(/^2$/)
    expect(quotaValue(dialog, '已记录实扣 · 配额单位')).toHaveTextContent(
      /^0.000028$/
    )
    expect(within(dialog).getByText(/最小配额单位为 0.000001/)).toBeVisible()
    expect(dialog.textContent).not.toContain('userPortal.quotaDetails')
  })
})
