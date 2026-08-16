// metapi-go/features/token-routes — pattern + presentation helpers.
//
// Self-contained rewrite of the legacy `web/pages/token-routes/utils.ts`.
// i18n keys are returned by the label helpers; callers wrap with `t()`.
// Error helpers use `i18n.t()` to return pre-translated strings (safe to
// pass as zod messages — FormMessage's `t()` returns them as-is).

import i18n from '@/i18n/config'

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

export const ROUTE_BRAND_ICON_PREFIX = 'brand:'
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
// ---------------------------------------------------------------------------

export function isRegexModelPattern(modelPattern: string): boolean {
  return (modelPattern || '').trim().startsWith('re:')
}

export function isExactModelPattern(modelPattern: string): boolean {
  const normalized = (modelPattern || '').trim()
  if (!normalized) return false
  if (isRegexModelPattern(normalized)) return false
  return !/[*[\]()?{}|^$\\]/.test(normalized)
}

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

export function matchesModelPattern(model: string, pattern: string): boolean {
  const normalizedPattern = (pattern || '').trim()
  if (!normalizedPattern) return false
  const modelName = (model || '').trim()
  if (!modelName) return false
  if (isRegexModelPattern(normalizedPattern)) {
    const parsed = parseRegexModelPattern(normalizedPattern)
    return parsed.regex ? parsed.regex.test(modelName) : false
  }
  return modelName === normalizedPattern
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

export type ResolvedRouteIcon =
  | { kind: 'auto' }
  | { kind: 'none' }
  | { kind: 'brand'; value: string }
  | { kind: 'text'; value: string }

export function resolveRouteIcon(
  displayIcon: string | null | undefined
): ResolvedRouteIcon {
  const normalized = (displayIcon || '').trim()
  if (!normalized) return { kind: 'auto' }
  if (isRouteIconNoneValue(normalized)) return { kind: 'none' }
  if (normalized.startsWith(ROUTE_BRAND_ICON_PREFIX)) {
    const brandKey = normalized.slice(ROUTE_BRAND_ICON_PREFIX.length).trim()
    if (brandKey) return { kind: 'brand', value: brandKey }
  }
  return { kind: 'text', value: normalized }
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

// ---------------------------------------------------------------------------
// Channel draft helpers
// ---------------------------------------------------------------------------

export function dedupeChannelDrafts(
  drafts: Array<{
    accountId: number
    tokenId?: number
    sourceModel?: string
  }>
): Array<{ accountId: number; tokenId?: number; sourceModel?: string }> {
  const seen = new Set<string>()
  const result: Array<{
    accountId: number
    tokenId?: number
    sourceModel?: string
  }> = []
  for (const draft of drafts) {
    const key = `${draft.accountId}::${draft.tokenId ?? 0}::${draft.sourceModel ?? ''}`
    if (seen.has(key)) continue
    seen.add(key)
    result.push({
      accountId: draft.accountId,
      tokenId: draft.tokenId || undefined,
      sourceModel: draft.sourceModel || undefined,
    })
  }
  return result
}
