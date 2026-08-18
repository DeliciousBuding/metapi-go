// Badge recipe convergence regression (audit P1 #1): positive/semantic
// statuses must render through the design-system soft Badge variants
// (success / warning / info / destructive), never the solid `default`
// primary block and never a neutral variant hiding a coloured dot.
//
// The Badge primitive exposes its variant as a `data-variant` attribute
// (Base UI render state), so assertions target that semantic contract
// instead of fragile class snapshots.

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { useCheckinColumns } from '../components/checkin-columns'
import { FailureReasonBadge } from '../components/failure-reason-badge'
import type { CheckinLogRow, CheckinRowActions } from '../types'

const noopActions: CheckinRowActions = {
  onViewDetail: () => {},
  onTriggerAccount: () => {},
}

function CheckinStatusCell({ status }: { status: string }) {
  const columns = useCheckinColumns(noopActions)
  const statusColumn = columns.find((column) => column.id === 'status')
  if (!statusColumn?.cell) throw new Error('status column cell missing')
  const row = {
    checkin_logs: {
      id: 1,
      accountId: 1,
      status,
      message: null,
      reward: null,
      createdAt: '',
    },
    failureReason: null,
  } satisfies CheckinLogRow
  const cell = statusColumn.cell as unknown as (context: {
    row: { original: CheckinLogRow }
  }) => ReactElement
  return cell({ row: { original: row } })
}

function readBadgeVariant(container: HTMLElement): string | null {
  const badge = container.querySelector('[data-slot="badge"]')
  return badge?.getAttribute('data-variant') ?? null
}

afterEach(() => cleanup())

describe('checkin status badge variants', () => {
  it('renders the success status with the success soft variant', () => {
    const { container } = render(<CheckinStatusCell status='success' />)
    expect(readBadgeVariant(container)).toBe('success')
  })

  it('renders the skipped status with the neutral secondary variant', () => {
    const { container } = render(<CheckinStatusCell status='skipped' />)
    expect(readBadgeVariant(container)).toBe('secondary')
  })

  it('renders a failed status with the destructive variant', () => {
    const { container } = render(<CheckinStatusCell status='failed' />)
    expect(readBadgeVariant(container)).toBe('destructive')
  })
})

describe('failure reason badge variants', () => {
  const renderCategory = (category: string) =>
    render(
      <FailureReasonBadge
        reason={{
          code: 'probe_code',
          category,
          title: 'Reason title',
          actionHint: '',
          detailHint: '',
        }}
      />
    )

  it.each([
    ['auth', 'destructive'],
    ['verification', 'warning'],
    ['network', 'info'],
    ['site', 'outline'],
    ['state', 'success'],
    ['unknown', 'outline'],
  ] as const)(
    'maps the %s category to the %s variant',
    (category, expectedVariant) => {
      const { container } = renderCategory(category)
      expect(readBadgeVariant(container)).toBe(expectedVariant)
    }
  )

  it('falls back to the outline variant for an unrecognized category', () => {
    const { container } = renderCategory('something_new')
    expect(readBadgeVariant(container)).toBe('outline')
  })
})
