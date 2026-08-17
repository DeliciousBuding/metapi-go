// Behavior tests for StatCard's optional drilldown link. Asserts only the
// user-visible navigation affordance: when `to` is set the whole card is a
// link to the destination; when omitted the card is a plain, non-interactive
// surface. @tanstack/react-router's Link is stubbed so the test exercises the
// StatCard wiring in isolation (no router context needed).

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { StatCard } from '../stat-card'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    className,
  }: {
    to?: string
    children: ReactNode
    className?: string
  }) => (
    <a href={to} className={className}>
      {children}
    </a>
  ),
}))

afterEach(() => cleanup())

describe('StatCard drilldown link', () => {
  it('wraps the card in a link pointing at `to` when set', () => {
    render(<StatCard title='Accounts' value='42' to='/accounts' />)

    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/accounts')
    expect(link).toHaveTextContent('Accounts')
    expect(link).toHaveTextContent('42')
  })

  it('renders a plain card with no link when `to` is omitted', () => {
    render(<StatCard title='Sites' value='7' />)

    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByText('Sites')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
  })
})
