// Badge recipe convergence regression (audit P1 #1): account health badges
// use semantic soft variants — healthy uses the success variant (not the
// solid `default` primary block); degraded/unhealthy/expired keep their
// warning/destructive tones.

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { useAccountsColumns } from '../components/accounts-columns'
import type { Account, AccountRowActions } from '../types'

const noopActions: AccountRowActions = {
  onViewDetail: () => {},
  onRefresh: () => {},
  onTogglePin: () => {},
  onToggleStatus: () => {},
  onToggleCheckin: () => {},
  onEdit: () => {},
  onDelete: () => {},
}

function makeAccount(overrides: Record<string, unknown>): Account {
  return {
    id: 1,
    siteId: 1,
    username: 'probe-account',
    status: 'active',
    runtimeHealth: { state: 'unknown', reason: '' },
    ...overrides,
  } as unknown as Account
}

function AccountHealthCell({ account }: { account: Account }) {
  const columns = useAccountsColumns(noopActions)
  const statusColumn = columns.find((column) => column.id === 'status')
  if (!statusColumn?.cell) throw new Error('status column cell missing')
  const cell = statusColumn.cell as unknown as (context: {
    row: { original: Account }
  }) => ReactElement
  return cell({ row: { original: account } })
}

function readBadgeVariant(container: HTMLElement): string | null {
  const badge = container.querySelector('[data-slot="badge"]')
  return badge?.getAttribute('data-variant') ?? null
}

afterEach(() => cleanup())

describe('account health badge variants', () => {
  it('renders a healthy account with the success soft variant', () => {
    const account = makeAccount({
      runtimeHealth: { state: 'healthy', reason: '' },
    })
    const { container } = render(<AccountHealthCell account={account} />)
    expect(readBadgeVariant(container)).toBe('success')
  })

  it('renders a degraded account with the warning variant', () => {
    const account = makeAccount({
      runtimeHealth: { state: 'degraded', reason: '' },
    })
    const { container } = render(<AccountHealthCell account={account} />)
    expect(readBadgeVariant(container)).toBe('warning')
  })

  it('renders an unhealthy account with the destructive variant', () => {
    const account = makeAccount({
      runtimeHealth: { state: 'unhealthy', reason: '' },
    })
    const { container } = render(<AccountHealthCell account={account} />)
    expect(readBadgeVariant(container)).toBe('destructive')
  })

  it('renders an expired account with the destructive variant', () => {
    const account = makeAccount({ status: 'expired' })
    const { container } = render(<AccountHealthCell account={account} />)
    expect(readBadgeVariant(container)).toBe('destructive')
  })

  it('renders a disabled account with the neutral secondary variant', () => {
    const account = makeAccount({
      status: 'disabled',
      runtimeHealth: { state: 'disabled', reason: '' },
    })
    const { container } = render(<AccountHealthCell account={account} />)
    expect(readBadgeVariant(container)).toBe('secondary')
  })
})
