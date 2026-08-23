import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SettingsOverview } from '../components/settings-overview'
import { getSettingsSubareas } from '../config/settings-config'

// Pins the overview tile contract (wave 8 lane C IA restructure): /settings
// renders exactly one whole-card link per subarea (5 tiles, no per-section
// lists) and a unique h1. Tiles link straight to each subarea's default
// section, so every tile is a large click target with a visible focus ring.

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

describe('SettingsOverview tiles', () => {
  it('renders a unique h1', () => {
    render(<SettingsOverview />)

    const headings = screen.getAllByRole('heading', { level: 1 })
    expect(headings).toHaveLength(1)
  })

  it('renders exactly one tile link per subarea, to its default section', () => {
    const subareas = getSettingsSubareas()
    render(<SettingsOverview />)

    const links = screen.getAllByRole('link')
    expect(links).toHaveLength(subareas.length)

    const hrefs = links.map((link) => link.getAttribute('href')).sort()
    const expected = subareas
      .map((subarea) => `${subarea.basePath}/${subarea.defaultSection}`)
      .sort()
    expect(hrefs).toEqual(expected)
  })

  it('gives every tile a visible focus ring', () => {
    render(<SettingsOverview />)

    for (const link of screen.getAllByRole('link')) {
      expect(link.classList).toContain('focus-visible:ring-2')
      expect(link.classList).toContain('focus-visible:outline-none')
    }
  })
})
