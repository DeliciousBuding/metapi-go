// Behavior tests for the StatusBadge copy feedback loop (#889): the badge
// maintained a `copied` flag but never rendered it, and its tooltip was a
// hardcoded English string. Clicking a copyable badge must now (a) write the
// clipboard, (b) render the copied check + "Copied" title, (c) revert after
// the flash window, and (d) toast when the clipboard rejects.

import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { StatusBadge } from '../core/status-badge'

const { mockToastError } = vi.hoisted(() => ({
  mockToastError: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    error: mockToastError,
    success: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
}))

const writeTextMock = vi.fn()

beforeEach(() => {
  vi.useFakeTimers()
  mockToastError.mockReset()
  writeTextMock.mockReset()
  writeTextMock.mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    writable: true,
    configurable: true,
    value: { writeText: writeTextMock },
  })
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('StatusBadge copy feedback', () => {
  it('copies the label, renders the copied check and reverts after 1.5s', async () => {
    render((<StatusBadge label='gpt-4o' />) as ReactElement)

    const badge = screen
      .getByText('gpt-4o')
      .closest('[data-slot="status-badge"]')
    if (!badge) throw new Error('badge root not rendered')

    // Before the click the hint title embeds the value (i18n, not hardcoded).
    expect(badge).toHaveAttribute('title', 'Click to copy: gpt-4o')

    fireEvent.click(badge)

    expect(writeTextMock).toHaveBeenCalledWith('gpt-4o')
    // The copied state surfaces: check icon + "Copied" title.
    await act(async () => {})
    expect(badge).toHaveAttribute('title', 'Copied')
    expect(badge.querySelector('svg')).not.toBeNull()

    // After the flash window the hint returns.
    act(() => {
      vi.advanceTimersByTime(1600)
    })
    expect(badge).toHaveAttribute('title', 'Click to copy: gpt-4o')
  })

  it('toasts when the clipboard write is rejected', async () => {
    writeTextMock.mockRejectedValue(new Error('denied'))
    render((<StatusBadge label='sk-secret' />) as ReactElement)

    const badge = screen
      .getByText('sk-secret')
      .closest('[data-slot="status-badge"]')
    if (!badge) throw new Error('badge root not rendered')

    fireEvent.click(badge)
    await act(async () => {})

    expect(mockToastError).toHaveBeenCalledWith('Copy failed')
    expect(badge).not.toHaveAttribute('title', 'Copied')
  })

  it('non-copyable badges keep a plain title and never hit the clipboard', () => {
    render((<StatusBadge label='static' copyable={false} />) as ReactElement)

    const badge = screen
      .getByText('static')
      .closest('[data-slot="status-badge"]')
    if (!badge) throw new Error('badge root not rendered')

    expect(badge).toHaveAttribute('title', 'static')
    fireEvent.click(badge)
    expect(writeTextMock).not.toHaveBeenCalled()
  })
})
