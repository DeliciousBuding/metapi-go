import type { TFunction } from 'i18next'
import { LayoutGrid, ScrollText, Settings, Zap } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { ROOT_NAVIGATION } from '@/components/layout/config/root-navigation'

import {
  matchActionEntries,
  resolveSmartLogQuery,
  SEARCH_ACTION_ENTRIES,
  SEARCH_SHORTCUT_ENTRIES,
} from './lib/search-actions'
import {
  getSettingsNavEntries,
  matchNavEntries,
  pageEntriesFromNavGroups,
  type SearchNavEntry,
} from './lib/search-nav'
import {
  MAX_ITEMS_PER_GROUP,
  type SearchGroup,
  type SearchItem,
} from './search-result-list'

function smartLogGroups(query: string, t: TFunction): SearchGroup[] {
  const match = resolveSmartLogQuery(query)
  if (!match) return []
  const heading = t('search.groups.proxyLogs')
  const label =
    match.kind === 'request-id'
      ? t('proxyLogs.detail.requestId')
      : t('proxyLogs.detail.httpStatus')
  return [
    {
      key: 'smart-log',
      heading,
      icon: ScrollText,
      items: [
        {
          key: `smart-log-${match.kind}-${match.value}`,
          label: `${label}: ${match.value}`,
          description: heading,
          target: { to: match.to, search: match.search },
        },
      ],
    },
  ]
}

function actionGroups(query: string, t: TFunction): SearchGroup[] {
  const actions = matchActionEntries(SEARCH_ACTION_ENTRIES, query, t)
  const shortcuts = matchActionEntries(SEARCH_SHORTCUT_ENTRIES, query, t)
  const items: SearchItem[] = [
    ...actions.map((entry) => ({
      key: `action-${entry.id}`,
      label: t(entry.titleKey),
      description: null,
      icon: entry.icon,
      action: entry.id,
    })),
    ...shortcuts.map((entry) => ({
      key: `shortcut-${entry.id}`,
      label:
        entry.id === 'failed-proxy-logs'
          ? `${t('search.groups.proxyLogs')} · ${t(entry.titleKey)}`
          : t(entry.titleKey),
      description: null,
      icon: entry.icon,
      target: {
        to: entry.to,
        ...(entry.search ? { search: entry.search } : {}),
      },
    })),
  ]
  return items.length
    ? [
        {
          key: 'actions',
          heading: t('search.groups.actions'),
          icon: Zap,
          items,
        },
      ]
    : []
}

function navigationItems(
  entries: SearchNavEntry[],
  t: TFunction
): SearchItem[] {
  return entries.map((entry) => ({
    key: entry.key,
    label: t(entry.titleKey),
    description: null,
    ...(entry.icon ? { icon: entry.icon } : {}),
    target: { to: entry.url },
  }))
}

export function useLocalSearchGroups(query: string): SearchGroup[] {
  const { t } = useTranslation()
  const pages = useMemo(
    () => pageEntriesFromNavGroups(ROOT_NAVIGATION.navGroups),
    []
  )
  const settings = useMemo(() => getSettingsNavEntries(), [])
  return useMemo(() => {
    const matchedPages = query
      ? matchNavEntries(pages, query, t).slice(0, MAX_ITEMS_PER_GROUP)
      : pages
    const matchedSettings = query
      ? matchNavEntries(settings, query, t).slice(0, MAX_ITEMS_PER_GROUP)
      : settings.filter((entry) => entry.key.startsWith('settings-subarea-'))
    const groups = smartLogGroups(query, t)
    if (!query || matchedPages.length)
      groups.push({
        key: 'nav-pages',
        heading: t('search.groups.pages'),
        icon: LayoutGrid,
        items: navigationItems(matchedPages, t),
      })
    if (!query || matchedSettings.length)
      groups.push({
        key: 'nav-settings',
        heading: t('search.groups.settings'),
        icon: Settings,
        items: navigationItems(matchedSettings, t),
      })
    return [...groups, ...actionGroups(query, t)]
  }, [query, t, pages, settings])
}
