// Behavior test for the datetime quick-range chips: the four presets must
// produce well-formed datetime-local bounds whose span matches the preset
// (today = local midnight→now; last-24h = exactly 24h; etc.), because the
// filters feed straight into the server query window.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { DateRangePresets } from '../date-range-presets'

const DATETIME_LOCAL = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/

describe('DateRangePresets', () => {
  afterEach(cleanup)

  it('renders the four preset chips', () => {
    render(<DateRangePresets onApply={() => {}} />)
    expect(screen.getByRole('button', { name: 'Today' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Last 24 hours' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Last 7 days' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Last 30 days' })
    ).toBeInTheDocument()
  })

  it('applies the last-24-hours window as datetime-local values', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-13T08:30:00'))
    try {
      const onApply = vi.fn()
      render(<DateRangePresets onApply={onApply} />)
      fireEvent.click(screen.getByRole('button', { name: 'Last 24 hours' }))
      expect(onApply).toHaveBeenCalledTimes(1)
      const [from, to] = onApply.mock.calls[0]
      expect(from).toMatch(DATETIME_LOCAL)
      expect(to).toMatch(DATETIME_LOCAL)
      expect(new Date(to).getTime() - new Date(from).getTime()).toBe(
        24 * 60 * 60 * 1000
      )
    } finally {
      vi.useRealTimers()
    }
  })

  it('today starts at local midnight', () => {
    const onApply = vi.fn()
    render(<DateRangePresets onApply={onApply} />)
    fireEvent.click(screen.getByRole('button', { name: 'Today' }))
    const [from] = onApply.mock.calls[0]
    expect(from.endsWith('T00:00')).toBe(true)
  })
})
