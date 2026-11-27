// metapi-go/layout — action registry and smart operational query matcher for
// the global search palette (⌘K / Ctrl+K).
//
// Two layers live here without touching any feature module (Rule 2):
//   1. `SEARCH_ACTION_ENTRIES`: audited high-frequency write operations (keeps
//      the original four stable write actions unchanged).
//   2. `SEARCH_SHORTCUT_ENTRIES`: high-frequency operator diagnostic shortcuts
//      pointing straight to existing workspaces (route diagnosis workspace,
//      failed proxy logs, channel diagnostics / model tester, cross-site price
//      compare).
//   3. `resolveSmartLogQuery`: detects HTTP status codes (e.g. `429`, `503`) or
//      request-ID prefixes (`req_...`) typed into the palette and returns a
//      direct `/proxy-logs` navigation target.

import {
  AlertTriangle,
  CalendarCheck,
  FlaskConical,
  RefreshCw,
  Route,
  Scale,
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

type SearchShortcutId =
  | 'route-workspace'
  | 'failed-proxy-logs'
  | 'model-tester'
  | 'price-compare'

type SearchShortcutEntry = {
  id: SearchShortcutId
  titleKey: string
  keywords: readonly string[]
  icon: LucideIcon
  to: string
  search?: Record<string, unknown>
}

type SmartLogTarget = {
  kind: 'http-status' | 'request-id'
  value: string
  to: '/proxy-logs'
  search: Record<string, unknown>
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
 * High-frequency operational shortcuts reusing existing i18n keys and routes:
 *   - Route workspace (/token-routes)
 *   - Failed attempt logs (/proxy-logs?view=attempts&status=failed)
 *   - Channel diagnostics / model tester (/model-tester)
 *   - Cross-site price comparison (/price-compare)
 */
export const SEARCH_SHORTCUT_ENTRIES: readonly SearchShortcutEntry[] = [
  {
    id: 'route-workspace',
    titleKey: 'tokenRoutes.workspace.title',
    keywords: [
      'route diagnosis',
      'route sandbox',
      'call path',
      '路由诊断',
      '路由沙盒',
      '诊断路由',
    ],
    icon: Route,
    to: '/token-routes',
  },
  {
    id: 'failed-proxy-logs',
    titleKey: 'proxyLogs.page.summaryFailed',
    keywords: [
      'failed logs',
      'error logs',
      'errors',
      '429',
      '500',
      '502',
      '503',
      '失败日志',
      '异常日志',
      '报错',
    ],
    icon: AlertTriangle,
    to: '/proxy-logs',
    search: { view: 'attempts', status: 'failed' },
  },
  {
    id: 'model-tester',
    titleKey: 'sidebar.items.modelTester',
    keywords: [
      'model tester',
      'test model',
      'probe',
      '模型测试',
      '渠道诊断',
      '测试器',
    ],
    icon: FlaskConical,
    to: '/model-tester',
  },
  {
    id: 'price-compare',
    titleKey: 'sidebar.items.priceCompare',
    keywords: ['price compare', 'pricing', 'cost', '跨站比价', '比价', '价格'],
    icon: Scale,
    to: '/price-compare',
  },
]

const HTTP_STATUS_RE = /^[1-5]\d{2}$/
const REQUEST_ID_RE = /^req_[A-Za-z0-9_-]{3,}$/i

/**
 * Recognizes direct diagnostic queries typed into the command palette:
 *   - 3-digit HTTP status codes (e.g. `429`, `502`, `503`) → jumps to
 *     `/proxy-logs?view=attempts&q=<code>` (plus `status=failed` for 4xx/5xx)
 *   - `req_...` internal request IDs → jumps to
 *     `/proxy-logs?view=requests&requestId=<id>`
 */
export function resolveSmartLogQuery(query: string): SmartLogTarget | null {
  const trimmed = query.trim()
  if (!trimmed) return null

  if (REQUEST_ID_RE.test(trimmed)) {
    return {
      kind: 'request-id',
      value: trimmed,
      to: '/proxy-logs',
      search: { view: 'requests', requestId: trimmed },
    }
  }

  if (HTTP_STATUS_RE.test(trimmed)) {
    const code = Number(trimmed)
    const search: Record<string, unknown> = {
      view: 'attempts',
      q: trimmed,
    }
    if (code >= 400) {
      search.status = 'failed'
    }
    return {
      kind: 'http-status',
      value: trimmed,
      to: '/proxy-logs',
      search,
    }
  }

  return null
}

/**
 * Local case-insensitive substring match over translated titles plus bilingual
 * keyword aliases. Starts-with matches rank before contains matches; registry
 * order is kept within a rank. Empty query returns every entry.
 */
export function matchActionEntries<
  T extends { titleKey: string; keywords: readonly string[] },
>(
  entries: readonly T[],
  query: string,
  resolveLabel: (titleKey: string) => string
): T[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return [...entries]

  const scored: Array<{ entry: T; rank: number }> = []
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
