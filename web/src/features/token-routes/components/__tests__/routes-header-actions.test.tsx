// Responsive page-header action cluster for the token-routes list page.
// Contract under test (issue #889 mobile audit): at ≤640px / English locale
// the primary "Add Route" CTA must remain a directly rendered, always-visible
// button, while the secondary actions (Auto-rebuild, Refresh decisions)
// collapse into a "More" dropdown instead of overflowing the SidebarInset
// overflow-x-hidden ancestor and getting clipped. The desktop (≥sm) inline
// rendering of the secondary actions must also stay intact.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { RoutesHeaderActions } from '../routes-header-actions'

// Base UI dropdown positioning needs matchMedia + ResizeObserver under jsdom.
beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

afterEach(() => cleanup())

function renderActions(
  overrides: Partial<Parameters<typeof RoutesHeaderActions>[0]> = {}
) {
  const onRebuild = vi.fn()
  const onRefreshDecisions = vi.fn()
  const onAddRoute = vi.fn()
  render(
    <RoutesHeaderActions
      onRebuild={onRebuild}
      isRebuildPending={false}
      onRefreshDecisions={onRefreshDecisions}
      isRefreshDecisionsPending={false}
      onAddRoute={onAddRoute}
      {...overrides}
    />
  )
  return { onRebuild, onRefreshDecisions, onAddRoute }
}

describe('RoutesHeaderActions responsive collapse', () => {
  it('always renders the primary Add Route CTA outside the collapse container', () => {
    renderActions()

    const addButton = screen.getByRole('button', { name: /add route/i })
    expect(addButton).toBeInTheDocument()
    // The CTA must never live inside the mobile-only (`sm:hidden`) container.
    expect(addButton.closest('.sm\\:hidden')).toBeNull()
  })

  it('collapses Auto-rebuild into the More dropdown for <sm', () => {
    const { onRebuild } = renderActions()

    fireEvent.click(screen.getByRole('button', { name: /more actions/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /auto-rebuild/i }))
    expect(onRebuild).toHaveBeenCalledTimes(1)
  })

  it('collapses Refresh decisions into the More dropdown for <sm', () => {
    const { onRefreshDecisions } = renderActions()

    fireEvent.click(screen.getByRole('button', { name: /more actions/i }))
    // Base UI closes the menu after each selection, so query the item fresh.
    fireEvent.click(
      screen.getByRole('menuitem', { name: /refresh decisions/i })
    )
    expect(onRefreshDecisions).toHaveBeenCalledTimes(1)
  })

  it('keeps the desktop (≥sm) inline secondary actions wired to the same handlers', () => {
    const { onRebuild, onRefreshDecisions } = renderActions()

    // With the dropdown closed the only matching buttons in the tree are the
    // inline desktop ones (the collapsed copies live inside the menu).
    fireEvent.click(screen.getByRole('button', { name: /auto-rebuild/i }))
    expect(onRebuild).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: /refresh decisions/i }))
    expect(onRefreshDecisions).toHaveBeenCalledTimes(1)
  })

  it('fires the Add Route CTA handler', () => {
    const { onAddRoute } = renderActions()

    fireEvent.click(screen.getByRole('button', { name: /add route/i }))
    expect(onAddRoute).toHaveBeenCalledTimes(1)
  })

  it('disables secondary actions while their mutations are pending', () => {
    renderActions({ isRebuildPending: true, isRefreshDecisionsPending: true })

    fireEvent.click(screen.getByRole('button', { name: /more actions/i }))

    // Base UI menu items expose the disabled state via aria-disabled.
    expect(
      screen.getByRole('menuitem', { name: /auto-rebuild/i })
    ).toHaveAttribute('aria-disabled', 'true')
  })
})
