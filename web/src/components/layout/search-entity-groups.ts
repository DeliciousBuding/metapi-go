import type { TFunction } from 'i18next'
import {
  Boxes,
  CalendarCheck,
  Globe,
  KeyRound,
  ScrollText,
  UserRound,
} from 'lucide-react'

import type {
  SearchAccount,
  SearchAccountToken,
  SearchCheckinLog,
  SearchModel,
  SearchProxyLog,
  SearchResponse,
  SearchSite,
} from '@/lib/api/search'

import {
  MAX_ITEMS_PER_GROUP,
  type SearchGroup,
  type SearchTarget,
} from './search-result-list'

function firstNonEmpty(
  ...values: Array<string | null | undefined>
): string | null {
  for (const value of values) {
    const trimmed = value?.trim()
    if (trimmed) return trimmed
  }
  return null
}

function proxyLogTarget(log: SearchProxyLog, query: string): SearchTarget {
  const text = firstNonEmpty(log.modelRequested, query)
  return {
    to: '/proxy-logs',
    search: {
      view: 'attempts',
      ...(text ? { q: text } : {}),
      ...(log.status === 'success' || log.status === 'failed'
        ? { status: log.status }
        : {}),
    },
  }
}

export function entitySearchGroups(
  results: SearchResponse | null,
  query: string,
  t: TFunction
): SearchGroup[] {
  if (!results) return []
  const unknownLabel = t('search.unknown')
  const groups: SearchGroup[] = [
    {
      key: 'sites',
      heading: t('search.groups.sites'),
      icon: Globe,
      items: (results.sites ?? [])
        .slice(0, MAX_ITEMS_PER_GROUP)
        .map((site: SearchSite) => ({
          key: `site-${site.id}`,
          label: firstNonEmpty(site.name) ?? unknownLabel,
          description: firstNonEmpty(site.url),
          target: { to: '/sites', search: { edit: site.id } },
        })),
    },
    {
      key: 'accounts',
      heading: t('search.groups.accounts'),
      icon: UserRound,
      items: (results.accounts ?? [])
        .slice(0, MAX_ITEMS_PER_GROUP)
        .map((account: SearchAccount) => ({
          key: `account-${account.id}`,
          label: firstNonEmpty(account.username) ?? unknownLabel,
          description: firstNonEmpty(account.siteName),
          target: { to: '/accounts', search: { accountId: account.id } },
        })),
    },
    {
      key: 'accountTokens',
      heading: t('search.groups.tokens'),
      icon: KeyRound,
      items: (results.accountTokens ?? [])
        .slice(0, MAX_ITEMS_PER_GROUP)
        .map((token: SearchAccountToken) => ({
          key: `token-${token.id}`,
          label: firstNonEmpty(token.name) ?? unknownLabel,
          description: firstNonEmpty(token.accountUsername),
          // Older search responses lack the owning account id.
          target: {
            to: '/accounts',
            search:
              token.accountId != null
                ? { accountId: token.accountId }
                : { q: query },
          },
        })),
    },
    {
      key: 'checkinLogs',
      heading: t('search.groups.checkinLogs'),
      icon: CalendarCheck,
      items: (results.checkinLogs ?? [])
        .slice(0, MAX_ITEMS_PER_GROUP)
        .map((log: SearchCheckinLog) => ({
          key: `checkin-${log.id}`,
          label:
            firstNonEmpty(log.message, log.reward, log.status) ?? unknownLabel,
          description: firstNonEmpty(log.accountUsername),
          target: {
            to: '/checkin',
            search: {
              ...(log.accountId != null ? { accountId: log.accountId } : {}),
              ...(query ? { q: query } : {}),
            },
          },
        })),
    },
    {
      key: 'proxyLogs',
      heading: t('search.groups.proxyLogs'),
      icon: ScrollText,
      items: (results.proxyLogs ?? [])
        .slice(0, MAX_ITEMS_PER_GROUP)
        .map((log) => ({
          key: `proxy-log-${log.id}`,
          label:
            firstNonEmpty(log.modelRequested, log.modelActual) ?? unknownLabel,
          description: firstNonEmpty(log.status),
          target: proxyLogTarget(log, query),
        })),
    },
    {
      key: 'models',
      heading: t('search.groups.models'),
      icon: Boxes,
      items: (results.models ?? [])
        .slice(0, MAX_ITEMS_PER_GROUP)
        .map((model: SearchModel) => ({
          key: `model-${model.modelName}`,
          label: firstNonEmpty(model.modelName) ?? unknownLabel,
          description:
            model.tokenCount != null
              ? t('search.modelTokens', { count: model.tokenCount })
              : null,
          target: { to: '/models', search: { model: model.modelName } },
        })),
    },
  ]
  return groups.filter((group) => group.items.length > 0)
}
