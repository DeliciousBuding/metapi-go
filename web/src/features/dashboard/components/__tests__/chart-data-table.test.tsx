// Component-level axe gate + semantics for the sr-only chart data summary
// table (S10 chart accessibility alternative layer, #1035). Pins the
// screen-reader contract: captioned table, column/row headers, visually
// hidden, zero axe violations.
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import axe from 'axe-core'
import { afterEach, describe, expect, it } from 'vitest'

import { ChartDataTable } from '../chart-data-table'

const baseProps = {
  caption: 'Income vs spend',
  seriesLabel: 'Series',
  columns: ['Total', 'Latest day'],
  rows: [
    { name: 'Income', values: ['$18.000', '$6.000'] },
    { name: 'Outcome', values: ['$10.000', '$2.000'] },
    { name: 'Net', values: ['$8.000', '$4.000'] },
  ],
}

afterEach(() => cleanup())

describe('ChartDataTable axe gate', () => {
  it('produces zero axe violations', async () => {
    const { container } = render(
      <main>
        <ChartDataTable {...baseProps} />
      </main>
    )

    const results = await axe.run(container)
    expect(results.violations).toEqual([])
  })
})

describe('ChartDataTable screen-reader semantics', () => {
  it('renders a table named by its caption', () => {
    render(<ChartDataTable {...baseProps} />)

    expect(
      screen.getByRole('table', { name: 'Income vs spend' })
    ).toBeInTheDocument()
  })

  it('marks the series column and key points as column headers', () => {
    render(<ChartDataTable {...baseProps} />)

    const headers = screen.getAllByRole('columnheader')
    expect(headers.map((header) => header.textContent)).toEqual([
      'Series',
      'Total',
      'Latest day',
    ])
  })

  it('marks each series as a row header with one row per series', () => {
    render(<ChartDataTable {...baseProps} />)

    expect(
      screen.getAllByRole('rowheader').map((header) => header.textContent)
    ).toEqual(['Income', 'Outcome', 'Net'])
    // header row + one body row per series
    expect(screen.getAllByRole('row')).toHaveLength(4)
  })

  it('stays visually hidden via sr-only', () => {
    const { container } = render(<ChartDataTable {...baseProps} />)

    expect(container.querySelector('table')).toHaveClass('sr-only')
  })

  it('renders nothing without rows', () => {
    const { container } = render(<ChartDataTable {...baseProps} rows={[]} />)

    expect(container).toBeEmptyDOMElement()
  })
})
