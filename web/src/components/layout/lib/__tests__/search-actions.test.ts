// metapi-go/layout — command palette action registry tests.
// Covers the registry contract (stable ids, bilingual title coverage,
// keyword discipline) and the local matcher the palette uses while typing.

import { describe, expect, it } from 'vitest'

import en from '../../../../i18n/locales/en.json'
import zhCN from '../../../../i18n/locales/zh-CN.json'
import {
  matchActionEntries,
  resolveSmartLogQuery,
  SEARCH_ACTION_ENTRIES,
  SEARCH_SHORTCUT_ENTRIES,
  type SearchActionEntry,
} from '../search-actions'

type TranslationNode = Record<string, unknown>

function resolveKey(root: TranslationNode, dottedKey: string): unknown {
  let current: unknown = root
  for (const segment of dottedKey.split('.')) {
    if (current === null || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[segment]
  }
  return current
}

describe('SEARCH_ACTION_ENTRIES registry', () => {
  it('registers exactly the audited high-frequency actions with stable ids', () => {
    expect(SEARCH_ACTION_ENTRIES.map((entry) => entry.id)).toEqual([
      'add-site',
      'run-checkin-all',
      'rebuild-routes',
      'refresh-route-decisions',
    ])
  })

  it('uses unique ids, action-namespaced title keys and an icon per entry', () => {
    const ids = SEARCH_ACTION_ENTRIES.map((entry) => entry.id)
    expect(new Set(ids).size).toBe(ids.length)
    for (const entry of SEARCH_ACTION_ENTRIES) {
      expect(entry.titleKey).toMatch(/^search\.actions\./)
      expect(entry.icon).toBeDefined()
      expect(entry.keywords.length).toBeGreaterThan(0)
    }
  })

  it('resolves every title key to a non-empty string in both locales', () => {
    for (const entry of SEARCH_ACTION_ENTRIES) {
      const enLabel = resolveKey(en.translation, entry.titleKey)
      const zhLabel = resolveKey(zhCN.translation, entry.titleKey)
      expect(typeof enLabel === 'string' && enLabel.length > 0).toBe(true)
      expect(typeof zhLabel === 'string' && zhLabel.length > 0).toBe(true)
    }
  })
})

describe('matchActionEntries', () => {
  const labels: Record<string, string> = {
    'search.actions.addSite': 'Add site',
    'search.actions.runCheckinAll': 'Run all check-ins',
    'search.actions.rebuildRoutes': 'Auto-rebuild routes',
    'search.actions.refreshRouteDecisions': 'Refresh route decisions',
  }
  const resolveLabel = (key: string) => labels[key] ?? key

  it('returns every entry in registry order for an empty query', () => {
    expect(
      matchActionEntries(SEARCH_ACTION_ENTRIES, '  ', resolveLabel)
    ).toEqual([...SEARCH_ACTION_ENTRIES])
  })

  it('matches translated labels case-insensitively', () => {
    const matched = matchActionEntries(
      SEARCH_ACTION_ENTRIES,
      'REBUILD',
      resolveLabel
    )
    expect(matched.map((entry) => entry.id)).toContain('rebuild-routes')
  })

  it('matches bilingual keywords the label alone does not contain', () => {
    // "签到" does not appear in the English label "Run all check-ins".
    const byZhKeyword = matchActionEntries(
      SEARCH_ACTION_ENTRIES,
      '签到',
      resolveLabel
    )
    expect(byZhKeyword.map((entry) => entry.id)).toContain('run-checkin-all')

    // "new site" is a synonym of the "Add site" label.
    const bySynonym = matchActionEntries(
      SEARCH_ACTION_ENTRIES,
      'new site',
      resolveLabel
    )
    expect(bySynonym.map((entry) => entry.id)).toContain('add-site')
  })

  it('ranks starts-with matches before contains matches', () => {
    const entries: SearchActionEntry[] = [
      {
        id: 'rebuild-routes',
        titleKey: 'contains.only',
        keywords: ['route rebuild special'],
        icon: SEARCH_ACTION_ENTRIES[0].icon,
      },
      {
        id: 'add-site',
        titleKey: 'starts.with',
        keywords: ['rebuild'],
        icon: SEARCH_ACTION_ENTRIES[0].icon,
      },
    ]
    const matched = matchActionEntries(entries, 'rebuild', (key) =>
      key === 'starts.with' ? 'Something else' : 'Another thing'
    )
    expect(matched.map((entry) => entry.id)).toEqual([
      'add-site',
      'rebuild-routes',
    ])
  })

  it('returns no entries when the query matches nothing', () => {
    expect(
      matchActionEntries(SEARCH_ACTION_ENTRIES, 'zzz', resolveLabel)
    ).toEqual([])
  })
})

describe('SEARCH_SHORTCUT_ENTRIES & resolveSmartLogQuery', () => {
  it('exposes the four operational shortcuts with valid bilingual i18n keys', () => {
    expect(SEARCH_SHORTCUT_ENTRIES.map((entry) => entry.id)).toEqual([
      'route-workspace',
      'failed-proxy-logs',
      'model-tester',
      'price-compare',
    ])
    for (const entry of SEARCH_SHORTCUT_ENTRIES) {
      const enLabel = resolveKey(en.translation, entry.titleKey)
      const zhLabel = resolveKey(zhCN.translation, entry.titleKey)
      expect(typeof enLabel === 'string' && enLabel.length > 0).toBe(true)
      expect(typeof zhLabel === 'string' && zhLabel.length > 0).toBe(true)
    }
  })

  it('parses 4xx/5xx HTTP status codes into failed proxy-log deep links', () => {
    expect(resolveSmartLogQuery('429')).toEqual({
      kind: 'http-status',
      value: '429',
      to: '/proxy-logs',
      search: { view: 'attempts', q: '429', status: 'failed' },
    })
    expect(resolveSmartLogQuery('503')).toEqual({
      kind: 'http-status',
      value: '503',
      to: '/proxy-logs',
      search: { view: 'attempts', q: '503', status: 'failed' },
    })
    expect(resolveSmartLogQuery('200')).toEqual({
      kind: 'http-status',
      value: '200',
      to: '/proxy-logs',
      search: { view: 'attempts', q: '200' },
    })
  })

  it('parses req_ prefixed request IDs into direct request-view deep links', () => {
    expect(resolveSmartLogQuery('req_abc12345')).toEqual({
      kind: 'request-id',
      value: 'req_abc12345',
      to: '/proxy-logs',
      search: { view: 'requests', requestId: 'req_abc12345' },
    })
  })

  it('ignores non-diagnostic free text queries', () => {
    expect(resolveSmartLogQuery('')).toBeNull()
    expect(resolveSmartLogQuery('99')).toBeNull()
    expect(resolveSmartLogQuery('999')).toBeNull()
    expect(resolveSmartLogQuery('req_')).toBeNull()
    expect(resolveSmartLogQuery('gpt-5')).toBeNull()
  })
})
