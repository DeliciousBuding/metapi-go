import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  isValidMapObject,
  parseStringMap,
  serializeStringMap,
} from '@/lib/helpers/string-map'

import { StringMapEditor } from '../string-map-editor'

afterEach(cleanup)
function Editor({ initial = '' }: { initial?: string }) {
  const [value, onChange] = useState(initial)
  return <StringMapEditor value={value} onChange={onChange} />
}
describe('string map editor', () => {
  it('round trips escapes, whitespace, Unicode and prototype keys', () => {
    const entries = [
      { key: '__proto__', value: '你好\n"\\' },
      { key: ' a ', value: '' },
    ]
    expect(parseStringMap(serializeStringMap(entries))).toEqual(entries)
  })
  it('preserves duplicate pairs and rejects their submission', () => {
    const raw = '{"a":"first","a":"second"}'
    expect(parseStringMap(raw)).toHaveLength(2)
    expect(isValidMapObject(raw)).toBe(false)
  })
  it('keeps unsupported legacy objects verbatim and permits existing object contracts', () => {
    const raw = '{ "count": 2, "flag": true, "nested": {"x":1} }'
    render(<Editor initial={raw} />)
    expect(screen.getByRole('textbox')).toHaveValue(raw)
    expect(screen.getByRole('button', { name: 'Edit rows' })).toBeDisabled()
    expect(isValidMapObject(raw)).toBe(true)
  })
  it('retains invalid JSON and never switches it into lossy rows', () => {
    render(<Editor initial='{"a":' />)
    expect(screen.getByRole('textbox')).toHaveValue('{"a":')
    expect(screen.getByRole('button', { name: 'Edit rows' })).toBeDisabled()
    expect(isValidMapObject('{"a":')).toBe(false)
  })
  it('retains empty and duplicate row drafts across JSON mode switches', () => {
    render(<Editor initial='{"a":"one"}' />)
    fireEvent.click(screen.getByRole('button', { name: 'Add entry' }))
    expect(screen.getByText('Enter a key for this entry.')).toBeVisible()
    fireEvent.change(screen.getAllByLabelText('Key')[1], {
      target: { value: 'a' },
    })
    fireEvent.change(screen.getAllByLabelText('Value')[1], {
      target: { value: 'two' },
    })
    expect(
      screen.getAllByText('This key is already used. Each key must be unique.')
    ).toHaveLength(2)
    fireEvent.click(screen.getByRole('button', { name: 'Edit JSON' }))
    expect(screen.getByRole('textbox')).toHaveValue(
      '{\n  "a": "one",\n  "a": "two"\n}'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Edit rows' }))
    expect(screen.getAllByLabelText('Value')[1]).toHaveValue('two')
    fireEvent.click(screen.getByRole('button', { name: 'Remove entry 1' }))
    expect(screen.getByLabelText('Value')).toHaveValue('two')
  })
  it('forwards field identity and exposes native keyboard-focusable actions without submitting', () => {
    const submit = vi.fn()
    render(
      <form onSubmit={submit}>
        <StringMapEditor
          value=''
          onChange={vi.fn()}
          id='field'
          aria-invalid={false}
          aria-describedby='help'
        />
      </form>
    )
    const add = screen.getByRole('button', { name: 'Add entry' })
    add.focus()
    expect(add).toHaveFocus()
    expect(add).toHaveAttribute('id', 'field')
    expect(add).toHaveAttribute('aria-describedby', 'help')
    fireEvent.keyDown(add, { key: 'Enter' })
    fireEvent.click(add)
    expect(submit).not.toHaveBeenCalled()
  })
})
