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
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'

import { ModelsPage } from '../models-page'

const model = {
  name: 'claude-example',
  description: '模型说明',
  accountCount: 0,
  tokenCount: 0,
  avgLatency: null,
  successRate: null,
  tags: [],
  supportedEndpointTypes: [],
  pricingSources: [],
  accounts: [],
}
let queryClient: QueryClient

beforeEach(async () => {
  await i18n.changeLanguage('zhCN')
  window.history.replaceState({}, '', '/models')
  vi.spyOn(api, 'getModelsMarketplace').mockResolvedValue({
    items: [model],
    total: 1,
  })
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
})
afterEach(async () => {
  cleanup()
  queryClient.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  await i18n.changeLanguage('en')
})

it.each([true, false])(
  'returns focus to the original model button after closing details (mobile=%s)',
  async (mobile) => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn((query: string) => ({
        matches: mobile,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      }))
    )
    const root = createRootRoute({ component: Outlet })
    const route = createRoute({
      getParentRoute: () => root,
      path: '/models',
      component: ModelsPage,
    })
    const router = createRouter({
      routeTree: root.addChildren([route]),
      history: createMemoryHistory({ initialEntries: ['/models'] }),
    })
    render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    const opener = await screen.findByRole('button', {
      name: `查看详情: ${model.name}`,
    })
    act(() => opener.focus())
    fireEvent.click(opener)
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: model.name })).toHaveFocus()
    )
    expect(opener).toBeInTheDocument()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await waitFor(() => expect(opener).toHaveFocus())
  }
)
