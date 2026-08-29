// ChartContainer must expose charts as named figures so screen readers can
// skip to them instead of walking raw SVG paths. Pins the accessible-name
// precedence: explicit aria-label > HTML title > first string series label;
// with none of the three the figure stays unnamed (role is still present).
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { ChartContainer, type ChartConfig } from '@/components/ui/chart'

// recharts' ResponsiveContainer instantiates a ResizeObserver to measure the
// wrapper; jsdom ships none, so stub the constructor (no observation needed —
// the assertions target the wrapper div, not the rendered chart children).
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver =
  ResizeObserverStub as unknown as typeof ResizeObserver

function renderChart(config: ChartConfig, props: Record<string, unknown> = {}) {
  return render(
    <ChartContainer config={config} {...props}>
      <div data-testid='probe' />
    </ChartContainer>
  )
}

afterEach(() => cleanup())

describe('ChartContainer figure semantics', () => {
  it('renders role=figure', () => {
    const { container } = renderChart({ series: { label: 'Calls' } })

    expect(container.querySelector('[data-slot="chart"]')).toHaveAttribute(
      'role',
      'figure'
    )
  })

  it('uses an explicit aria-label when provided', () => {
    renderChart(
      { series: { label: 'Calls' } },
      { 'aria-label': 'Call volume by day' }
    )

    expect(
      screen.getByRole('figure', { name: 'Call volume by day' })
    ).toBeInTheDocument()
  })

  it('falls back to the HTML title', () => {
    renderChart({ series: { label: 'Calls' } }, { title: 'Site traffic' })

    expect(
      screen.getByRole('figure', { name: 'Site traffic' })
    ).toBeInTheDocument()
  })

  it('falls back to the first string series label from config', () => {
    renderChart({
      income: { label: 'Income' },
      outcome: { label: 'Outcome' },
    })

    expect(screen.getByRole('figure', { name: 'Income' })).toBeInTheDocument()
  })

  it('stays unnamed when no label source exists', () => {
    renderChart({ count: { label: '' } })

    const figure = screen.getByRole('figure')
    expect(figure).not.toHaveAttribute('aria-label')
    expect(figure).not.toHaveAttribute('title')
  })

  it('stamps the runtime color style with the CSP nonce meta value', () => {
    const nonce = 'test-nonce-1234567890'
    const meta = document.createElement('meta')
    meta.name = 'csp-nonce'
    meta.content = nonce
    document.head.appendChild(meta)
    try {
      const { container } = renderChart({
        series: { label: 'Calls', color: 'var(--chart-1)' },
      })

      const style =
        container.querySelector('style') ?? document.querySelector('style')
      expect(style).toBeTruthy()
      // Chromium hides reflected getAttribute() for nonce after insertion for
      // security, but the IDL nonce property must carry the CSP value; React
      // passes the prop through in exactly this way.
      expect(style).toHaveProperty('nonce', nonce)
      expect(style?.textContent).toContain('--color-series')
    } finally {
      meta.remove()
    }
  })
})
