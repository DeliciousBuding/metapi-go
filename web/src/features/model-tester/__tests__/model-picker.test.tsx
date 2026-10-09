import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ModelPicker } from '@/components/common/model-picker'
import i18n from '@/i18n/config'

const models = [
  { name: 'gpt-4o' },
  { name: 'gpt-4o-mini' },
  { name: 'claude-sonnet-4' },
]
function Picker() {
  const [value, setValue] = useState('gpt-4o')
  return (
    <ModelPicker
      aria-label='Model'
      aria-describedby='model-help'
      id='model-input'
      models={models}
      value={value}
      onValueChange={setValue}
    />
  )
}
beforeEach(async () => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
  Element.prototype.scrollIntoView = vi.fn()

  await i18n.changeLanguage('en')
})
afterEach(cleanup)
describe('searchable model picker', () => {
  it('forwards form semantics and keeps the selected model visible', () => {
    render(<Picker />)
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveAttribute(
      'id',
      'model-input'
    )
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveAttribute(
      'aria-describedby',
      'model-help'
    )
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveTextContent(
      'gpt-4o'
    )
  })
  it('filters by provider and selects with the keyboard', async () => {
    render(<Picker />)
    fireEvent.click(screen.getByRole('combobox', { name: 'Model' }))
    const search = await screen.findByRole('combobox', {
      name: 'Search models or providers…',
    })
    fireEvent.change(search, { target: { value: 'Anthropic' } })
    await waitFor(() => expect(screen.getAllByRole('option')).toHaveLength(1))
    fireEvent.keyDown(search, { key: 'Enter' })
    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'Model' })).toHaveTextContent(
        'claude-sonnet-4'
      )
    )
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })
  it('explains an empty search without changing the selected model', async () => {
    render(<Picker />)
    fireEvent.click(screen.getByRole('combobox', { name: 'Model' }))
    fireEvent.change(
      await screen.findByRole('combobox', {
        name: 'Search models or providers…',
      }),
      { target: { value: 'missing-model' } }
    )
    expect(
      await screen.findByText(
        'No matching models. Try another model or provider.'
      )
    ).toBeInTheDocument()
    expect(screen.queryAllByRole('option')).toHaveLength(0)
  })
})
