// Behavior tests for the catalog-sources keyboard reorder alternative
// (#1032): the row drag handle must be operable without a pointer — Enter or
// Space grabs, ArrowUp/ArrowDown move the drop highlight, Space/Enter drops
// (committing an absolute sortOrder through the existing PUT endpoint), and
// Escape (or focus loss) cancels without mutating.

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
import type { ReactElement } from 'react'
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

import { CatalogSourcesSection } from '../catalog-sources-section'

const {
  mockGetCatalogSync,
  mockUpdateCatalogSource,
  mockSyncCatalog,
  mockUpdateCatalogSyncConfig,
  mockCreateCatalogSource,
  mockDeleteCatalogSource,
} = vi.hoisted(() => ({
  mockGetCatalogSync: vi.fn(),
  mockUpdateCatalogSource: vi.fn(),
  mockSyncCatalog: vi.fn(),
  mockUpdateCatalogSyncConfig: vi.fn(),
  mockCreateCatalogSource: vi.fn(),
  mockDeleteCatalogSource: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getCatalogSync: mockGetCatalogSync,
    updateCatalogSource: mockUpdateCatalogSource,
    syncCatalog: mockSyncCatalog,
    updateCatalogSyncConfig: mockUpdateCatalogSyncConfig,
    createCatalogSource: mockCreateCatalogSource,
    deleteCatalogSource: mockDeleteCatalogSource,
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
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

function makeSource(
  id: number,
  name: string,
  sortOrder: number,
  type: 'official' | 'custom' = 'official'
) {
  return {
    id,
    name,
    url: `https://${name.toLowerCase().replaceAll(' ', '-')}.example.com/all.json`,
    enabled: true,
    type,
    sortOrder,
    lastSuccessAt: null,
    lastError: null,
    lastCount: 0,
    lastAttemptAt: null,
  }
}

beforeEach(() => {
  mockGetCatalogSync.mockReset()
  mockUpdateCatalogSource.mockReset()
  mockSyncCatalog.mockReset()
  mockUpdateCatalogSyncConfig.mockReset()
  mockCreateCatalogSource.mockReset()
  mockDeleteCatalogSource.mockReset()
  mockGetCatalogSync.mockResolvedValue({
    autoSync: true,
    intervalMin: 720,
    snapshot: { source: 'Mirror A', fetchedAt: null, models: 0 },
    sources: [
      makeSource(1, 'Mirror A', 0),
      makeSource(2, 'Mirror B', 1, 'custom'),
      makeSource(3, 'Mirror C', 2),
    ],
  })
  mockUpdateCatalogSource.mockResolvedValue({
    source: makeSource(1, 'Mirror A', 1),
  })
})

afterEach(() => cleanup())

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <CatalogSourcesSection />
      </QueryClientProvider>
    ) as ReactElement
  )
}

async function findHandle(name: string) {
  // The name may appear both in the merged-snapshot summary and the row.
  await screen.findAllByText(name)
  return screen.getByRole('button', { name: `Reorder ${name}` })
}

describe('CatalogSourcesSection keyboard reorder', () => {
  it('exposes the keyboard pattern to assistive tech', async () => {
    renderSection()
    const handle = await findHandle('Mirror A')

    expect(handle).toHaveAttribute(
      'aria-keyshortcuts',
      'Enter Space ArrowUp ArrowDown Escape'
    )
    expect(handle).toHaveAttribute('aria-pressed', 'false')
    expect(handle).toHaveAttribute(
      'aria-describedby',
      'catalog-source-reorder-help'
    )
    expect(screen.getByText(/To reorder with the keyboard:/)).toBeTruthy()
  })

  it('Enter grabs, ArrowDown moves, Space drops the absolute index', async () => {
    renderSection()
    const handle = await findHandle('Mirror A')

    fireEvent.keyDown(handle, { key: 'Enter' })
    expect(handle).toHaveAttribute('aria-pressed', 'true')

    fireEvent.keyDown(handle, { key: 'ArrowDown' })
    fireEvent.keyDown(handle, { key: ' ' })

    await waitFor(() => {
      expect(mockUpdateCatalogSource).toHaveBeenCalledWith(1, {
        sortOrder: 1,
      })
    })
    expect(handle).toHaveAttribute('aria-pressed', 'false')

    // Optimistic reorder: Mirror B now renders above Mirror A.
    const rows = screen.getAllByRole('row')
    expect(within(rows[1]).getByText('Mirror B')).toBeTruthy()
    expect(within(rows[2]).getByText('Mirror A')).toBeTruthy()
  })

  it('Space also grabs and Enter also drops', async () => {
    renderSection()
    const handle = await findHandle('Mirror A')

    fireEvent.keyDown(handle, { key: ' ' })
    expect(handle).toHaveAttribute('aria-pressed', 'true')

    fireEvent.keyDown(handle, { key: 'ArrowDown' })
    fireEvent.keyDown(handle, { key: 'Enter' })

    await waitFor(() => {
      expect(mockUpdateCatalogSource).toHaveBeenCalledWith(1, {
        sortOrder: 1,
      })
    })
  })

  it('ArrowUp on the first row clamps and drops nothing', async () => {
    renderSection()
    const handle = await findHandle('Mirror A')

    fireEvent.keyDown(handle, { key: 'Enter' })
    fireEvent.keyDown(handle, { key: 'ArrowUp' })
    fireEvent.keyDown(handle, { key: ' ' })

    expect(mockUpdateCatalogSource).not.toHaveBeenCalled()
    expect(handle).toHaveAttribute('aria-pressed', 'false')
  })

  it('ArrowDown on the last row clamps and drops nothing', async () => {
    renderSection()
    const handle = await findHandle('Mirror C')

    fireEvent.keyDown(handle, { key: 'Enter' })
    fireEvent.keyDown(handle, { key: 'ArrowDown' })
    fireEvent.keyDown(handle, { key: ' ' })

    expect(mockUpdateCatalogSource).not.toHaveBeenCalled()
    expect(handle).toHaveAttribute('aria-pressed', 'false')
  })

  it('Escape cancels the grab without committing', async () => {
    renderSection()
    const handle = await findHandle('Mirror A')

    fireEvent.keyDown(handle, { key: 'Enter' })
    fireEvent.keyDown(handle, { key: 'ArrowDown' })
    fireEvent.keyDown(handle, { key: 'Escape' })

    expect(mockUpdateCatalogSource).not.toHaveBeenCalled()
    expect(handle).toHaveAttribute('aria-pressed', 'false')
  })

  it('moving focus away cancels an active grab', async () => {
    renderSection()
    const handle = await findHandle('Mirror A')

    fireEvent.keyDown(handle, { key: 'Enter' })
    expect(handle).toHaveAttribute('aria-pressed', 'true')

    fireEvent.blur(handle)

    expect(mockUpdateCatalogSource).not.toHaveBeenCalled()
    expect(handle).toHaveAttribute('aria-pressed', 'false')
  })
})
