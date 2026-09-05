// metapi-go/features/token-routes — pattern + presentation helpers.
//
// Self-contained rewrite of the legacy `web/pages/token-routes/utils.ts`.
// i18n keys are returned by the label helpers; callers wrap with `t()`.
// Error helpers use `i18n.t()` to return pre-translated strings (safe to
// pass as zod messages — FormMessage's `t()` returns them as-is).

import i18n from '@/i18n/config'
import { isRegexModelPattern } from '@/lib/helpers/model-pattern'

import type {
  RouteMode,
  RouteRoutingStrategy,
  RouteRow,
  RouteSummaryRow,
} from './types'

function isRouteIconNoneValue(raw: string | null | undefined): boolean {
  return (raw || '').trim() === ROUTE_ICON_NONE_VALUE
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------
export const ROUTE_ICON_NONE_VALUE = '__route_icon_none__'

// ---------------------------------------------------------------------------
// Route mode normalization
// ---------------------------------------------------------------------------

export function normalizeRouteMode(
  routeMode: RouteMode | string | null | undefined
): RouteMode {
  return routeMode === 'explicit_group' ? 'explicit_group' : 'pattern'
}

export function isExplicitGroupRoute(
  route: Pick<RouteRow | RouteSummaryRow, 'routeMode'>
): boolean {
  return normalizeRouteMode(route.routeMode) === 'explicit_group'
}

// ---------------------------------------------------------------------------
// Pattern grammar
// The pure grammar predicates live in @/lib/helpers/model-pattern (shared
// with lib helpers — S5 boundary inversion); re-exported for compatibility.
export {
  isExactModelPattern,
  isRegexModelPattern,
} from '@/lib/helpers/model-pattern'
type ParsedRegex = {
  regex: { test(value: string): boolean } | null
  error: string | null
}

export function parseRegexModelPattern(modelPattern: string): ParsedRegex {
  const normalized = (modelPattern || '').trim()
  if (!isRegexModelPattern(normalized)) {
    return { regex: null, error: null }
  }
  const body = normalized.slice(3).trim()
  if (!body) {
    return { regex: null, error: i18n.t('tokenRoutes.utils.regexEmpty') }
  }
  try {
    const regex = new RegExp(body)
    return { regex, error: null }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    return { regex: null, error: message }
  }
}

export function getModelPatternError(modelPattern: string): string | null {
  const normalized = (modelPattern || '').trim()
  if (!normalized) return null
  if (!isRegexModelPattern(normalized)) return null
  const parsed = parseRegexModelPattern(normalized)
  if (!parsed.error) return null
  return i18n.t('tokenRoutes.utils.regexError', { error: parsed.error })
}

// ---------------------------------------------------------------------------
// Presentation — title / icon / context length
// ---------------------------------------------------------------------------

export function resolveRouteTitle(
  route: Pick<RouteRow | RouteSummaryRow, 'displayName' | 'modelPattern'>
): string {
  const title = (route.displayName || '').trim()
  return title || route.modelPattern
}

export function normalizeRouteDisplayIconValue(
  raw: string | null | undefined
): string {
  const normalized = (raw || '').trim()
  if (isRouteIconNoneValue(normalized)) return ROUTE_ICON_NONE_VALUE
  return normalized
}

export function formatContextLength(
  contextLength: number | null | undefined
): string {
  if (!contextLength || contextLength <= 0) return ''
  if (contextLength >= 1_000_000) {
    const mega = contextLength / 1_000_000
    return `${Number.isInteger(mega) ? mega : mega.toFixed(1).replace(/\.0$/, '')}M`
  }
  if (contextLength >= 1000) {
    const kilo = contextLength / 1000
    return `${Number.isInteger(kilo) ? kilo : kilo.toFixed(1).replace(/\.0$/, '')}k`
  }
  return String(contextLength)
}

// ---------------------------------------------------------------------------
// Routing strategy label — returns i18n key; callers wrap with `t()`
// ---------------------------------------------------------------------------

const ROUTING_STRATEGY_LABEL_KEYS: Record<RouteRoutingStrategy, string> = {
  weighted: 'tokenRoutes.strategies.weighted',
  round_robin: 'tokenRoutes.strategies.round_robin',
  stable_first: 'tokenRoutes.strategies.stable_first',
  least_busy: 'tokenRoutes.strategies.least_busy',
  lowest_latency: 'tokenRoutes.strategies.lowest_latency',
  lowest_cost: 'tokenRoutes.strategies.lowest_cost',
}

export function normalizeRoutingStrategy(
  value: string | null | undefined
): RouteRoutingStrategy {
  switch (value) {
    case 'round_robin':
    case 'stable_first':
    case 'least_busy':
    case 'lowest_latency':
    case 'lowest_cost':
      return value
    default:
      return 'weighted'
  }
}

export function routingStrategyLabel(value: string | null | undefined): string {
  return ROUTING_STRATEGY_LABEL_KEYS[normalizeRoutingStrategy(value)]
}
