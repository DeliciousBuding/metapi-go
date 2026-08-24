import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SiteDetailSheet } from '../components/site-detail-sheet'
import type { Site } from '../types'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

const baseSite: Site = {
  id: 7,
  name: 'Primary site',
  url: 'https://primary.example/path',
  platform: 'openai',
  status: 'active',
}

afterEach(() => cleanup())

describe('site detail URL safety', () => {
  it.each(['https://primary.example/path', 'http://localhost:8080/path'])(
    'renders a valid URL as a safe new-tab link: %s',
    (siteURL) => {
      render(
        <SiteDetailSheet
          site={{ ...baseSite, url: siteURL }}
          open
          onOpenChange={vi.fn()}
        />
      )

      const link = screen.getByRole('link', { name: siteURL })
      expect(link).toHaveAttribute('href', siteURL)
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'))
      expect(link).toHaveAttribute('rel', expect.stringContaining('noreferrer'))
    }
  )

  it.each([
    'javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'file:///etc/passwd',
    'not a url',
  ])('renders an unsafe historical URL as plain text: %s', (unsafeURL) => {
    const site = { ...baseSite, url: unsafeURL }
    const { container } = render(
      <SiteDetailSheet site={site} open onOpenChange={vi.fn()} />
    )

    expect(screen.getAllByText(unsafeURL).length).toBeGreaterThan(0)
    expect(
      screen.queryByRole('link', { name: unsafeURL })
    ).not.toBeInTheDocument()
    expect(container.querySelector('a')).toBeNull()
  })
})
