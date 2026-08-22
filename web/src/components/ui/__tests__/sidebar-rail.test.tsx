import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { Sidebar, SidebarProvider, SidebarRail } from '../sidebar'

// Pins the WCAG 2.5.8 closeout for the sidebar rail (F-line residual C):
// the invisible drag strip is a pure pointer affordance, so its hit width is
// the whole story. jsdom computes no layout, so the test pins the Tailwind
// tokens that PRODUCE the 24px target: `w-6` (24px box) plus the
// `-right-6` offset that keeps the box centered on the sidebar edge (the
// previous `w-4`/`-right-4` pair produced a 16px hit area). The keyboard
// alternative (Ctrl/Cmd+B) must stay intact because the rail itself is
// tabIndex=-1.

function renderDesktopSidebar() {
  return render(
    <SidebarProvider>
      <Sidebar side='left' variant='inset' collapsible='icon'>
        <SidebarRail />
      </Sidebar>
    </SidebarProvider>
  )
}

function getRail(): HTMLElement {
  return screen.getByLabelText('Toggle Sidebar')
}

afterEach(() => cleanup())

describe('SidebarRail hit area', () => {
  it('uses a 24px (w-6) hit box', () => {
    renderDesktopSidebar()

    expect(getRail().classList).toContain('w-6')
    expect(getRail().classList).not.toContain('w-4')
  })

  it('keeps the wider box centered on the left sidebar edge', () => {
    renderDesktopSidebar()

    expect(getRail().classList).toContain('group-data-[side=left]:-right-6')
    expect(getRail().classList).not.toContain('group-data-[side=left]:-right-4')
  })

  it('toggles the sidebar when clicked', () => {
    renderDesktopSidebar()

    const sidebarRoot = document.querySelector('[data-slot="sidebar"]')
    expect(sidebarRoot).not.toBeNull()
    expect(sidebarRoot!.getAttribute('data-state')).toBe('expanded')

    fireEvent.click(getRail())

    expect(sidebarRoot!.getAttribute('data-state')).toBe('collapsed')
  })
})

describe('SidebarRail keyboard alternative', () => {
  it('keeps the rail out of the tab order (tabIndex=-1)', () => {
    renderDesktopSidebar()

    expect(getRail()).toHaveAttribute('tabindex', '-1')
  })

  it('still toggles via the Ctrl/Cmd+B shortcut', () => {
    renderDesktopSidebar()

    const sidebarRoot = document.querySelector('[data-slot="sidebar"]')
    expect(sidebarRoot!.getAttribute('data-state')).toBe('expanded')

    fireEvent.keyDown(document.body, { key: 'b', ctrlKey: true })

    expect(sidebarRoot!.getAttribute('data-state')).toBe('collapsed')
  })
})
