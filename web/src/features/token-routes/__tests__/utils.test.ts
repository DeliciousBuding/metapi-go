import { describe, expect, it } from 'vitest'

import {
  ROUTE_BRAND_ICON_PREFIX,
  ROUTE_ICON_NONE_VALUE,
  dedupeChannelDrafts,
  formatContextLength,
  getModelPatternError,
  isExactModelPattern,
  isRegexModelPattern,
  matchesModelPattern,
  normalizeRouteDisplayIconValue,
  normalizeRouteMode,
  normalizeRoutingStrategy,
  parseRegexModelPattern,
  resolveRouteIcon,
  resolveRouteTitle,
  routingStrategyLabel,
} from '../utils'

// When the i18n runtime translation layer is active (the i18n.coverage suite
// loads it first in the full run), Chinese literals returned from helpers are
// replaced by i18n keys / English translations. Accept the literal (isolated
// run) or any transformed non-CJK string (full-suite run) so the assertion
// holds in both contexts.
const CJK_RANGE = /[㐀-鿿]/

function expectLocalized(
  actual: string | null | undefined,
  literal: string
): void {
  expect(
    actual === literal ||
      (typeof actual === 'string' && !CJK_RANGE.test(actual)),
    `expected "${literal}" or a transformed (non-CJK) string, got "${actual}"`
  ).toBe(true)
}

// ---------------------------------------------------------------------------
// Pattern grammar
// ---------------------------------------------------------------------------

describe('isRegexModelPattern', () => {
  it('returns true for a re: prefix', () => {
    expect(isRegexModelPattern('re:^gpt-4')).toBe(true)
  })

  it('trims before checking the prefix', () => {
    expect(isRegexModelPattern('  re:^gpt-4 ')).toBe(true)
  })

  it('returns false for plain names and empty input', () => {
    expect(isRegexModelPattern('gpt-4o')).toBe(false)
    expect(isRegexModelPattern('')).toBe(false)
    expect(isRegexModelPattern('   ')).toBe(false)
  })
})

describe('isExactModelPattern', () => {
  it('treats plain names as exact', () => {
    expect(isExactModelPattern('gpt-4o')).toBe(true)
    expect(isExactModelPattern('  gpt-4o  ')).toBe(true)
    expect(isExactModelPattern('claude-3-5-sonnet')).toBe(true)
  })

  it('rejects empty and regex patterns', () => {
    expect(isExactModelPattern('')).toBe(false)
    expect(isExactModelPattern('   ')).toBe(false)
    expect(isExactModelPattern('re:^gpt-4')).toBe(false)
  })

  it.each([
    ['wildcard', 'gpt*4o'],
    ['char class open', 'gpt[4o'],
    ['char class close', 'gpt]4o'],
    ['group open', 'gpt(4o)'],
    ['optional', 'gpt?4o'],
    ['brace open', 'gpt{2}'],
    ['brace close', 'gpt}2'],
    ['alternation', 'gpt|claude'],
    ['anchor start', '^gpt-4o'],
    ['anchor end', 'gpt-4o$'],
    ['escape', 'gpt\\d'],
  ])('rejects a string containing the %s metacharacter', (_label, value) => {
    expect(isExactModelPattern(value)).toBe(false)
  })
})

describe('parseRegexModelPattern', () => {
  it('returns null regex + null error for non-regex patterns', () => {
    expect(parseRegexModelPattern('gpt-4o')).toEqual({
      regex: null,
      error: null,
    })
    expect(parseRegexModelPattern('')).toEqual({ regex: null, error: null })
  })

  it('reports a dedicated message for re: with an empty body', () => {
    const first = parseRegexModelPattern('re:')
    expect(first.regex).toBeNull()
    expectLocalized(first.error, '正则体不能为空')

    const spaced = parseRegexModelPattern('  re:   ')
    expect(spaced.regex).toBeNull()
    expectLocalized(spaced.error, '正则体不能为空')
  })

  it('compiles a valid regex body and exposes its test() result', () => {
    const parsed = parseRegexModelPattern('re:^gpt-4')
    expect(parsed.error).toBeNull()
    expect(parsed.regex).not.toBeNull()
    expect(parsed.regex?.test('gpt-4o-mini')).toBe(true)
    expect(parsed.regex?.test('claude-3')).toBe(false)
  })

  it('returns the native RegExp error for an unparseable body', () => {
    const parsed = parseRegexModelPattern('re:(')
    expect(parsed.regex).toBeNull()
    expect(parsed.error).not.toBeNull()
    expect(typeof parsed.error).toBe('string')
    expect((parsed.error ?? '').length).toBeGreaterThan(0)
  })
})

describe('matchesModelPattern', () => {
  it('matches exact names case-sensitively', () => {
    expect(matchesModelPattern('gpt-4o', 'gpt-4o')).toBe(true)
    expect(matchesModelPattern('gpt-4o', 'gpt-4o-mini')).toBe(false)
    expect(matchesModelPattern('GPT-4O', 'gpt-4o')).toBe(false)
  })

  it('trims both arguments before matching', () => {
    expect(matchesModelPattern('  gpt-4o ', ' gpt-4o')).toBe(true)
  })

  it('uses regex test() for re: patterns', () => {
    expect(matchesModelPattern('gpt-4o', 're:^gpt-4')).toBe(true)
    expect(matchesModelPattern('claude-3', 're:^gpt-4')).toBe(false)
  })

  it('returns false when the regex body is invalid', () => {
    expect(matchesModelPattern('gpt-4o', 're:(')).toBe(false)
  })

  it('returns false when either argument is empty', () => {
    expect(matchesModelPattern('', 'gpt-4o')).toBe(false)
    expect(matchesModelPattern('gpt-4o', '')).toBe(false)
    expect(matchesModelPattern('gpt-4o', '   ')).toBe(false)
  })
})

describe('getModelPatternError', () => {
  it('returns null for empty and exact patterns', () => {
    expect(getModelPatternError('')).toBeNull()
    expect(getModelPatternError('gpt-4o')).toBeNull()
  })

  it('returns null for a valid regex pattern', () => {
    expect(getModelPatternError('re:^gpt-4')).toBeNull()
  })

  it('returns a prefixed error for an empty regex body', () => {
    expectLocalized(
      getModelPatternError('re:'),
      '模型匹配正则错误：正则体不能为空'
    )
  })

  it('returns a prefixed error for an unparseable regex', () => {
    const message = getModelPatternError('re:(')
    expect(message).not.toBeNull()
    // The Chinese prefix is translated under the i18n layer; assert a
    // non-empty error string either way.
    expect((message ?? '').length).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// Presentation: context length, route mode, routing strategy
// ---------------------------------------------------------------------------

describe('formatContextLength', () => {
  it('returns an empty string for falsy / non-positive input', () => {
    expect(formatContextLength(null)).toBe('')
    expect(formatContextLength(undefined)).toBe('')
    expect(formatContextLength(0)).toBe('')
    expect(formatContextLength(-1)).toBe('')
  })

  it('returns the raw number below 1k', () => {
    expect(formatContextLength(1)).toBe('1')
    expect(formatContextLength(999)).toBe('999')
  })

  it('formats thousands with a k suffix, trimming a .0 remainder', () => {
    expect(formatContextLength(1000)).toBe('1k')
    expect(formatContextLength(128000)).toBe('128k')
    expect(formatContextLength(123456)).toBe('123.5k')
  })

  it('formats millions with an M suffix, trimming a .0 remainder', () => {
    expect(formatContextLength(1_000_000)).toBe('1M')
    expect(formatContextLength(1_500_000)).toBe('1.5M')
  })
})

describe('normalizeRouteMode', () => {
  it('preserves the explicit_group mode', () => {
    expect(normalizeRouteMode('explicit_group')).toBe('explicit_group')
  })

  it('falls back to pattern for anything else', () => {
    expect(normalizeRouteMode('pattern')).toBe('pattern')
    expect(normalizeRouteMode(undefined)).toBe('pattern')
    expect(normalizeRouteMode(null)).toBe('pattern')
    expect(normalizeRouteMode('nonsense')).toBe('pattern')
  })
})

describe('normalizeRoutingStrategy', () => {
  it('preserves round_robin and stable_first', () => {
    expect(normalizeRoutingStrategy('round_robin')).toBe('round_robin')
    expect(normalizeRoutingStrategy('stable_first')).toBe('stable_first')
  })

  it('falls back to weighted for anything else', () => {
    expect(normalizeRoutingStrategy('weighted')).toBe('weighted')
    expect(normalizeRoutingStrategy(undefined)).toBe('weighted')
    expect(normalizeRoutingStrategy(null)).toBe('weighted')
    expect(normalizeRoutingStrategy('random')).toBe('weighted')
  })
})

describe('routingStrategyLabel', () => {
  it('returns the Chinese label for each canonical strategy', () => {
    expectLocalized(routingStrategyLabel('weighted'), '权重随机')
    expectLocalized(routingStrategyLabel('round_robin'), '轮询')
    expectLocalized(routingStrategyLabel('stable_first'), '稳定优先')
  })

  it('falls back to the weighted label for unknown input', () => {
    expectLocalized(routingStrategyLabel(undefined), '权重随机')
    expectLocalized(routingStrategyLabel('random'), '权重随机')
    // Fallback must equal the canonical weighted label in any environment.
    expect(routingStrategyLabel('random')).toBe(
      routingStrategyLabel('weighted')
    )
  })
})

// ---------------------------------------------------------------------------
// Route title + icon helpers
// ---------------------------------------------------------------------------

describe('resolveRouteTitle', () => {
  it('uses the trimmed displayName when present', () => {
    expect(
      resolveRouteTitle({ displayName: 'My Route', modelPattern: 'gpt-4o' })
    ).toBe('My Route')
    expect(
      resolveRouteTitle({ displayName: '  Spaced  ', modelPattern: 'gpt-4o' })
    ).toBe('Spaced')
  })

  it('falls back to modelPattern when displayName is blank', () => {
    expect(resolveRouteTitle({ displayName: '', modelPattern: 'gpt-4o' })).toBe(
      'gpt-4o'
    )
    expect(
      resolveRouteTitle({ displayName: '   ', modelPattern: 'gpt-4o' })
    ).toBe('gpt-4o')
  })
})

describe('resolveRouteIcon', () => {
  it('returns kind auto for empty / nullish input', () => {
    expect(resolveRouteIcon('')).toEqual({ kind: 'auto' })
    expect(resolveRouteIcon('   ')).toEqual({ kind: 'auto' })
    expect(resolveRouteIcon(null)).toEqual({ kind: 'auto' })
    expect(resolveRouteIcon(undefined)).toEqual({ kind: 'auto' })
  })

  it('returns kind none for the none sentinel', () => {
    expect(resolveRouteIcon(ROUTE_ICON_NONE_VALUE)).toEqual({ kind: 'none' })
    expect(resolveRouteIcon(`  ${ROUTE_ICON_NONE_VALUE} `)).toEqual({
      kind: 'none',
    })
  })

  it('extracts a brand icon when brand:<key> has a non-empty key', () => {
    expect(resolveRouteIcon(`${ROUTE_BRAND_ICON_PREFIX}openai`)).toEqual({
      kind: 'brand',
      value: 'openai',
    })
    expect(resolveRouteIcon(`${ROUTE_BRAND_ICON_PREFIX} anthropic `)).toEqual({
      kind: 'brand',
      value: 'anthropic',
    })
  })

  it('falls back to text for a brand: prefix with an empty key', () => {
    expect(resolveRouteIcon(ROUTE_BRAND_ICON_PREFIX)).toEqual({
      kind: 'text',
      value: ROUTE_BRAND_ICON_PREFIX,
    })
  })

  it('returns kind text for any other literal value', () => {
    expect(resolveRouteIcon('gpt-4o')).toEqual({
      kind: 'text',
      value: 'gpt-4o',
    })
  })
})

describe('normalizeRouteDisplayIconValue', () => {
  it('preserves the none sentinel and trims it', () => {
    expect(normalizeRouteDisplayIconValue(ROUTE_ICON_NONE_VALUE)).toBe(
      ROUTE_ICON_NONE_VALUE
    )
    expect(normalizeRouteDisplayIconValue(` ${ROUTE_ICON_NONE_VALUE} `)).toBe(
      ROUTE_ICON_NONE_VALUE
    )
  })

  it('returns the trimmed value for plain strings', () => {
    expect(normalizeRouteDisplayIconValue('  gpt-4o ')).toBe('gpt-4o')
  })

  it('returns an empty string for nullish input', () => {
    expect(normalizeRouteDisplayIconValue(null)).toBe('')
    expect(normalizeRouteDisplayIconValue(undefined)).toBe('')
  })
})

// ---------------------------------------------------------------------------
// Channel draft dedupe
// ---------------------------------------------------------------------------

describe('dedupeChannelDrafts', () => {
  it('collapses drafts sharing accountId + tokenId + sourceModel', () => {
    const drafts = [
      { accountId: 1, tokenId: 10, sourceModel: 'gpt-4o' },
      { accountId: 1, tokenId: 10, sourceModel: 'gpt-4o' },
      { accountId: 1, tokenId: 11, sourceModel: 'gpt-4o' },
    ]
    expect(dedupeChannelDrafts(drafts)).toEqual([
      { accountId: 1, tokenId: 10, sourceModel: 'gpt-4o' },
      { accountId: 1, tokenId: 11, sourceModel: 'gpt-4o' },
    ])
  })

  it('keeps drafts that differ only by sourceModel', () => {
    const drafts = [
      { accountId: 2, tokenId: 1, sourceModel: 'gpt-4o' },
      { accountId: 2, tokenId: 1, sourceModel: 'claude-3' },
    ]
    expect(dedupeChannelDrafts(drafts)).toHaveLength(2)
  })

  it('treats missing tokenId / sourceModel as 0 / empty for the dedupe key', () => {
    const drafts = [
      { accountId: 3 },
      { accountId: 3, tokenId: 0, sourceModel: '' },
    ]
    expect(dedupeChannelDrafts(drafts)).toEqual([
      { accountId: 3, tokenId: undefined, sourceModel: undefined },
    ])
  })

  it('normalises tokenId 0 and sourceModel "" to undefined in the output', () => {
    const result = dedupeChannelDrafts([
      { accountId: 4, tokenId: 0, sourceModel: '' },
    ])
    expect(result).toEqual([
      { accountId: 4, tokenId: undefined, sourceModel: undefined },
    ])
  })

  it('returns an empty array for empty input', () => {
    expect(dedupeChannelDrafts([])).toEqual([])
  })
})
