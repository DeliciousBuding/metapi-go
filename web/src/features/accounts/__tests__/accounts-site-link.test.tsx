// Behavior tests for #1108: the accounts list site cell exposes a safe,
// keyboard-focusable external link (quick jump to the upstream site) while
// missing/invalid stored URLs degrade to plain text — same ladder as the
// sites page columns (#985).

import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import { describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { useAccountsColumns } from '../components/accounts-columns'
import type { Account, AccountRowActions } from '../types'

const noopActions: AccountRowActions = {
  onEdit: () => {},
  onDelete: () => {},
  onRefresh: () => {},
  onViewDetail: () => {},
  onTogglePin: () => {},
  onToggleCheckin: () => {},
  onToggleStatus: () => {},
}

function makeAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 1,
    siteId: 7,
    username: 'user@example.com',
    status: 'active',
    checkinEnabled: true,
    tags: [],
    site: {
      id: 7,
      name: 'Primary site',
      url: 'https://primary.example',
      platform: 'new-api',
      status: 'active',
    },
    ...overrides,
  } as unknown as Account
}

function SiteCell({ account }: { account: Account }) {
  const columns = useAccountsColumns(noopActions)
  const column = columns.find((entry) => entry.id === 'site')
  if (!column?.cell) throw new Error('site column cell missing')
  const cell = column.cell as unknown as (context: {
    row: { original: Account }
  }) => ReactElement
  return cell({ row: { original: account } })
}

describe('accounts site cell quick jump (#1108)', () => {
  it('renders the site name as an external link for a valid http(s) URL', () => {
    const { container } = render(<SiteCell account={makeAccount()} />)
    const link = container.querySelector('a')
    expect(link).not.toBeNull()
    expect(link).toHaveAttribute('href', 'https://primary.example')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    expect(screen.getByText('Primary site')).toBeInTheDocument()
  })

  it('degrades to plain text when the stored URL is not a valid http(s) endpoint', () => {
    const { container } = render(
      <SiteCell
        account={makeAccount({
          site: {
            id: 7,
            name: 'Broken site',
            url: 'javascript:alert(1)',
            platform: 'openai',
            status: 'active',
          },
        })}
      />
    )
    expect(container.querySelector('a')).toBeNull()
    expect(screen.getByText('Broken site')).toBeInTheDocument()
  })

  it('renders a dash when the account has no site', () => {
    const { container } = render(
      <SiteCell account={makeAccount({ site: undefined as never })} />
    )
    expect(container.querySelector('a')).toBeNull()
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})

// Silence the router probe: no navigation happens in these cell tests.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useSearch: () => ({}),
  useLocation: () => ({ searchStr: '', pathname: '/accounts' }),
}))
