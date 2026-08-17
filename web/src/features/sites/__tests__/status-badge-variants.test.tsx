// Badge recipe convergence regression (audit P1 #1): site status badges
// keep the semantic soft variants — active stays on the success variant
// (the original sites migration that this convergence extends).

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import {
  useSitesColumns,
  type SitesColumnActions,
} from '../components/sites-columns'
import type { Site, SiteStatus } from '../types'

const noopActions: SitesColumnActions = {
  onEdit: () => {},
  onView: () => {},
  onToggleStatus: () => {},
  onTogglePin: () => {},
  onDelete: () => {},
}

function SiteStatusCell({ status }: { status: SiteStatus }) {
  const columns = useSitesColumns(noopActions)
  const statusColumn = columns.find((column) => column.id === 'status')
  if (!statusColumn?.cell) throw new Error('status column cell missing')
  const row = { status } as Site
  const cell = statusColumn.cell as unknown as (context: {
    row: { original: Site }
  }) => ReactElement
  return cell({ row: { original: row } })
}

function readBadgeVariant(container: HTMLElement): string | null {
  const badge = container.querySelector('[data-slot="badge"]')
  return badge?.getAttribute('data-variant') ?? null
}

afterEach(() => cleanup())

describe('site status badge variants', () => {
  it('renders an active site with the success soft variant', () => {
    const { container } = render(<SiteStatusCell status='active' />)
    expect(readBadgeVariant(container)).toBe('success')
  })

  it('renders a disabled site with the neutral secondary variant', () => {
    const { container } = render(<SiteStatusCell status='disabled' />)
    expect(readBadgeVariant(container)).toBe('secondary')
  })
})
