// Layout contract tests for the settings section nav (audit P2 #6 closeout).
//
// Below `lg` the nav must degrade into a single-row horizontally scrollable
// chip strip so it never buries the page content on a 375px viewport; at
// `lg` and above it keeps the sticky vertical sidebar shape. Active state is
// URL-derived (query string and trailing slashes ignored) and exposed as
// `aria-current="page"`. Asserts stable DOM/class contracts only.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SettingsSidebar } from '../components/settings-sidebar'
import type { SettingsSectionNavItem } from '../types'

const locationState = vi.hoisted(() => ({
  href: '/settings/general/authentication',
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    ...props
  }: {
    to?: unknown
    children?: ReactNode
  } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href={String(to)} {...props}>
      {children}
    </a>
  ),
  useLocation: (options?: {
    select?: (location: { href: string }) => unknown
  }) => {
    const location = { href: locationState.href }
    return options?.select ? options.select(location) : location
  },
}))

function requireElement(container: HTMLElement, selector: string): HTMLElement {
  const element = container.querySelector<HTMLElement>(selector)
  if (!element) throw new Error(`Expected element matching ${selector}`)
  return element
}

const sections: SettingsSectionNavItem[] = [
  {
    title: 'Authentication',
    url: '/settings/general/authentication',
    group: 'settings.groups.account',
  },
  {
    title: 'Notifications',
    url: '/settings/general/notifications',
    group: 'settings.groups.account',
  },
  {
    title: 'Danger zone',
    url: '/settings/general/danger',
    readonly: true,
  },
]

beforeEach(() => {
  locationState.href = '/settings/general/authentication'
})

afterEach(() => cleanup())

describe('SettingsSidebar', () => {
  it('renders one link per section and keeps group labels in the DOM', () => {
    render(<SettingsSidebar items={sections} title='General' />)

    expect(
      screen.getByRole('link', { name: /Authentication/ })
    ).toHaveAttribute('href', '/settings/general/authentication')
    expect(
      screen.getByRole('link', { name: /Notifications/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Danger zone/ })
    ).toBeInTheDocument()
    // Group labels stay in the DOM for the desktop layout; CSS hides them
    // below lg rather than unmounting them.
    expect(screen.getByText('General')).toBeInTheDocument()
  })

  it('marks the URL-derived active section with aria-current', () => {
    render(<SettingsSidebar items={sections} />)

    expect(
      screen.getByRole('link', { name: /Authentication/ })
    ).toHaveAttribute('aria-current', 'page')
    expect(
      screen.getByRole('link', { name: /Notifications/ })
    ).not.toHaveAttribute('aria-current')
  })

  it('matches the active section ignoring query strings and trailing slashes', () => {
    locationState.href = '/settings/general/notifications/?highlight=1'

    render(<SettingsSidebar items={sections} />)

    expect(screen.getByRole('link', { name: /Notifications/ })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(
      screen.getByRole('link', { name: /Authentication/ })
    ).not.toHaveAttribute('aria-current')
  })

  it('renders read-only sections with the read-only badge', () => {
    render(<SettingsSidebar items={sections} />)

    expect(screen.getByRole('link', { name: /Read-only/ })).toHaveTextContent(
      'Danger zone'
    )
  })

  it('collapses into a horizontal scrollable chip strip below lg', () => {
    const { container } = render(
      <SettingsSidebar items={sections} title='General' />
    )

    const nav = requireElement(container, 'nav')
    // Mobile-first overflow contract: one row, horizontal scroll.
    expect(nav.className).toContain('flex-row')
    expect(nav.className).toContain('overflow-x-auto')

    // Chips must not wrap or shrink, otherwise the strip reflows into a wall.
    for (const link of screen.getAllByRole('link')) {
      expect(link.className).toContain('whitespace-nowrap')
      expect(link.className).toContain('shrink-0')
      expect(link.className).toContain('rounded-full')
    }

    // Title and group labels collapse on mobile via hidden + lg:block while
    // staying mounted for the desktop layout.
    expect(screen.getByText('General').className).toContain('hidden')
    expect(screen.getByText('General').className).toContain('lg:block')
    expect(screen.getByText('settings.groups.account').className).toContain(
      'hidden'
    )
  })

  it('keeps the desktop sticky vertical sidebar contract', () => {
    const { container } = render(<SettingsSidebar items={sections} />)

    const aside = requireElement(container, 'aside')
    expect(aside.className).toContain('lg:sticky')
    expect(aside.className).toContain('lg:top-6')
    expect(aside.className).toContain('lg:w-60')

    const nav = requireElement(container, 'nav')
    expect(nav.className).toContain('lg:flex-col')
    for (const link of screen.getAllByRole('link')) {
      expect(link.className).toContain('lg:w-full')
      expect(link.className).toContain('lg:rounded-md')
    }
  })
})
