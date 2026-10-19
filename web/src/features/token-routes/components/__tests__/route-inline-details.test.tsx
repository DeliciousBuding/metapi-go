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

import '@/i18n/config'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { api } from '@/lib/api'

import type {
  RouteRowActions,
  RouteSummaryRow,
  RouteChannel,
} from '../../types'
import { RouteChannelList } from '../route-channel-list'
import { useRoutesColumns } from '../routes-columns'

const routes: RouteSummaryRow[] = [1, 2].map((id) => ({
  id,
  modelPattern: `model-${id}`,
  displayName: null,
  displayIcon: null,
  routeMode: 'pattern',
  modelMapping: null,
  routingStrategy: 'weighted',
  contextLength: null,
  enabled: true,
  channelCount: 1,
  enabledChannelCount: 1,
  siteNames: ['Example site'],
  decisionSnapshot: null,
  decisionRefreshedAt: null,
}))
const channel: RouteChannel = {
  id: 10,
  accountId: 3,
  tokenId: 5,
  sourceModel: 'model-1',
  priority: 2,
  weight: 10,
  enabled: true,
  manualOverride: false,
  successCount: 8,
  failCount: 1,
  account: { username: 'Example account' },
  site: { id: 4, name: 'Example site', platform: 'new-api' },
  token: {
    id: 5,
    name: 'Example key',
    accountId: 3,
    enabled: true,
    isDefault: true,
  },
}
const actions: RouteRowActions = {
  onEdit: vi.fn(),
  onDelete: vi.fn(),
  onToggleEnabled: vi.fn(),
  onViewDetail: vi.fn(),
  onClearCooldown: vi.fn(),
  onRefreshDecision: vi.fn(),
}
let mobile = false

beforeEach(() => {
  vi.spyOn(api, 'getRouteChannels').mockResolvedValue([channel])
  vi.spyOn(api, 'getModelPriceCompare').mockResolvedValue({ items: [] })
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

const contentId = (id: number) => `channel-details-${id}`
function Harness() {
  const [expanded, setExpanded] = useState<ReadonlySet<number>>(() => new Set())
  const toggle = useCallback(
    (id: number) =>
      setExpanded((previous) => {
        const next = new Set(previous)
        if (next.has(id)) next.delete(id)
        else next.add(id)
        return next
      }),
    []
  )
  const columns = useRoutesColumns(actions, null, null, new Set(), {
    expandedIds: expanded,
    toggle,
    getContentId: contentId,
  })
  const { table } = useDataTable({
    data: routes,
    columns,
    getRowId: (route) => String(route.id),
    enableRowSelection: true,
  })
  return (
    <DataTablePage
      table={table}
      rowDisclosure={{
        isExpanded: (row) => expanded.has(row.original.id),
        onToggle: (row) => toggle(row.original.id),
        getContentId: (row) => contentId(row.original.id),
        getLabel: (row) => `Channels for ${row.original.modelPattern}`,
        renderContent: (row) => <RouteChannelList route={row.original} />,
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
      <Harness />
    </QueryClientProvider>
  )
}

describe('route inline details', () => {
  it.each(['configured', 'observed'])(
    'shows sample cost provenance without fallback token prices (%s)',
    async (source) => {
      vi.mocked(api.getModelPriceCompare).mockResolvedValue({
        items: [
          {
            accountId: 3,
            model: 'model-1',
            source,
            ratesSource: 'fallback',
            missingPrice: false,
            inputPerMillion: 2,
            outputPerMillion: 2,
            estimatedCostSample: 0.25,
            configuredUnitCost: 0.25,
          },
        ],
      })
      renderHarness()
      fireEvent.click(screen.getByRole('button', { name: 'model-1' }))
      const region = screen.getByRole('region', {
        name: 'Channels for model-1',
      })
      // Wait for the provenance badge, not the initial loading placeholder:
      // a fallback-only rate must still read "Price unavailable" after the
      // price response arrives, instead of degrading to "— / —".
      await within(region).findByText('Cost source')
      expect(within(region).getByText('Price unavailable')).toBeInTheDocument()
      expect(region).not.toHaveTextContent('— / —')
      expect(region).toHaveTextContent('Cost source')
      expect(region).toHaveTextContent(
        source === 'configured' ? 'Configured' : 'Observed'
      )
      expect(region).not.toHaveTextContent('$2.0000')
      expect(region).not.toHaveTextContent('$0.2500')
    }
  )

  it('shows loading until channels arrive and reports malformed responses', async () => {
    let resolveChannels: (value: unknown) => void = () => {}
    vi.mocked(api.getRouteChannels).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveChannels = resolve
        })
    )
    renderHarness()
    fireEvent.click(screen.getByRole('button', { name: 'model-1' }))
    const region = screen.getByRole('region', { name: 'Channels for model-1' })
    expect(within(region).getAllByRole('status').length).toBeGreaterThan(0)
    resolveChannels({ channels: [] })
    expect(await within(region).findByRole('alert')).toHaveTextContent(
      'Failed to load route channels'
    )
    expect(within(region).queryByRole('list')).not.toBeInTheDocument()
  })

  it.each([false, true])(
    'lazily expands multiple rows and keeps controls independent (mobile=%s)',
    async (isMobile) => {
      mobile = isMobile
      renderHarness()
      expect(api.getRouteChannels).not.toHaveBeenCalled()
      fireEvent.click(screen.getByRole('button', { name: 'model-1' }))
      const region = await screen.findByRole('region', {
        name: 'Channels for model-1',
      })
      await within(region).findByText('Example account')
      expect(screen.getByRole('button', { name: 'model-1' })).toHaveAttribute(
        'aria-expanded',
        'true'
      )
      expect(region).toHaveTextContent('Example key')
      expect(region).toHaveTextContent('100.0%')
      expect(region).toHaveTextContent('Price unavailable')
      fireEvent.click(within(region).getByText('Example account'))
      expect(region).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'model-2' }))
      expect(screen.getAllByRole('region')).toHaveLength(2)
      fireEvent.click(
        screen.getAllByRole('checkbox', { name: 'Select row' })[0]
      )
      expect(screen.getAllByRole('region')).toHaveLength(2)
      fireEvent.click(screen.getByRole('button', { name: 'model-1' }))
      expect(
        screen.queryByRole('region', { name: 'Channels for model-1' })
      ).not.toBeInTheDocument()
    }
  )

  it('expands from a non-interactive row cell and retries a failed channel request', async () => {
    vi.mocked(api.getRouteChannels).mockRejectedValueOnce(
      new Error('temporary failure')
    )
    renderHarness()
    fireEvent.click(screen.getAllByText('Weighted random')[0])
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'temporary failure'
    )
    expect(screen.queryByText('No channels yet')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByText('Example account')
    await waitFor(() =>
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    )
  })
})
