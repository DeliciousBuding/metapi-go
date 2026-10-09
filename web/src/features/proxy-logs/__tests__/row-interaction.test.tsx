import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  handleProxyLogRowKeyDown,
  isInteractiveRowTarget,
} from '../lib/row-interaction'

afterEach(cleanup)

describe('proxy log row keyboard interaction', () => {
  it('keeps nested disclosure Enter and Space from opening row details', () => {
    const onView = vi.fn()
    render(
      <div
        tabIndex={0}
        onClick={(event) => {
          if (!isInteractiveRowTarget(event.target)) onView()
        }}
        onKeyDown={(event) => handleProxyLogRowKeyDown(event, onView)}
      >
        <button
          type='button'
          aria-expanded='false'
          onClick={(event) =>
            event.currentTarget.setAttribute('aria-expanded', 'true')
          }
        >
          Show full text
        </button>
      </div>
    )

    const button = screen.getByRole('button', { name: 'Show full text' })
    button.focus()
    for (const key of ['Enter', ' ']) fireEvent.keyDown(button, { key })
    fireEvent.click(button)

    expect(isInteractiveRowTarget(button)).toBe(true)
    expect(button).toHaveAttribute('aria-expanded', 'true')
    expect(onView).not.toHaveBeenCalled()
  })

  it('opens details when Enter or Space originates from the focused row', () => {
    const onView = vi.fn()
    render(
      <div
        data-testid='row'
        tabIndex={0}
        onKeyDown={(event) => handleProxyLogRowKeyDown(event, onView)}
      />
    )

    const row = screen.getByTestId('row')
    row.focus()
    fireEvent.keyDown(row, { key: 'Enter' })
    fireEvent.keyDown(row, { key: ' ' })

    expect(isInteractiveRowTarget(row)).toBe(false)
    expect(onView).toHaveBeenCalledTimes(2)
  })
})
