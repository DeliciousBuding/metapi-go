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
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import i18n from '@/i18n/config'
import { apiClient } from '@/lib/http-client'

import type { PersonalModel } from '../api'
import { PortalModels } from '../components/portal-models'

const original = apiClient.defaults.adapter
let client: QueryClient
let data: unknown
const model: PersonalModel = {
  id: 'public-model',
  routeId: 1,
  protocols: ['chat/completions', 'responses'],
  referencePrice: {
    currency: 'USD',
    unit: 'per_million_tokens',
    input: 0,
    output: 2,
    cacheRead: null,
    cacheWrite: null,
    updatedAt: null,
  },
}
const payload = (items: PersonalModel[]) => ({
  items,
  quota: { unit: 'token_credits', tokensPerUnit: 1000000, groupMultiplier: 2 },
})
beforeEach(async () => {
  await i18n.changeLanguage('en')
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  data = payload([model])
  const adapter: AxiosAdapter = async (config) => {
    if (data instanceof Error) throw data
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  }
  apiClient.defaults.adapter = adapter
})
afterEach(() => {
  cleanup()
  client.clear()
  apiClient.defaults.adapter = original
})
async function mount(href = '/user?view=models&page=1') {
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: '/user',
    component: () => <PortalModels userId={1} />,
    validateSearch: (search) => search,
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
describe('personal model catalog', () => {
  it('distinguishes zero reference prices from unknown and provides only configured request examples', async () => {
    await mount()
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'View pricing and connection for public-model',
      })
    )
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('0')).toBeVisible()
    expect(within(dialog).getAllByText('Unknown')).toHaveLength(2)
    expect(
      within(dialog).queryByRole('tab', { name: 'messages' })
    ).not.toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('tab', { name: 'responses' }))
    expect(within(dialog).getByLabelText('Request example')).toHaveTextContent(
      '/v1/responses'
    )
    expect(within(dialog).getByLabelText('Request example')).toHaveTextContent(
      'METAPI_API_KEY'
    )
    expect(within(dialog).getByText(/not your billing rate/)).toBeVisible()
    expect(screen.getByText(/current group multiplier 2/)).toBeVisible()
  })
  it('keeps search and paging in the real router and distinguishes no matches from no granted models', async () => {
    data = payload(
      Array.from({ length: 25 }, (_, i) => ({
        ...model,
        id: `model-${String(i).padStart(2, '0')}`,
      }))
    )
    const router = await mount('/user?view=models&page=2')
    expect(await screen.findByText('model-24')).toBeVisible()
    fireEvent.change(screen.getByLabelText('Search model names'), {
      target: { value: 'model-01' },
    })
    await waitFor(() =>
      expect(router.state.location.searchStr).toContain('page=1')
    )
    expect(router.state.location.searchStr).toContain('q=model-01')
    expect(await screen.findByText('model-01')).toBeVisible()
    await act(async () => {
      await router.navigate({ href: '/user?view=models&page=2' })
    })
    expect(await screen.findByText('model-24')).toBeVisible()
    fireEvent.change(screen.getByLabelText('Search model names'), {
      target: { value: 'absent' },
    })
    expect(
      await screen.findByText('No models match this search. Try another name.')
    ).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(await screen.findByText('model-00')).toBeVisible()
  })
  it('removes stale connection actions after a failed refresh and offers retry', async () => {
    await mount()
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'View pricing and connection for public-model',
      })
    )
    expect(await screen.findByRole('dialog')).toBeVisible()
    data = new Error('unavailable')
    await act(async () => {
      await client.invalidateQueries({ queryKey: ['userPortal', 1, 'models'] })
    })
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    data = payload([])
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText(/No callable models/)).toBeVisible()
  })
  it('never invents a chat sample for an unknown protocol or a missing reference price', async () => {
    data = payload([{ ...model, protocols: [], referencePrice: null }])
    await mount()
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'View pricing and connection for public-model',
      })
    )
    expect(
      await screen.findByText(/confirmed protocol information/)
    ).toBeVisible()
    expect(screen.getByText(/unknown does not mean free/)).toBeVisible()
    expect(screen.queryByLabelText('Request example')).not.toBeInTheDocument()
  })

  it('restores model and allowed protocol context from the URL and carries it to key creation', async () => {
    const router = await mount(
      '/user?view=models&page=1&model=public-model&protocol=responses'
    )
    const dialog = await screen.findByRole('dialog')
    expect(
      within(dialog).getByRole('tab', { name: 'responses' })
    ).toHaveAttribute('aria-selected', 'true')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create key' }))
    await waitFor(() =>
      expect(router.state.location.searchStr).toContain('view=keys')
    )
    expect(router.state.location.searchStr).toContain('model=public-model')
    expect(router.state.location.searchStr).toContain('protocol=responses')
  })
})
