import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import type { ProxyLog } from '../../types'
import { UsageCell } from '../usage-cell'

afterEach(cleanup)
describe('proxy log usage', () => {
  it('shows input and output without counting a total twice', () => {
    render(
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
    expect(screen.getByText('1,200')).toBeInTheDocument()
    expect(screen.getByText('400')).toBeInTheDocument()
    expect(screen.queryByText('1,600')).not.toBeInTheDocument()
  })
  it('distinguishes observed zero from unavailable usage', () => {
    render(
      <UsageCell
        log={{ promptTokens: 0, completionTokens: null } as ProxyLog}
      />
    )
    expect(screen.getByText('0')).toBeInTheDocument()
    expect(screen.getByText('—')).toBeInTheDocument()
  })
  it('does not display invalid negative or non-finite counts', () => {
    render(
      <UsageCell
        log={{ promptTokens: -1, completionTokens: Infinity } as ProxyLog}
      />
    )
    expect(screen.getAllByText('—')).toHaveLength(2)
  })
})
