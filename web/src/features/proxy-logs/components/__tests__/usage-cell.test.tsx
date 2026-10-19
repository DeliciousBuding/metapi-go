import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import type { ProxyLog } from '../../types'
import { UsageCell } from '../usage-cell'

afterEach(cleanup)
describe('proxy log usage', () => {
  it('shows measured cache reads and writes without inventing a hit percentage', () => {
    const { container } = render(
      <UsageCell
        log={
          {
            promptTokens: 1200,
            completionTokens: 400,
            cacheReadTokens: 900,
            cacheCreationTokens: 20,
          } as ProxyLog
        }
      />
    )
    expect(container.textContent).toContain('1,200 / 400')
    expect(container.textContent).toContain('+900')
    // Cache writes stay measured even though only the read is on the dense
    // list line; the tooltip carries the full breakdown.
    const cell = container.firstElementChild as HTMLElement
    expect(cell.getAttribute('title')).toContain('900')
    expect(cell.getAttribute('title')).toContain('20')
    expect(screen.queryByText(/%/)).not.toBeInTheDocument()
  })
  it('shows input and output without counting a total twice', () => {
    const { container } = render(
      <UsageCell
        log={
          {
            promptTokens: 1200,
            completionTokens: 400,
            totalTokens: 1600,
          } as ProxyLog
        }
      />
    )
    expect(container.textContent).toContain('1,200 / 400')
    expect(screen.queryByText('1,600')).not.toBeInTheDocument()
  })
  it('distinguishes observed zero from unavailable usage', () => {
    const { container } = render(
      <UsageCell
        log={{ promptTokens: 0, completionTokens: null } as ProxyLog}
      />
    )
    expect(container.textContent).toContain('0 / —')
  })
  it('does not display invalid negative or non-finite counts', () => {
    const { container } = render(
      <UsageCell
        log={{ promptTokens: -1, completionTokens: Infinity } as ProxyLog}
      />
    )
    expect(container.textContent).toContain('— / —')
  })
})
