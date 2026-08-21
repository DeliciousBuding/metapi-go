// Behavior test for the sites actions cell per-row pending state (#889):
// while a row's status/pin update is in flight, that row's Enable/Disable
// and Pin/Unpin menu items are disabled; other rows stay fully interactive.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import {
  afterAll,
  afterEach,
  beforeAll,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import {
  useSitesColumns,
  type SitesColumnActions,
} from '../components/sites-columns'
import type { Site } from '../types'

const noopActions: SitesColumnActions = {
  onEdit: () => {},
  onView: () => {},
  onToggleStatus: () => {},
  onTogglePin: () => {},
  onDelete: () => {},
}

function makeSite(overrides: Partial<Site>): Site {
  return {
    id: 1,
    name: 'OpenAI',
    url: 'https://api.openai.com',
    status: 'active',
    isPinned: false,
    ...overrides,
  }
}

function ActionsCell({
  site,
  pendingSiteId,
}: {
  site: Site
  pendingSiteId: number | null
}) {
  const columns = useSitesColumns(noopActions, pendingSiteId)
  const column = columns.find((entry) => entry.id === 'actions')
  if (!column?.cell) throw new Error('actions column cell missing')
  const cell = column.cell as unknown as (context: {
    row: { original: Site }
  }) => ReactElement
  return cell({ row: { original: site } })
}

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

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

afterEach(() => cleanup())

afterAll(() => {
  vi.restoreAllMocks()
})

async function openRowMenu() {
  fireEvent.click(screen.getByRole('button', { name: 'Row actions' }))
  return screen.findByRole('menuitem', { name: /Disable/ })
}

describe('sites actions cell per-row pending', () => {
  it('disables the status and pin toggles for the pending row', async () => {
    render(<ActionsCell site={makeSite({ id: 7 })} pendingSiteId={7} />)

    await openRowMenu()
    // The canonical Spinner (role=status) prepends its localized label to
    // the menu item's accessible name while pending.
    const statusToggle = screen.getByRole('menuitem', { name: /Disable/ })
    const pinToggle = screen.getByRole('menuitem', { name: /Pin/ })

    expect(statusToggle).toHaveAttribute('aria-disabled', 'true')
    expect(pinToggle).toHaveAttribute('aria-disabled', 'true')
    // Non-mutating actions stay enabled.
    expect(
      screen.getByRole('menuitem', { name: 'View details' })
    ).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('keeps every action enabled for rows that are not pending', async () => {
    render(<ActionsCell site={makeSite({ id: 8 })} pendingSiteId={7} />)

    await openRowMenu()

    expect(
      screen.getByRole('menuitem', { name: 'Disable' })
    ).not.toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('menuitem', { name: 'Pin' })).not.toHaveAttribute(
      'aria-disabled',
      'true'
    )
  })
})
