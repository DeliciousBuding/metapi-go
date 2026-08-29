// metapi-go/layout — action registry for the global search palette.
//
// The ⌘K palette's action layer: high-frequency WRITE actions, alongside
// (not replacing) the navigation and entity layers. Each entry points at an
// affordance that already exists in the product — the registry only carries
// stable identity, i18n title keys, match keywords and icons. Execution
// lives in `search-modal.tsx`, where the router and mutation hooks are
// available.
//
// Selection audit (Wave 21, #1035 S6) — every entry reuses an existing
// dialog/mutation, no new business logic:
//   - add-site: the sites page already consumes a one-shot `?create=1`
//     deep link (the dashboard onboarding and the sites guided flow write
//     one), so the palette drives the exact same dialog-opening path.
//   - run-checkin-all: the same `useManualCheckin` mutation the checkin
//     page's "Run all check-ins" button fires.
//   - rebuild-routes: the same `useRebuildRoutes` mutation the routes page
//     fires, behind the same ConfirmDialog wording (the page gates it).
//   - refresh-route-decisions: the same `useRefreshRouteDecisions` mutation
//     the routes page header button fires.
//
// Rejected candidates (evaluated against the code, not invented):
//   - add account: the accounts page only consumes a one-shot create
//     deep link together with a valid siteId; a standalone `?create=1`
//     never opens the dialog.
//   - add route: the routes page has no `create` deep link, only `edit`.
//   - add channel: channels are never created manually (read-only list;
//     channels are composed by route rebuild / the route channel editor).
//   - retest channel: no such operation exists anywhere in web/src.
//   - refresh balances: only per-row and selection-scoped batch refresh
//     exist; a palette-wide variant would be a new operation.
//   - backup export: confirm-token + export-type + download flow (#1034),
//     a settings section, not a palette-sized action.
//   - clear cache / usage data: ConfirmDialog-gated maintenance ops,
//     reachable through the settings entries of the navigation layer.
//   - single-account check-in: needs the account picker dialog, which stays
//     on the checkin page; the all-accounts trigger covers the quick case.

import {
  CalendarCheck,
  RefreshCw,
  Server,
  Zap,
  type LucideIcon,
} from 'lucide-react'

export type SearchActionId =
  | 'add-site'
  | 'run-checkin-all'
  | 'rebuild-routes'
  | 'refresh-route-decisions'

export type SearchActionEntry = {
  /** Stable identity for cmdk item keys and the execution switch. */
  id: SearchActionId
  /** i18n key of the action title (verb phrase, en + zh-CN). */
  titleKey: string
  /** Lowercase bilingual aliases matched in addition to the title. */
  keywords: readonly string[]
  /** Icon rendered instead of the group icon. */
  icon: LucideIcon
}

/**
 * Registry order is the quick-entry render order: the creation action
 * first (the most common onboarding write), then operational triggers.
 */
export const SEARCH_ACTION_ENTRIES: readonly SearchActionEntry[] = [
  {
    id: 'add-site',
    titleKey: 'search.actions.addSite',
    keywords: ['new site', 'create site', '新建站点', '创建站点', '站点'],
    icon: Server,
  },
  {
    id: 'run-checkin-all',
    titleKey: 'search.actions.runCheckinAll',
    keywords: ['checkin', 'check-in', 'check in', '签到', '打卡'],
    icon: CalendarCheck,
  },
  {
    id: 'rebuild-routes',
    titleKey: 'search.actions.rebuildRoutes',
    keywords: ['rebuild', '重建', '重建路由'],
    icon: Zap,
  },
  {
    id: 'refresh-route-decisions',
    titleKey: 'search.actions.refreshRouteDecisions',
    keywords: ['decisions', '决策', '刷新决策'],
    icon: RefreshCw,
  },
]

/**
 * Local case-insensitive substring match over the translated action titles
 * plus their bilingual keyword aliases. Starts-with matches rank before
 * contains matches (label and keywords share the ranking); the registry
 * order is kept within a rank. Empty query returns every entry
 * (quick-entry mode).
 */
export function matchActionEntries(
  entries: readonly SearchActionEntry[],
  query: string,
  resolveLabel: (titleKey: string) => string
): SearchActionEntry[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return [...entries]

  const scored: Array<{ entry: SearchActionEntry; rank: number }> = []
  for (const entry of entries) {
    let rank = Number.POSITIVE_INFINITY
    const candidates = [
      resolveLabel(entry.titleKey).toLowerCase(),
      ...entry.keywords,
    ]
    for (const candidate of candidates) {
      if (candidate.startsWith(needle)) {
        rank = Math.min(rank, 0)
      } else if (candidate.includes(needle)) {
        rank = Math.min(rank, 1)
      }
    }
    if (Number.isFinite(rank)) scored.push({ entry, rank })
  }
  scored.sort((left, right) => left.rank - right.rank)
  return scored.map((item) => item.entry)
}
