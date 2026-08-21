// Regression test for the About page's Build Info card (issue #887 residual):
// the three rows used to render a permanent em-dash because the feature never
// called the backend. They now come from `GET /api/about`, and the card must
// distinguish "loading", "request failed" and "the binary carries no such
// value" instead of collapsing all three into an em-dash.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AboutPage } from '../about-page'

const { mockGetAbout } = vi.hoisted(() => ({
  mockGetAbout: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: { getAbout: mockGetAbout },
}))

const INJECTED_BUILD_INFO = {
  version: 'v0.16.3',
  commit: '1604f49aa0b1c2d3e4f5061728394a5b6c7d8e9f',
  buildTime: '2026-08-21T09:30:00Z',
  goVersion: 'go1.26.6',
}

beforeEach(() => {
  mockGetAbout.mockReset()
})

afterEach(() => cleanup())

function renderAboutPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <AboutPage />
      </QueryClientProvider>
    ) as ReactElement
  )
}

/** The Build Info card, located by its section heading. */
function getBuildInfoCard(): HTMLElement {
  const heading = screen.getByText('Build info')
  const card = heading.closest('[data-slot="card"]')
  if (!(card instanceof HTMLElement)) {
    throw new Error('Build info card not found')
  }
  return card
}

/** Read the value cell of a Build Info row by its label. */
function getRowValue(label: string): string {
  const card = getBuildInfoCard()
  const labelNode = within(card).getByText(label)
  const row = labelNode.parentElement
  if (!row) throw new Error(`row for ${label} not found`)
  const valueNode = row.lastElementChild
  if (!valueNode) throw new Error(`value cell for ${label} not found`)
  return valueNode.textContent ?? ''
}

describe('AboutPage — Build Info card', () => {
  it('renders the real build provenance returned by the backend', async () => {
    mockGetAbout.mockResolvedValue(INJECTED_BUILD_INFO)

    renderAboutPage()

    await waitFor(() => {
      expect(getRowValue('Go version')).toBe('go1.26.6')
    })
    expect(getRowValue('Build time')).toBe('2026-08-21T09:30:00Z')
    expect(getRowValue('Commit')).toBe(INJECTED_BUILD_INFO.commit)
    expect(mockGetAbout).toHaveBeenCalledTimes(1)
  })

  it('prefers the backend binary version over the bundle constant', async () => {
    mockGetAbout.mockResolvedValue(INJECTED_BUILD_INFO)

    renderAboutPage()

    await waitFor(() => {
      expect(screen.getByText('v0.16.3')).toBeInTheDocument()
    })
  })

  it('shows an em-dash for fields the running binary does not carry', async () => {
    // A local `go build` injects no commit/build time: the backend answers with
    // empty strings, which must render as an em-dash — never as a fake value.
    mockGetAbout.mockResolvedValue({
      version: 'dev',
      commit: '',
      buildTime: '',
      goVersion: 'go1.26.6',
    })

    renderAboutPage()

    await waitFor(() => {
      expect(getRowValue('Go version')).toBe('go1.26.6')
    })
    expect(getRowValue('Commit')).toBe('—')
    expect(getRowValue('Build time')).toBe('—')
  })

  it('shows an em-dash when the backend omits the fields entirely', async () => {
    mockGetAbout.mockResolvedValue({})

    renderAboutPage()

    await waitFor(() => {
      expect(getRowValue('Commit')).toBe('—')
    })
    expect(getRowValue('Build time')).toBe('—')
    expect(getRowValue('Go version')).toBe('—')
  })

  it('renders an error state with retry instead of a fabricated value', async () => {
    mockGetAbout.mockRejectedValue(new Error('Network Error'))

    renderAboutPage()

    const card = await waitFor(() => {
      const buildInfoCard = getBuildInfoCard()
      expect(within(buildInfoCard).getByRole('alert')).toBeInTheDocument()
      return buildInfoCard
    })
    expect(
      within(card).getByText('Failed to load build info: Network Error')
    ).toBeInTheDocument()
    expect(
      within(card).getByRole('button', { name: 'Retry' })
    ).toBeInTheDocument()
    // The error state must not silently degrade into em-dash rows.
    expect(within(card).queryByText('—')).not.toBeInTheDocument()
  })

  it('keeps the network-independent page shell usable while loading', async () => {
    // Never resolves: the shell (title + repository links) must already render.
    mockGetAbout.mockReturnValue(new Promise(() => {}))

    renderAboutPage()

    expect(
      screen.getByRole('heading', { level: 1, name: 'About' })
    ).toBeInTheDocument()
    expect(screen.getByText('Repository')).toBeInTheDocument()
    // Loading is a skeleton, not an em-dash: "not loaded yet" and "the binary
    // carries no such value" must not look the same.
    const card = getBuildInfoCard()
    expect(within(card).queryByText('—')).not.toBeInTheDocument()
    expect(card.querySelector('[data-slot="skeleton"]')).not.toBeNull()
  })
})
