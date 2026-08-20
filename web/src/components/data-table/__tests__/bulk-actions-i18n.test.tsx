// Behavior tests for the bulk-actions toolbar copy (#889): the selection
// summary, toolbar aria-label and badge aria used hardcoded English with a
// naive plural "s". Everything now resolves through i18n plural keys.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { DataTableBulkActions } from '../toolbar/bulk-actions'

function makeTable(selectedCount: number) {
  return {
    getFilteredSelectedRowModel: () => ({
      rows: Array.from({ length: selectedCount }, () => ({})),
    }),
    resetRowSelection: () => {},
  }
}

afterEach(() => cleanup())

describe('DataTableBulkActions i18n', () => {
  it('renders the translated plural summary and aria for multiple rows', () => {
    render(
      <DataTableBulkActions table={makeTable(2) as never} entityName='Site'>
        <button type='button'>Action</button>
      </DataTableBulkActions>
    )

    expect(screen.getByRole('toolbar')).toHaveAttribute(
      'aria-label',
      'Bulk actions for 2 selected Sites'
    )
    expect(screen.getByText('Sites selected')).toBeInTheDocument()
    // Badge keeps a machine-readable count label.
    expect(screen.getByText('2')).toHaveAttribute('aria-label', '2 selected')
  })

  it('keeps the entity singular for a single selected row', () => {
    render(
      <DataTableBulkActions table={makeTable(1) as never} entityName='Site'>
        <button type='button'>Action</button>
      </DataTableBulkActions>
    )

    expect(screen.getByRole('toolbar')).toHaveAttribute(
      'aria-label',
      'Bulk actions for 1 selected Site'
    )
    expect(screen.getByText('Site selected')).toBeInTheDocument()
  })
})
