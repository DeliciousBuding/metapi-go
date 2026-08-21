// Responsive page-header actions for the proxy-logs list page.
// Contract under test (issue #889 mobile audit): the auto-refresh segmented
// control stays on the page (always visible), while Export and Refresh
// collapse into a "More" dropdown below `sm` instead of overflowing the
// ~420px header cluster and getting clipped. The ≥sm inline buttons must
// keep working with the same handlers.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ProxyLogsHeaderActions } from '../proxy-logs-header-actions'

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
  overrides: Partial<Parameters<typeof ProxyLogsHeaderActions>[0]> = {}
) {
  const onExport = vi.fn()
  const onRefresh = vi.fn()
  render(
    <ProxyLogsHeaderActions
      onExport={onExport}
      isExporting={false}
      onRefresh={onRefresh}
      isRefreshing={false}
      {...overrides}
    />
  )
  return { onExport, onRefresh }
}

describe('ProxyLogsHeaderActions responsive collapse', () => {
  it('collapses Export into the More dropdown for <sm', () => {
    const { onExport } = renderActions()

    fireEvent.click(screen.getByRole('button', { name: /more actions/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /export csv/i }))
    expect(onExport).toHaveBeenCalledTimes(1)
  })

  it('collapses Refresh into the More dropdown for <sm', () => {
    const { onRefresh } = renderActions()

    fireEvent.click(screen.getByRole('button', { name: /more actions/i }))
    // Base UI closes the menu after each selection, so query the item fresh.
    fireEvent.click(screen.getByRole('menuitem', { name: /^refresh$/i }))
    expect(onRefresh).toHaveBeenCalledTimes(1)
  })

  it('keeps the desktop (≥sm) inline Export/Refresh buttons wired', () => {
    const { onExport, onRefresh } = renderActions()

    // With the dropdown closed the only Export/Refresh buttons in the tree
    // are the inline desktop ones (the collapsed copies live in the menu).
    const desktopExport = screen.getByRole('button', { name: /export csv/i })
    fireEvent.click(desktopExport)
    expect(onExport).toHaveBeenCalledTimes(1)

    const desktopRefresh = screen.getByRole('button', { name: /^refresh$/i })
    fireEvent.click(desktopRefresh)
    expect(onRefresh).toHaveBeenCalledTimes(1)
  })

  it('disables Export while a CSV export is in flight', () => {
    renderActions({ isExporting: true })

    fireEvent.click(screen.getByRole('button', { name: /more actions/i }))

    // Base UI menu items expose the disabled state via aria-disabled.
    expect(
      screen.getByRole('menuitem', { name: /export csv/i })
    ).toHaveAttribute('aria-disabled', 'true')
  })
})
