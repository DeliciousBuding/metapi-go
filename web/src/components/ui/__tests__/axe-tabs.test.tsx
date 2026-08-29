// Component-level axe gate for the Tabs primitive: a tab group must produce
// zero structural violations — tabs are named inside a tablist, the active
// tab is announced as selected, and the visible panel is labelled by its
// tab (jsdom skips color-contrast — that stays with the browser-level
// a11y-scan gate).
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import axe from 'axe-core'
import { afterEach, describe, expect, it } from 'vitest'

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import '@/i18n/config'

function renderTabs() {
  return render(
    <Tabs defaultValue='overview'>
      <TabsList>
        <TabsTrigger value='overview'>Overview</TabsTrigger>
        <TabsTrigger value='billing'>Billing</TabsTrigger>
      </TabsList>
      <TabsContent value='overview'>Usage summary for this site.</TabsContent>
      <TabsContent value='billing'>Invoices and balances.</TabsContent>
    </Tabs>
  )
}

afterEach(() => cleanup())

describe('Tabs axe gate', () => {
  it('tab group produces zero axe violations', async () => {
    const { container } = renderTabs()

    const results = await axe.run(container)
    expect(results.violations).toEqual([])
  })

  it('ships tablist semantics with a selected tab and a labelled panel', () => {
    renderTabs()

    expect(screen.getByRole('tablist')).not.toBeNull()

    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(2)
    const selected = tabs.filter(
      (tab) => tab.getAttribute('aria-selected') === 'true'
    )
    expect(selected).toHaveLength(1)
    expect(selected[0]).toHaveTextContent('Overview')

    const panel = screen.getByRole('tabpanel')
    expect(panel).toHaveTextContent('Usage summary for this site.')
    const named =
      (panel.getAttribute('aria-labelledby') ?? '') !== '' ||
      (panel.getAttribute('aria-label') ?? '') !== ''
    expect(named).toBe(true)
  })
})
