import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SiteScopePicker } from '../site-scope-picker'

afterEach(cleanup)
it('locates selected and unavailable sites without replacing the saved allow-list', () => {
  const onChange = vi.fn()
  render(
    <SiteScopePicker
      value={[2, 99]}
      onChange={onChange}
      sites={[
        { id: 1, name: 'Alpha' },
        { id: 2, name: 'Beta' },
      ]}
    />
  )
  expect(screen.getByText('2 selected')).toBeInTheDocument()
  fireEvent.change(screen.getByRole('textbox', { name: 'Search options' }), {
    target: { value: 'Alpha' },
  })
  expect(
    screen.queryByRole('checkbox', { name: 'Beta' })
  ).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Locate selected' }))
  expect(screen.getByRole('checkbox', { name: 'Beta' })).toHaveFocus()
  expect(
    screen.getByRole('checkbox', { name: 'ID 99 (unresolved)' })
  ).toBeChecked()
  expect(
    screen.queryByRole('checkbox', { name: 'Alpha' })
  ).not.toBeInTheDocument()
  expect(onChange).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('checkbox', { name: 'ID 99 (unresolved)' }))
  expect(onChange).toHaveBeenCalledWith([2])
})
