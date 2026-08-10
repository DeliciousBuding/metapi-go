// metapi-go features/token-routes — pattern + presentation helpers.
//
// Self-contained rewrite of the legacy `web/pages/token-routes/utils.ts`.
// The legacy version imported a safe-regex parser from
// `web/shared/tokenRoutePatterns.js`; that module is not yet ported into
// `web/src/shared/`, so the pattern grammar is reimplemented here on top of
// the native `RegExp` constructor with try/catch. This is fine for the
// client-side preview/hit-sample UI — the server still owns the authoritative
// safe-regex validation, so a client-side `RegExp` round-trip cannot escalate
// privileges or bypass the backend.
//
// `isExactModelPattern` is also imported by the migrated
// `@/lib/helpers/zeroChannelRoutes` helper, so its signature (string → boolean)
// is preserved exactly.

import type {
  RouteMode,
  RouteRoutingStrategy,
  RouteRow,
  RouteSummaryRow,
} from './types'

// ---------------------------------------------------------------------------
// Constants — kept in sync with the legacy utils + presentation module
// ---------------------------------------------------------------------------

export const ROUTE_BRAND_ICON_PREFIX = 'brand:'
export const ROUTE_ICON_NONE_VALUE = '__route_icon_none__'

// ---------------------------------------------------------------------------
// Route mode normalization
// ---------------------------------------------------------------------------

export function normalizeRouteMode(
  routeMode: RouteMode | string | null | undefined,
): RouteMode {
  return routeMode === 'explicit_group' ? 'explicit_group' : 'pattern'
}

export function isExplicitGroupRoute(
  route: Pick<RouteRow | RouteSummaryRow, 'routeMode'>,
): boolean {
  return normalizeRouteMode(route.routeMode) === 'explicit_group'
}

// ---------------------------------------------------------------------------
// Pattern grammar — `re:` prefix = regex, anything else = exact literal match
// ---------------------------------------------------------------------------

export function isRegexModelPattern(modelPattern: string): boolean {
  return (modelPattern || '').trim().startsWith('re:')
}

export function isExactModelPattern(modelPattern: string): boolean {
  const normalized = (modelPattern || '').trim()
  if (!normalized) return false
  if (isRegexModelPattern(normalized)) return false
  // Treat any regex metacharacter as a signal that this is not a plain
  // exact name (mirrors the legacy stub behavior so zero-channel placeholder
  // dedup keeps working).
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
    return { regex: null, error: '正则体不能为空' }
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
  return `模型匹配正则错误：${parsed.error}`
}

// ---------------------------------------------------------------------------
// Presentation — title / icon / context length
// ---------------------------------------------------------------------------

export function resolveRouteTitle(
  route: Pick<RouteRow | RouteSummaryRow, 'displayName' | 'modelPattern'>,
): string {
  const title = (route.displayName || '').trim()
  return title || route.modelPattern
}

export function isRouteIconNoneValue(
  raw: string | null | undefined,
): boolean {
  return (raw || '').trim() === ROUTE_ICON_NONE_VALUE
}

export function normalizeRouteDisplayIconValue(
  raw: string | null | undefined,
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
  displayIcon: string | null | undefined,
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

/**
 * Compact context-length label: `128000` → `128k`, `1000000` → `1M`.
 * `null` / `0` / empty → `''` (unknown, no enforce).
 */
export function formatContextLength(
  contextLength: number | null | undefined,
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
// Routing strategy label — used by the columns + detail sheet
// ---------------------------------------------------------------------------

const ROUTING_STRATEGY_LABELS: Record<RouteRoutingStrategy, string> = {
  weighted: '权重随机',
  round_robin: '轮询',
  stable_first: '稳定优先',
}

export function normalizeRoutingStrategy(
  value: string | null | undefined,
): RouteRoutingStrategy {
  if (value === 'round_robin' || value === 'stable_first') return value
  return 'weighted'
}

export function routingStrategyLabel(
  value: string | null | undefined,
): string {
  return ROUTING_STRATEGY_LABELS[normalizeRoutingStrategy(value)]
}

// ---------------------------------------------------------------------------
// Channel draft helpers — convert the multi-select form value into the
// backend `batchAddChannels` payload shape.
// ---------------------------------------------------------------------------

export function dedupeChannelDrafts(
  drafts: Array<{
    accountId: number
    tokenId?: number
    sourceModel?: string
  }>,
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
