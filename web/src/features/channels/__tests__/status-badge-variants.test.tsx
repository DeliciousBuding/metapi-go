// Badge recipe convergence regression (audit P1 #1): channel routing
// statuses render through semantic soft Badge variants — enabled uses the
// success variant, not the solid `default` primary block.

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { useChannelsColumns } from '../components/channels-columns'
import type { ChannelRow, ChannelStatus } from '../types'

function ChannelStatusCell({ status }: { status: ChannelStatus }) {
  const columns = useChannelsColumns()
  const statusColumn = columns.find((column) => column.id === 'status')
  if (!statusColumn?.cell) throw new Error('status column cell missing')
  const row = { status } as ChannelRow
  const cell = statusColumn.cell as unknown as (context: {
    row: { original: ChannelRow }
  }) => ReactElement
  return cell({ row: { original: row } })
}

function readBadgeVariant(container: HTMLElement): string | null {
  const badge = container.querySelector('[data-slot="badge"]')
  return badge?.getAttribute('data-variant') ?? null
}

afterEach(() => cleanup())

describe('channel status badge variants', () => {
  it.each([
    ['enabled', 'success'],
    ['cooldown', 'warning'],
    ['breaker_open', 'destructive'],
    ['manually_disabled', 'secondary'],
  ] as const)(
    'maps the %s routing status to the %s variant',
    (status, expectedVariant) => {
      const { container } = render(<ChannelStatusCell status={status} />)
      expect(readBadgeVariant(container)).toBe(expectedVariant)
    }
  )
})
