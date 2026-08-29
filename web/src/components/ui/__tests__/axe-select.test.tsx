// Component-level axe gate for the Select primitive: a named select must
// produce zero structural violations closed and open — the trigger is a
// named combobox, the popup exposes a single listbox, and every item is an
// option with selectable state (jsdom skips color-contrast — that stays
// with the browser-level a11y-scan gate).
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import axe from 'axe-core'
import { afterEach, describe, expect, it } from 'vitest'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import '@/i18n/config'

function renderSelect() {
  return render(
    <Select defaultValue='postgres'>
      <SelectTrigger aria-label='Database dialect'>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value='sqlite'>SQLite</SelectItem>
        <SelectItem value='postgres'>PostgreSQL</SelectItem>
      </SelectContent>
    </Select>
  )
}

async function openListbox() {
  fireEvent.click(screen.getByRole('combobox', { name: 'Database dialect' }))
  await screen.findByRole('listbox')
}

afterEach(() => cleanup())

describe('Select axe gate', () => {
  it('closed select produces zero axe violations', async () => {
    const { container } = renderSelect()

    const results = await axe.run(container)
    expect(results.violations).toEqual([])
  })

  it('open select produces zero axe violations', async () => {
    const { container } = renderSelect()
    await openListbox()

    const results = await axe.run(container)
    expect(results.violations).toEqual([])
  })

  it('exposes combobox/listbox semantics with a selected option', async () => {
    renderSelect()

    const trigger = screen.getByRole('combobox', { name: 'Database dialect' })
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')

    await openListbox()

    const options = screen.getAllByRole('option')
    expect(options).toHaveLength(2)
    const selected = options.filter(
      (option) => option.getAttribute('aria-selected') === 'true'
    )
    expect(selected).toHaveLength(1)
    expect(selected[0]).toHaveTextContent('PostgreSQL')
  })
})
