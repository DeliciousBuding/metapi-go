import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SiteCreatedModal } from '../site-created-modal'

const navigate = vi.hoisted(() => vi.fn())
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigate }))

beforeAll(() => {
  window.matchMedia = vi.fn().mockImplementation(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
})
afterEach(() => {
  cleanup()
  navigate.mockReset()
})

function mount(platform: string) {
  render(
    <SiteCreatedModal
      open
      onOpenChange={vi.fn()}
      site={{
        id: 12,
        name: 'Fixture connection',
        url: 'https://fixture.example',
        platform,
      }}
    />
  )
}

describe('site credential handoff', () => {
  it.each(['openai', 'claude', 'gemini'])(
    'continues %s to an API key',
    (platform) => {
      mount(platform)
      expect(
        screen.queryByRole('button', { name: 'Go to accounts' })
      ).not.toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'Add API Key' }))
      expect(navigate).toHaveBeenCalledWith({
        to: '/accounts',
        search: { siteId: 12, create: true, segment: 'apikey' },
        replace: true,
      })
    }
  )
  it('retains session and API-key choices for a management gateway', () => {
    mount('new-api')
    expect(screen.getByRole('button', { name: 'Add API Key' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Go to accounts' }))
    expect(navigate).toHaveBeenCalledWith({
      to: '/accounts',
      search: { siteId: 12, create: true },
      replace: true,
    })
  })
  it('continues an OAuth adapter to the OAuth connection flow', () => {
    mount('codex')
    expect(
      screen.queryByRole('button', { name: 'Add API Key' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Connect with OAuth' }))
    expect(navigate).toHaveBeenCalledWith({ to: '/oauth' })
  })
})
