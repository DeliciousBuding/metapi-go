import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen, within } from '@testing-library/react'
import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SettingsPage } from '../components/settings-page'
import type { SettingsSubarea } from '../types'

// Pins the WCAG 2.5.8 closeout for the settings breadcrumb text links
// (F-line residual D). Breadcrumb links are text-sm (20px line) with no
// padding, so ui:audit flagged them as TINY-HIT. They get `py-0.5` click
// padding for a 24px hit height, compensated by `-my-0.5` so the header
// row keeps its exact height — inline text links are exempt from 2.5.8's
// target-size requirement, this is a best-effort enlargement that must not
// shift the layout.

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
    const location = { href: '/settings/general/authentication' }
    return options?.select ? options.select(location) : location
  },
}))

const stubSubarea: SettingsSubarea = {
  id: 'general',
  title: 'General',
  basePath: '/settings/general',
  defaultSection: 'authentication',
  sectionIds: ['authentication'],
  getSectionNavItems: () => [
    { title: 'Authentication', url: '/settings/general/authentication' },
  ],
  getSectionContent: () => <div data-testid='section-content' />,
  getSectionMeta: () => ({
    id: 'authentication',
    title: 'Authentication',
    build: () => null,
  }),
}

afterEach(() => cleanup())

describe('SettingsPage breadcrumb link hit area', () => {
  it('gives every clickable breadcrumb link 24px hit height without growing the row', () => {
    render(<SettingsPage subarea={stubSubarea} activeSection='authentication' />)

    const breadcrumb = screen.getByRole('navigation', { name: 'breadcrumb' })
    // Only real anchors are clickable; the aria-disabled BreadcrumbPage span
    // (role=link, current page) is not an interaction target.
    const breadcrumbLinks = within(breadcrumb)
      .getAllByRole('link')
      .filter((element) => element.tagName === 'A')

    expect(breadcrumbLinks.length).toBeGreaterThanOrEqual(2)
    for (const link of breadcrumbLinks) {
      expect(link.classList).toContain('py-0.5')
      expect(link.classList).toContain('-my-0.5')
    }
  })
})
