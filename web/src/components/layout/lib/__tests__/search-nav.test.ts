// metapi-go/layout — local search navigation index tests.
// Covers the page/settings entry builders and the local matcher used by
// the ⌘K palette's navigation layer.

import { describe, expect, it, vi } from 'vitest'

// The layout-owned settings registry needs a feature provider at runtime.
// In this isolated lib test we seed a five-subarea fixture so the palette
// navigation tests exercise the same declarative projection without creating
// a components -> features edge.
import {
  getSettingsNavEntries,
  matchNavEntries,
  pageEntriesFromNavGroups,
  type SearchNavEntry,
} from '../search-nav'

vi.mock('../settings-nav-registry', () => ({
  getSettingsSubareas: () => [
    {
      id: 'basic',
      title: 'Basic',
      basePath: '/settings/basic',
      defaultSection: 'site',
      getSectionNavItems: () => [
        { title: 'Site', url: '/settings/basic/site' },
        { title: 'Proxy', url: '/settings/basic/proxy' },
        { title: 'Models', url: '/settings/basic/models' },
        { title: 'Account', url: '/settings/basic/account' },
      ],
    },
    {
      id: 'downstream',
      title: 'Downstream',
      basePath: '/settings/downstream',
      defaultSection: 'keys',
      getSectionNavItems: () => [
        { title: 'Keys', url: '/settings/downstream/keys' },
        { title: 'Routes', url: '/settings/downstream/routes' },
        {
          title: 'Credential scope',
          url: '/settings/downstream/credential-scope',
        },
        { title: 'Calls', url: '/settings/downstream/calls' },
      ],
    },
    {
      id: 'operations',
      title: 'Operations',
      basePath: '/settings/operations',
      defaultSection: 'overview',
      getSectionNavItems: () => [
        { title: 'Overview', url: '/settings/operations/overview' },
        { title: 'Audit', url: '/settings/operations/audit' },
        {
          title: 'Scheduled Tasks',
          url: '/settings/operations/scheduled-tasks',
        },
        {
          title: 'Operational Events',
          url: '/settings/operations/program-logs',
        },
      ],
    },
    {
      id: 'content',
      title: 'Content',
      basePath: '/settings/content',
      defaultSection: 'import-export',
      getSectionNavItems: () => [
        { title: 'Import/export', url: '/settings/content/import-export' },
        { title: 'Backup', url: '/settings/content/backup' },
        { title: 'Branding', url: '/settings/content/branding' },
        { title: 'Announcements', url: '/settings/content/announcements' },
      ],
    },
    {
      id: 'system',
      title: 'System & Ops',
      basePath: '/settings/system',
      defaultSection: 'general',
      getSectionNavItems: () => [
        { title: 'General', url: '/settings/system/general' },
        { title: 'Security', url: '/settings/system/security' },
        { title: 'Database', url: '/settings/system/database' },
        { title: 'Notify', url: '/settings/system/notify' },
      ],
    },
  ],
}))
describe('pageEntriesFromNavGroups', () => {
  it('flattens link items across groups, keeping icons', () => {
    const icon = () => null
    const entries = pageEntriesFromNavGroups([
      {
        items: [
          { title: 'sidebar.items.sites', url: '/sites', icon },
          { title: 'sidebar.items.checkin', url: '/checkin' },
        ],
      },
      { items: [{ title: 'sidebar.items.about', url: '/about' }] },
    ])

    expect(entries.map((entry) => entry.url)).toEqual([
      '/sites',
      '/checkin',
      '/about',
    ])
    expect(entries[0]).toMatchObject({
      key: 'page-/sites',
      titleKey: 'sidebar.items.sites',
      scope: 'page',
      icon,
    })
    expect(entries[1].icon).toBeUndefined()
  })

  it('recurses collapsible items and skips non-string urls', () => {
    const entries = pageEntriesFromNavGroups([
      {
        items: [
          {
            title: 'group',
            items: [{ title: 'nested.title', url: '/nested' }],
          },
          { title: 'object.url', url: { pathname: '/object' } },
        ],
      },
    ])

    expect(entries.map((entry) => entry.url)).toEqual(['/nested'])
  })
})

describe('getSettingsNavEntries', () => {
  it('registers all five subareas plus one entry per section', () => {
    const entries = getSettingsNavEntries()

    const subareaEntries = entries.filter((entry) =>
      entry.key.startsWith('settings-subarea-')
    )
    const sectionEntries = entries.filter((entry) =>
      entry.key.startsWith('settings-section-')
    )

    expect(subareaEntries).toHaveLength(5)
    expect(sectionEntries.length).toBeGreaterThanOrEqual(17)
    expect(entries.every((entry) => entry.scope === 'settings')).toBe(true)
  })

  it('deep-links subareas to their default section and sections to their own URL', () => {
    const entries = getSettingsNavEntries()

    const basic = entries.find(
      (entry) => entry.key === 'settings-subarea-basic'
    )
    expect(basic?.url).toBe('/settings/basic/site')
    expect(
      entries.some(
        (entry) =>
          entry.key === 'settings-section-/settings/operations/program-logs'
      )
    ).toBe(true)
  })
})

describe('matchNavEntries', () => {
  const entries: SearchNavEntry[] = [
    { key: 'a', titleKey: 'a', url: '/a', scope: 'page' },
    { key: 'b', titleKey: 'b', url: '/b', scope: 'page' },
    { key: 'c', titleKey: 'c', url: '/c', scope: 'settings' },
  ]
  const labels: Record<string, string> = {
    a: 'Scheduled Tasks',
    b: 'Proxy Logs',
    c: 'Scheduling extras',
  }
  const resolveLabel = (key: string) => labels[key]

  it('returns every entry for an empty query (quick-entry mode)', () => {
    expect(matchNavEntries(entries, '  ', resolveLabel)).toEqual(entries)
  })

  it('matches case-insensitively and ranks starts-with before contains', () => {
    const matched = matchNavEntries(entries, 'schedul', resolveLabel)
    expect(matched.map((entry) => entry.key)).toEqual(['a', 'c'])
  })

  it('returns contains matches when nothing starts with the query', () => {
    const matched = matchNavEntries(entries, 'logs', resolveLabel)
    expect(matched.map((entry) => entry.key)).toEqual(['b'])
  })

  it('returns no entries when the query matches nothing', () => {
    expect(matchNavEntries(entries, 'zzz', resolveLabel)).toEqual([])
  })
})
