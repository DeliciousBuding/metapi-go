// Unit tests for the shared USD formatter added to lib/format (used by the
// sites balance column/detail sheet and the accounts detail quota field).

import { describe, expect, it } from 'vitest'

import { formatUsd } from '../format'

describe('formatUsd', () => {
  it('formats whole and fractional dollar amounts with two decimals', () => {
    expect(formatUsd(1234.5)).toBe('$1234.50')
    expect(formatUsd(0.05)).toBe('$0.05')
    expect(formatUsd(100)).toBe('$100.00')
  })

  it('keeps a real zero balance honest instead of hiding it', () => {
    expect(formatUsd(0)).toBe('$0.00')
  })

  it('renders an em dash for null, undefined, and non-finite input', () => {
    expect(formatUsd(null)).toBe('—')
    expect(formatUsd(undefined)).toBe('—')
    expect(formatUsd(Number.NaN)).toBe('—')
    expect(formatUsd(Number.POSITIVE_INFINITY)).toBe('—')
  })
})
