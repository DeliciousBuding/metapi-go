import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { TimingCell } from '../timing-cell'

afterEach(cleanup)
describe('proxy-attempt timing', () => {
  it('does not mark a slow first output green just because headers arrived quickly', () => {
    render(
      <TimingCell
        isStream
        firstByteLatencyMs={100}
        firstOutputLatencyMs={6000}
        latencyMs={9000}
      />
    )
    expect(screen.getByRole('group')).toHaveClass('border-warning')
    expect(screen.getByRole('group')).not.toHaveClass('border-success')
  })
  it('keeps unknown stream output neutral even with fast headers', () => {
    render(<TimingCell isStream firstByteLatencyMs={100} latencyMs={9000} />)
    expect(screen.getByRole('group')).toHaveClass('border-muted-foreground/30')
  })
  it('keeps generated output separate from response headers for streams', () => {
    render(
      <TimingCell
        isStream
        firstOutputLatencyMs={2400}
        firstByteLatencyMs={1250}
        latencyMs={6500}
      />
    )
    expect(screen.getByText('2.40 s')).toBeInTheDocument()
    expect(screen.getByText('1.25 s')).toBeInTheDocument()
    expect(screen.getByText('6.50 s')).toBeInTheDocument()
  })
  it('does not infer historical first output from response headers', () => {
    render(<TimingCell isStream firstByteLatencyMs={1250} latencyMs={6500} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })
  it('rejects output before headers or after the completed attempt', () => {
    render(
      <TimingCell
        isStream
        firstOutputLatencyMs={100}
        firstByteLatencyMs={1250}
        latencyMs={6500}
      />
    )
    expect(screen.getByText('—')).toBeInTheDocument()
  })
  it('shows first byte and total as separate readable values', () => {
    render(<TimingCell firstByteLatencyMs={1250} latencyMs={6500} />)
    expect(screen.getByText('1.25 s')).toBeInTheDocument()
    expect(screen.getByText('6.50 s')).toBeInTheDocument()
    expect(screen.getByRole('group')).toHaveAccessibleName(/1.25.*6.50/)
  })
  it('keeps a missing first byte unknown instead of copying the total', () => {
    render(<TimingCell latencyMs={6500} />)
    expect(screen.getByText('—')).toBeInTheDocument()
    expect(screen.getByText('6.50 s')).toBeInTheDocument()
  })
  it('does not clamp an inconsistent first byte into an invented measurement', () => {
    render(<TimingCell firstByteLatencyMs={9000} latencyMs={6500} />)
    expect(screen.getByText('—')).toBeInTheDocument()
    expect(screen.queryByText('9.00 s')).not.toBeInTheDocument()
  })
  it('does not render non-finite durations', () => {
    render(
      <TimingCell
        firstByteLatencyMs={Number.NaN}
        latencyMs={Number.POSITIVE_INFINITY}
      />
    )
    expect(screen.getAllByText('—')).toHaveLength(2)
  })
})
