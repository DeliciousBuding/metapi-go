// SidebarMenuButton aria-current contract: the active nav item must announce
// itself as the current page (aria-current="page") so assistive tech users can
// locate "you are here" without re-reading the whole menu; inactive items
// stay silent. Callers keep passing isActive only — no nav-group change.
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import {
  Sidebar,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from '@/components/ui/sidebar'

function renderMenuButton(isActive: boolean) {
  return render(
    <SidebarProvider>
      <Sidebar>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton isActive={isActive}>Dashboard</SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </Sidebar>
    </SidebarProvider>
  )
}

afterEach(() => cleanup())

describe('SidebarMenuButton aria-current', () => {
  it('announces the active item as the current page', () => {
    renderMenuButton(true)

    expect(screen.getByRole('button', { name: 'Dashboard' })).toHaveAttribute(
      'aria-current',
      'page'
    )
  })

  it('omits aria-current on inactive items', () => {
    renderMenuButton(false)

    expect(
      screen.getByRole('button', { name: 'Dashboard' })
    ).not.toHaveAttribute('aria-current')
  })

  it('keeps the active styling state independent of aria-current', () => {
    renderMenuButton(true)

    expect(screen.getByRole('button', { name: 'Dashboard' })).toHaveAttribute(
      'data-active'
    )
  })
})
