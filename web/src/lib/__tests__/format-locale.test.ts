// Locale-aware number formatting contract: the three numeric formatters take
// an optional BCP-47 locale (default: browser) so callers can pin grouping and
// decimal separators to the active i18n language instead of the viewer's OS
// locale. Without a locale the output must stay identical to the historic
// browser-default behavior.
import { describe, expect, it } from 'vitest'

import { formatCurrency, formatInt, formatPrice } from '../format'

describe('numeric formatters — optional locale', () => {
  it('formatInt groups by the given locale', () => {
    expect(formatInt(1234567, 'de-DE')).toBe('1.234.567')
    expect(formatInt(1234567, 'zh-CN')).toBe('1,234,567')
  })

  it('formatInt keeps browser-default grouping when locale is omitted', () => {
    expect(formatInt(1234567)).toBe('1,234,567')
    expect(formatInt(1234567, undefined)).toBe('1,234,567')
  })

  it('formatCurrency uses locale decimal/grouping separators', () => {
    expect(formatCurrency(1234.56, { locale: 'de-DE' })).toBe('$1.234,56')
    expect(formatCurrency(1234.56, { locale: 'zh-CN' })).toBe('$1,234.56')
  })

  it('formatCurrency locale combines with fractionDigits', () => {
    expect(
      formatCurrency(0.123456, { fractionDigits: 4, locale: 'de-DE' })
    ).toBe('$0,1235')
  })

  it('formatCurrency keeps the sign before the symbol in any locale', () => {
    expect(formatCurrency(-12.5, { locale: 'de-DE' })).toBe('-$12,50')
  })

  it('formatPrice applies locale at fixed precision', () => {
    expect(formatPrice(1234.5, { fractionDigits: 4, locale: 'de-DE' })).toBe(
      '$1.234,5000'
    )
    expect(formatPrice(2.5, { fractionDigits: 4, locale: 'zh-CN' })).toBe(
      '$2.5000'
    )
  })

  it('formatPrice adaptive tier is untouched by the locale', () => {
    expect(formatPrice(0.005, { locale: 'de-DE' })).toBe('$0.0050')
    expect(formatPrice(2.5, { locale: 'de-DE' })).toBe('$2.50')
  })
})
