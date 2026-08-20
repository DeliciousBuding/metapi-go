// metapi-go/layout — local navigation index for the global search palette.
//
// The ⌘K palette searches backend entities via /api/search; this module
// supplies the complementary *local* layer: the primary pages (from the
// root sidebar data) and the Settings workspace (5 subareas + every
// section in the settings registry). Entries carry i18n title keys; the
// palette translates at render time so the index stays language-agnostic.

import type { ElementType } from 'react'

import { getSettingsSubareas } from '@/features/settings'

type SearchNavScope = 'page' | 'settings'

export type SearchNavEntry = {
  /** Stable identity for cmdk item keys. */
  key: string
  /** i18n key of the entry label. */
  titleKey: string
  /** Destination path (no query state). */
  url: string
  /** Which palette group the entry belongs to. */
  scope: SearchNavScope
  /** Optional icon rendered instead of the group icon. */
  icon?: ElementType
}

type SidebarLikeItem = {
  title: string
  url?: unknown
  icon?: ElementType
  items?: readonly SidebarLikeItem[]
}

/**
 * Flatten the root sidebar nav groups into page entries. Collapsible items
 * are recursed; object-shaped URLs (none today) are skipped because palette
 * targets must be plain paths.
 */
export function pageEntriesFromNavGroups(
  navGroups: readonly { items: readonly SidebarLikeItem[] }[]
): SearchNavEntry[] {
  const entries: SearchNavEntry[] = []

  const visit = (item: SidebarLikeItem): void => {
    if (item.items) {
      item.items.forEach(visit)
      return
    }
    if (typeof item.url !== 'string') return
    entries.push({
      key: `page-${item.url}`,
      titleKey: item.title,
      url: item.url,
      scope: 'page',
      ...(item.icon ? { icon: item.icon } : {}),
    })
  }

  navGroups.forEach((group) => group.items.forEach(visit))
  return entries
}

/**
 * Settings entries from the 5-subarea registry: one entry per subarea
 * (deep-linking to its default section) plus one entry per section.
 */
export function getSettingsNavEntries(): SearchNavEntry[] {
  const entries: SearchNavEntry[] = []
  for (const subarea of getSettingsSubareas()) {
    entries.push({
      key: `settings-subarea-${subarea.id}`,
      titleKey: subarea.title,
      url: `${subarea.basePath}/${subarea.defaultSection}`,
      scope: 'settings',
      ...(subarea.icon ? { icon: subarea.icon } : {}),
    })
    for (const section of subarea.getSectionNavItems()) {
      if (typeof section.url !== 'string') continue
      entries.push({
        key: `settings-section-${section.url}`,
        titleKey: section.title,
        url: section.url,
        scope: 'settings',
        ...(subarea.icon ? { icon: subarea.icon } : {}),
      })
    }
  }
  return entries
}

/**
 * Local case-insensitive substring match over the translated entry labels.
 * Starts-with matches rank before contains matches; the original order is
 * kept within a rank. Empty query returns every entry (quick-entry mode).
 */
export function matchNavEntries(
  entries: readonly SearchNavEntry[],
  query: string,
  resolveLabel: (titleKey: string) => string
): SearchNavEntry[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return [...entries]

  const scored: Array<{ entry: SearchNavEntry; rank: number }> = []
  for (const entry of entries) {
    const label = resolveLabel(entry.titleKey).toLowerCase()
    if (label.startsWith(needle)) {
      scored.push({ entry, rank: 0 })
    } else if (label.includes(needle)) {
      scored.push({ entry, rank: 1 })
    }
  }
  scored.sort((left, right) => left.rank - right.rank)
  return scored.map((item) => item.entry)
}
