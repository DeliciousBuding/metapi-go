// Component-level axe gate for the Dialog primitive: an open dialog must
// name itself (aria-labelledby from the title), describe itself, and never
// emit a structural violation axe can detect without layout (jsdom skips
// color-contrast — that stays with the browser-level a11y-scan gate).
import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import axe from 'axe-core'
import { afterEach, describe, expect, it } from 'vitest'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import '@/i18n/config'

function renderOpenDialog() {
  return render(
    <Dialog open onOpenChange={() => {}}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Rename site</DialogTitle>
          <DialogDescription>
            Pick a new display name for this site.
          </DialogDescription>
        </DialogHeader>
        <Button>Save</Button>
        <DialogFooter />
      </DialogContent>
    </Dialog>
  )
}

afterEach(() => cleanup())

describe('Dialog axe gate', () => {
  it('open dialog produces zero axe violations', async () => {
    const { container } = renderOpenDialog()

    // Axe scans the whole document; the rendered dialog is the only content.
    const results = await axe.run(container)
    expect(results.violations).toEqual([])
  })

  it('dialog carries an accessible name from its title', () => {
    renderOpenDialog()

    const dialog = document.querySelector('[data-slot="dialog-content"]')
    expect(dialog).not.toBeNull()
    const labelledBy = dialog!.getAttribute('aria-labelledby') ?? ''
    const title = document.getElementById(labelledBy)
    expect(title).not.toBeNull()
    expect(title).toHaveTextContent('Rename site')
  })
})
