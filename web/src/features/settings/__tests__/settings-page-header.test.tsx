import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { SettingsPage } from '../components/settings-page'
import type { SettingsSubarea } from '../types'

// Pins the single-h1 page-header contract (wave 8 lane C IA restructure):
// the settings section page renders exactly one h1 (the section title) plus
// its description, and no longer renders a breadcrumb or an in-page
// secondary sidebar — the main sidebar's collapsible tree is the single
// navigation surface.

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
    description: 'Admin token rotation and IP allowlist.',
    build: () => null,
  }),
}

afterEach(() => cleanup())

describe('SettingsPage page header', () => {
  it('renders exactly one h1 with the active section title', () => {
    render(
      <SettingsPage subarea={stubSubarea} activeSection='authentication' />
    )

    const headings = screen.getAllByRole('heading', { level: 1 })
    expect(headings).toHaveLength(1)
    expect(headings[0]).toHaveTextContent('Authentication')
  })

  it('renders the section description under the h1', () => {
    render(
      <SettingsPage subarea={stubSubarea} activeSection='authentication' />
    )

    expect(
      screen.getByText('Admin token rotation and IP allowlist.')
    ).toBeInTheDocument()
  })

  it('renders no breadcrumb navigation and no in-page secondary sidebar', () => {
    const { container } = render(
      <SettingsPage subarea={stubSubarea} activeSection='authentication' />
    )

    expect(
      screen.queryByRole('navigation', { name: 'breadcrumb' })
    ).not.toBeInTheDocument()
    expect(container.querySelector('aside')).not.toBeInTheDocument()
    expect(screen.getByTestId('section-content')).toBeInTheDocument()
  })
})
