import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SettingsOverview } from '../components/settings-overview'

// Pins the WCAG 2.5.8 closeout for the settings-overview subarea header
// links (F-line residual: 20px hit height, no hover feedback, no focus ring).
// Mirrors the breadcrumb pattern from settings-page.tsx: `py-0.5` click
// padding takes the 20px text link to 24px, compensated by `-my-0.5` so the
// card row keeps its exact height. `hover:bg-accent` (title text is already
// foreground, a text-color-only hover would be invisible) plus a
// focus-visible ring give keyboard users a visible focus target.

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
}))

afterEach(() => cleanup())

describe('SettingsOverview subarea header link hit area', () => {
  it('gives every subarea header link 24px hit height, hover feedback and focus ring classes', () => {
    render(<SettingsOverview />)

    // The subarea header links carry the group/subarea marker; section list
    // links below do not.
    const headerLinks = screen
      .getAllByRole('link')
      .filter((element) => element.className.includes('group/subarea'))

    expect(headerLinks.length).toBeGreaterThanOrEqual(5)
    for (const link of headerLinks) {
      expect(link.classList).toContain('py-0.5')
      expect(link.classList).toContain('-my-0.5')
      expect(link.classList).toContain('hover:bg-accent')
      expect(link.classList).toContain('focus-visible:ring-2')
    }
  })
})
