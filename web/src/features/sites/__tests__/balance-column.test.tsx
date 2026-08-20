// Cell-level tests for the sites list balance column: totalBalance wins,
// subscription remaining is the fallback, and a data-less site renders an
// em dash. Mirrors the status-badge-variants harness (real hook, cell
// invoked with a fixture row).

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

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

function renderBalanceCell(site: Partial<Site>) {
  function CellHarness() {
    const columns = useSitesColumns(noopActions)
    const balanceColumn = columns.find((column) => column.id === 'balance')
    if (!balanceColumn?.cell) throw new Error('balance column cell missing')
    const cell = balanceColumn.cell as unknown as (context: {
      row: { original: Site }
    }) => ReactElement
    return cell({ row: { original: site as Site } })
  }
  return render(<CellHarness />)
}

afterEach(() => cleanup())

describe('sites balance column', () => {
  it('renders the site totalBalance', () => {
    const { container } = renderBalanceCell({ totalBalance: 42.5 })
    expect(container.textContent).toBe('$42.50')
  })

  it('falls back to the subscription remaining USD', () => {
    const { container } = renderBalanceCell({
      subscriptionSummary: { activeCount: 1, totalRemainingUsd: 87.5 },
    })
    expect(container.textContent).toBe('$87.50')
  })

  it('renders an em dash when neither source has data', () => {
    const { container } = renderBalanceCell({})
    expect(container.textContent).toBe('—')
  })
})
