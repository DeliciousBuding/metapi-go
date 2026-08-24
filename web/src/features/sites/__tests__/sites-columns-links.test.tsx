// Behavior tests for #985: the sites list name and URL cells expose safe,
// keyboard-focusable external links while invalid stored URLs degrade to text.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

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

function makeSite(overrides: Partial<Site> = {}): Site {
  return {
    id: 7,
    name: 'Primary site',
    url: 'https://primary.example',
    status: 'active',
    ...overrides,
  }
}

function renderLinkCells(site: Site, onRowClick = vi.fn()) {
  let renderedColumns: ReturnType<typeof useSitesColumns> = []

  function CellsHarness() {
    const columns = useSitesColumns(noopActions)
    renderedColumns = columns
    const renderCell = (columnId: 'name' | 'url') => {
      const column = columns.find((entry) => entry.id === columnId)
      if (!column?.cell) throw new Error(`${columnId} column cell missing`)
      const cell = column.cell as unknown as (context: {
        row: { original: Site }
      }) => ReactElement
      return cell({ row: { original: site } })
    }

    return (
      <div data-testid='row' onClick={onRowClick}>
        {renderCell('name')}
        {renderCell('url')}
      </div>
    )
  }

  return {
    ...render(<CellsHarness />),
    getColumns: () => renderedColumns,
    onRowClick,
  }
}

afterEach(() => cleanup())

describe('sites list external links', () => {
  it('links the site name and URL to the trimmed http(s) destination in a new tab', () => {
    renderLinkCells(
      makeSite({ url: '  https://primary.example/console?tab=keys#active  ' })
    )

    const nameLink = screen.getByRole('link', { name: 'Primary site' })
    const urlLink = screen.getByRole('link', {
      name: 'https://primary.example/console?tab=keys#active',
    })

    for (const link of [nameLink, urlLink]) {
      expect(link).toHaveAttribute(
        'href',
        'https://primary.example/console?tab=keys#active'
      )
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    }
  })

  it('keeps a valid http URL unchanged instead of upgrading its protocol', () => {
    renderLinkCells(makeSite({ url: 'http://localhost:4000/admin' }))

    const links = screen.getAllByRole('link')
    expect(links).toHaveLength(2)
    for (const link of links) {
      expect(link).toHaveAttribute('href', 'http://localhost:4000/admin')
    }
  })

  it('uses visible text as the accessible name and keeps both links keyboard focusable', () => {
    renderLinkCells(makeSite())

    const nameLink = screen.getByRole('link', { name: 'Primary site' })
    const urlLink = screen.getByRole('link', {
      name: 'https://primary.example',
    })

    nameLink.focus()
    expect(nameLink).toHaveFocus()
    expect(nameLink).toHaveClass('focus-visible:ring-2')
    urlLink.focus()
    expect(urlLink).toHaveFocus()
    expect(urlLink).toHaveClass('focus-visible:ring-2')
  })

  it('does not bubble either anchor click to an enclosing desktop or mobile row action', () => {
    const onRowClick = vi.fn()
    renderLinkCells(makeSite(), onRowClick)

    fireEvent.click(screen.getByRole('link', { name: 'Primary site' }))
    fireEvent.click(
      screen.getByRole('link', { name: 'https://primary.example' })
    )

    expect(onRowClick).not.toHaveBeenCalled()
  })

  it.each([
    ['javascript URL', 'javascript:alert(1)'],
    ['data URL', 'data:text/html,<script>alert(1)</script>'],
    ['file URL', 'file:///etc/passwd'],
    ['ftp URL', 'ftp://files.example/archive'],
    ['protocol-relative URL', '//primary.example/admin'],
    ['non-hierarchical HTTP URL', 'http:primary.example/admin'],
    ['cloud metadata URL', 'http://169.254.169.254/latest/meta-data'],
    [
      'metadata hostname',
      'https://metadata.google.internal/computeMetadata/v1',
    ],
    ['ordinary invalid value', 'not a valid URL'],
    ['blank value', '   '],
  ])('renders name and URL as plain text for a %s', (_label, url) => {
    renderLinkCells(makeSite({ url }))

    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByText('Primary site')).toBeInTheDocument()
    if (url.trim()) {
      expect(screen.getByText(url)).toBeInTheDocument()
    }
  })

  it('preserves the name and URL column mobile layout metadata', () => {
    const { getColumns } = renderLinkCells(makeSite())
    const nameColumn = getColumns().find((column) => column.id === 'name')
    const urlColumn = getColumns().find((column) => column.id === 'url')

    expect(nameColumn?.meta).toMatchObject({
      label: 'Name',
      mobileTitle: true,
      mobileOrder: 0,
    })
    expect(urlColumn?.meta).toMatchObject({
      label: 'URL',
      mobileOrder: 10,
    })
  })
})
