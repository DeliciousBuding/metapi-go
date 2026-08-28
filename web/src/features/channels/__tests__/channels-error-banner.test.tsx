// Behavior tests for the channels error banner (competitor-study-2026-08
// P1-4): the "N failing" banner doubles as the one-click filter entry and
// flips into a clearable error-only indicator once the URL status facet is
// scoped to failing statuses.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ChannelsErrorBanner } from '../components/channels-error-banner'
import {
  CHANNELS_ERROR_STATUS_FILTER,
  isErrorOnlyStatusFilter,
} from '../lib/error-statuses'

afterEach(() => cleanup())

describe('isErrorOnlyStatusFilter', () => {
  it.each([
    ['', false],
    ['cooldown', true],
    ['breaker_open', true],
    ['cooldown,breaker_open', true],
    ['cooldown,enabled', false],
    ['enabled,manually_disabled', false],
  ])('classifies %j as %s', (input, expected) => {
    expect(isErrorOnlyStatusFilter(input)).toBe(expected)
  })
})

describe('ChannelsErrorBanner', () => {
  it('renders nothing without failing channels', () => {
    const { container } = render(
      <ChannelsErrorBanner
        errorCount={0}
        showErrorOnly={false}
        onFilterErrors={vi.fn()}
        onExitErrorOnly={vi.fn()}
      />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('offers the one-click filter when channels are failing', () => {
    const onFilterErrors = vi.fn()
    render(
      <ChannelsErrorBanner
        errorCount={3}
        showErrorOnly={false}
        onFilterErrors={onFilterErrors}
        onExitErrorOnly={vi.fn()}
      />
    )
    expect(
      screen.getByText('3 channels are failing (cooldown or circuit breaker).')
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show failing' }))
    expect(onFilterErrors).toHaveBeenCalledTimes(1)
  })

  it('switches to the clearable error-only indicator', () => {
    const onExitErrorOnly = vi.fn()
    render(
      <ChannelsErrorBanner
        errorCount={2}
        showErrorOnly
        onFilterErrors={vi.fn()}
        onExitErrorOnly={onExitErrorOnly}
      />
    )
    expect(
      screen.getByText('Showing failing channels only.')
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show all' }))
    expect(onExitErrorOnly).toHaveBeenCalledTimes(1)
  })

  it('exposes the canonical failing-status filter value', () => {
    expect(CHANNELS_ERROR_STATUS_FILTER).toBe('cooldown,breaker_open')
  })
})
