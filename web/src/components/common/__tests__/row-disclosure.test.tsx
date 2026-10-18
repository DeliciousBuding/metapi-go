import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  RowDisclosureContent,
  RowDisclosureTrigger,
  rowDisclosureClick,
} from '../row-disclosure'

afterEach(cleanup)

describe('row disclosure', () => {
  it('activates the surface without hijacking nested controls', () => {
    const toggle = vi.fn()
    render(
      <div onClick={(event) => rowDisclosureClick(event, toggle)}>
        <span>Surface</span>
        <button type='button'>Action</button>
        <a href='#details'>Link</a>
        <input aria-label='Select' type='checkbox' />
        <span role='switch' tabIndex={0} aria-checked={false}>
          Switch
        </span>
        <span data-row-disclosure-ignore>Custom control</span>
      </div>
    )
    fireEvent.click(screen.getByText('Surface'))
    expect(toggle).toHaveBeenCalledTimes(1)
    for (const control of [
      screen.getByRole('button'),
      screen.getByRole('link'),
      screen.getByRole('checkbox'),
      screen.getByRole('switch'),
      screen.getByText('Custom control'),
    ]) {
      fireEvent.click(control)
    }
    expect(toggle).toHaveBeenCalledTimes(1)
  })

  it('keeps the trigger a named button linked to its expanded region', () => {
    const toggle = vi.fn()
    render(
      <>
        <RowDisclosureTrigger expanded controls='route-7' onClick={toggle}>
          Example model
        </RowDisclosureTrigger>
        <RowDisclosureContent id='route-7' label='Example model channels'>
          Channel details
        </RowDisclosureContent>
      </>
    )
    const trigger = screen.getByRole('button', { name: 'Example model' })
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(trigger).toHaveAttribute(
      'aria-controls',
      screen.getByRole('region').id
    )
    fireEvent.click(trigger)
    expect(toggle).toHaveBeenCalledOnce()
  })
})
