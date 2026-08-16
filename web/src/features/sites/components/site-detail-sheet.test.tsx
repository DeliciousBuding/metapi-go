import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SiteDetailSheet } from './site-detail-sheet'

const navigate = vi.hoisted(() => vi.fn())

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))

const site = {
  id: 7,
  name: 'Primary site',
  url: 'https://primary.example',
  platform: 'openai',
  status: 'active' as const,
}

beforeEach(() => {
  navigate.mockReset()
})

afterEach(() => cleanup())

describe('SiteDetailSheet guided-flow links', () => {
  it('uses the validated account-create and token-route destinations', () => {
    render(<SiteDetailSheet site={site} open onOpenChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /Manage accounts/ }))
    fireEvent.click(screen.getByRole('button', { name: /Manage routes/ }))

    expect(navigate).toHaveBeenNthCalledWith(1, {
      to: '/accounts',
      search: { siteId: 7, create: true },
      replace: true,
    })
    expect(navigate).toHaveBeenNthCalledWith(2, {
      to: '/token-routes',
      search: { siteId: 7 },
      replace: true,
    })
  })
})
