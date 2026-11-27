// metapi-go/components/common — probe health bar states.
// Covers the honest empty state, the pending placeholder, chronological bar
// ordering with status colors, the success-rate/latency tooltip and the
// screen-reader summary.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import i18n from 'i18next'
import { afterEach, describe, expect, it } from 'vitest'

import {
  ProbeHealthBar,
  summarizeProbeResults,
  type ProbeResult,
} from '../probe-health-bar'

afterEach(() => cleanup())

let nextProbeId = 1

function probe(
  status: ProbeResult['status'],
  latencyMs: number | null = null
): ProbeResult {
  return {
    id: nextProbeId++,
    status,
    latencyMs,
    httpStatus: null,
    errorText: null,
    modelName: 'gpt-5.5',
    createdAt: '2026-08-28T02:00:00Z',
  }
}

function bars(container: HTMLElement): HTMLElement[] {
  const trigger = container.querySelector('[aria-label]')
  if (!trigger) throw new Error('health bar trigger missing')
  return [...trigger.children] as HTMLElement[]
}

describe('ProbeHealthBar states', () => {
  it('renders the pending placeholder while the batch query has not settled', () => {
    const { container } = render(<ProbeHealthBar results={undefined} pending />)
    const placeholder = container.querySelector('[aria-hidden="true"]')
    expect(placeholder).not.toBeNull()
    expect(placeholder?.textContent).toBe('—')
    expect(container.textContent).not.toContain(i18n.t('probeHealth.empty'))
  })

  it('renders the honest "No probes" empty state when history is absent', () => {
    const { container } = render(<ProbeHealthBar results={undefined} />)
    expect(container.textContent).toBe(i18n.t('probeHealth.empty'))
  })

  it('treats an empty results array the same as absent history', () => {
    const { container } = render(<ProbeHealthBar results={[]} />)
    expect(container.textContent).toBe(i18n.t('probeHealth.empty'))
  })

  it('renders one bar per real probe without fake placeholder slots, chronological left-to-right with status colors and height cues', () => {
    // API order is newest-first: success (newest), failure, inconclusive,
    // skipped (oldest). Display order must be oldest → newest.
    const results = [
      probe('success', 90),
      probe('failure'),
      probe('inconclusive'),
      probe('skipped'),
    ]
    const { container } = render(<ProbeHealthBar results={results} />)
    const renderedBars = bars(container)
    const classes = renderedBars.map((bar) => bar.className)
    expect(classes).toHaveLength(4)
    expect(classes[0]).toContain('bg-muted-foreground') // skipped (oldest)
    expect(classes[0]).toContain('h-1/2')
    expect(renderedBars[0]).toHaveAttribute('data-probe-status', 'skipped')
    expect(classes[1]).toContain('bg-warning') // inconclusive
    expect(classes[1]).toContain('h-3/4')
    expect(renderedBars[1]).toHaveAttribute('data-probe-status', 'inconclusive')
    expect(classes[2]).toContain('bg-destructive') // failure
    expect(classes[2]).toContain('h-full')
    expect(renderedBars[2]).toHaveAttribute('data-probe-status', 'failure')
    expect(classes[3]).toContain('bg-success') // success (newest)
    expect(classes[3]).toContain('h-full')
    expect(renderedBars[3]).toHaveAttribute('data-probe-status', 'success')
  })

  it('never renders fake gray placeholder bars when results are empty', () => {
    const { container } = render(<ProbeHealthBar results={[]} />)
    expect(container.querySelector('[data-probe-status]')).toBeNull()
    expect(container.querySelector('[data-probe-count]')).toBeNull()
    expect(container.textContent).toBe(i18n.t('probeHealth.empty'))
  })

  it('exposes a screen-reader summary with success and total counts', () => {
    const results = [
      probe('success', 100),
      probe('failure'),
      probe('success', 200),
    ]
    const { container } = render(<ProbeHealthBar results={results} />)
    const trigger = container.querySelector('[aria-label]')
    expect(trigger?.getAttribute('aria-label')).toBe(
      i18n.t('probeHealth.ariaSummary', { success: 2, total: 3 })
    )
  })

  it('shows success rate and average latency in the tooltip on focus', async () => {
    const results = [
      probe('success', 100),
      probe('failure'),
      probe('success', 200),
    ]
    const { container } = render(<ProbeHealthBar results={results} />)
    const trigger = container.querySelector('[aria-label]')
    expect(trigger).not.toBeNull()
    fireEvent.focus(trigger as HTMLElement)

    await waitFor(
      () => {
        const text = document.body.textContent ?? ''
        expect(text).toContain(
          i18n.t('probeHealth.tooltip.successRate', {
            rate: 66.7,
            success: 2,
            total: 3,
          })
        )
        expect(text).toContain(
          i18n.t('probeHealth.tooltip.window', { total: 3 })
        )
      },
      { timeout: 3000 }
    )
  })

  it('falls back to the no-latency tooltip line when no probe recorded latency', async () => {
    const results = [probe('failure'), probe('inconclusive')]
    const { container } = render(<ProbeHealthBar results={results} />)
    fireEvent.focus(container.querySelector('[aria-label]') as HTMLElement)

    await waitFor(
      () => {
        expect(document.body.textContent ?? '').toContain(
          i18n.t('probeHealth.tooltip.noLatency')
        )
      },
      { timeout: 3000 }
    )
  })
})

describe('summarizeProbeResults', () => {
  it('computes success rate with one decimal and averages only recorded latencies', () => {
    const summary = summarizeProbeResults([
      probe('success', 100),
      probe('failure'),
      probe('success', 200),
    ])
    expect(summary.total).toBe(3)
    expect(summary.success).toBe(2)
    expect(summary.successRatePct).toBe(66.7)
    expect(summary.avgLatencyMs).toBe(150)
  })

  it('reports zero rate and null latency for an empty window', () => {
    const summary = summarizeProbeResults([])
    expect(summary).toEqual({
      total: 0,
      success: 0,
      failure: 0,
      inconclusive: 0,
      skipped: 0,
      successRatePct: 0,
      avgLatencyMs: null,
      latestCreatedAt: null,
      latestLatencyMs: null,
      latestStatus: null,
    })
  })

  it('counts 100% success and records latest sample metadata when every probe succeeded', () => {
    const summary = summarizeProbeResults([
      probe('success', 50),
      probe('success', 70),
    ])
    expect(summary.successRatePct).toBe(100)
    expect(summary.avgLatencyMs).toBe(60)
    expect(summary.latestLatencyMs).toBe(50)
    expect(summary.latestStatus).toBe('success')
    expect(summary.latestCreatedAt).toBe('2026-08-28T02:00:00Z')
  })
})
