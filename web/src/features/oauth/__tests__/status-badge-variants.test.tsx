// Badge recipe convergence regression (audit P1 #1): OAuth connection
// health renders through semantic soft Badge variants — healthy uses the
// success variant, not the solid `default` primary block.

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import {
  useOAuthColumns,
  type OAuthColumnActions,
} from '../components/oauth-columns'
import type { OAuthClient, OAuthClientStatus } from '../types'

const noopActions: OAuthColumnActions = {
  onRefreshQuota: () => {},
  onRebind: () => {},
  onDelete: () => {},
}

function OAuthStatusCell({
  status,
}: {
  status: OAuthClientStatus | undefined
}) {
  const columns = useOAuthColumns(noopActions)
  const statusColumn = columns.find((column) => column.id === 'status')
  if (!statusColumn?.cell) throw new Error('status column cell missing')
  const row = { status } as OAuthClient
  const cell = statusColumn.cell as unknown as (context: {
    row: { original: OAuthClient }
  }) => ReactElement
  return cell({ row: { original: row } })
}

function readBadgeVariant(container: HTMLElement): string | null {
  const badge = container.querySelector('[data-slot="badge"]')
  return badge?.getAttribute('data-variant') ?? null
}

afterEach(() => cleanup())

describe('oauth status badge variants', () => {
  it('renders a healthy connection with the success soft variant', () => {
    const { container } = render(<OAuthStatusCell status='healthy' />)
    expect(readBadgeVariant(container)).toBe('success')
  })

  it('renders an abnormal connection with the destructive variant', () => {
    const { container } = render(<OAuthStatusCell status='abnormal' />)
    expect(readBadgeVariant(container)).toBe('destructive')
  })

  it('treats a missing status as healthy', () => {
    const { container } = render(<OAuthStatusCell status={undefined} />)
    expect(readBadgeVariant(container)).toBe('success')
  })
})
