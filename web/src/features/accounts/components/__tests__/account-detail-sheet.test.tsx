// Regression test for the account detail sheet's amount rendering. Accounts
// whose balance was never refreshed carry null amounts; they must render an
// em dash instead of a misleading $0.00 (issue #889). Mocks only the refresh
// mutation and the embedded tokens panel; the real sheet renders.
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { accountSchema, type Account } from '../../types'
import { AccountDetailSheet } from '../account-detail-sheet'

vi.mock('../../api', () => ({
  useRefreshAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../../tokens/components/tokens-panel', () => ({
  TokensPanel: () => null,
}))

beforeAll(() => {
  // base-ui Sheet queries matchMedia under jsdom.
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

afterEach(() => cleanup())

function makeAccount(overrides: Partial<Account>): Account {
  return accountSchema.parse({
    id: 1,
    siteId: 2,
    username: 'probe-user',
    credentialMode: 'session',
    ...overrides,
  })
}

// DetailField renders `<div><dt>label</dt><dd>value</dd></div>`; read the
// value by its visible label.
function fieldValue(label: string): string {
  const term = screen.getByText(label)
  const definition = term.parentElement?.querySelector('dd')
  if (!definition) {
    throw new Error(`Detail field "${label}" not found`)
  }
  return definition.textContent ?? ''
}

describe('AccountDetailSheet amount rendering', () => {
  it('renders never-refreshed (null) amounts as an em dash, not $0.00', () => {
    const account = makeAccount({
      balance: null,
      balanceUsed: null,
      todayReward: null,
      todaySpend: null,
    })

    render(<AccountDetailSheet account={account} open onOpenChange={vi.fn()} />)

    expect(fieldValue('Balance')).toBe('—')
    expect(fieldValue('Used')).toBe('—')
    expect(fieldValue("Today's reward")).toBe('—')
    expect(fieldValue("Today's spend")).toBe('—')
    expect(screen.queryByText('$0.00')).not.toBeInTheDocument()
    expect(screen.queryByText('+$0.00')).not.toBeInTheDocument()
  })

  it('keeps the currency prefix for known amounts', () => {
    const account = makeAccount({
      balance: 12.345,
      balanceUsed: 0.5,
      todayReward: 2,
      todaySpend: 7,
    })

    render(<AccountDetailSheet account={account} open onOpenChange={vi.fn()} />)

    expect(fieldValue('Balance')).toBe('$12.35')
    expect(fieldValue('Used')).toBe('$0.50')
    expect(fieldValue("Today's reward")).toBe('+$2.00')
    expect(fieldValue("Today's spend")).toBe('$7.00')
  })
})
