// metapi-go/features/sites — primary-site URL analysis for the add/edit
// form's normalization guidance.
//
// Port of the TS original's `shared/sitePrimaryUrl.ts`: analyzes the primary
// site URL in real time so the form can tell the operator when a common API
// request suffix (`/v1`, `/v1/models`, …) should be stripped before saving,
// warn when an `/api` or other extra path is about to be stored as-is, and
// persist the canonical value on save for the auto-strip case. The path sets
// and the three-way action classification match the TS original so the two
// frontends behave identically.

type PrimarySiteUrlAction =
  | 'unchanged'
  | 'auto_strip_known_api_suffix'
  | 'preserve_api_path'
  | 'preserve_semantic_path'
  | 'preserve_unknown_path'

export type PrimarySiteUrlAnalysis = {
  canonicalUrl: string
  /** URL to persist on save: canonicalUrl, stripped to origin for the known
   *  API-suffix case, raw (unparsed) fallback for invalid values. */
  persistedUrl: string
  matchedPath: string
  action: PrimarySiteUrlAction
}

// Request-route suffixes that are never the primary site URL (panel / login /
// check-in address). When one of these is present, the value is persisted as
// the bare origin.
const AUTO_STRIP_PRIMARY_SITE_PATHS = new Set([
  '/v1',
  '/v1beta',
  '/v1/models',
  '/v1/chat/completions',
  '/v1/responses',
  '/v1/messages',
  '/v1beta/models',
])

// Real primary-host paths of providers that happen to share their console and
// API surfaces at the same root — kept as-is without a warning.
const SEMANTIC_PRIMARY_SITE_PATHS = new Set([
  '/backend-api/codex',
  '/anthropic',
  '/apps/anthropic',
  '/api/anthropic',
  '/api/coding/paas/v4',
  '/v1beta/openai',
])

function normalizePathname(pathname: string): string {
  let normalized = typeof pathname === 'string' ? pathname.trim() : ''
  if (!normalized || normalized === '/') return '/'
  if (!normalized.startsWith('/')) normalized = `/${normalized}`
  while (normalized.length > 1 && normalized.endsWith('/')) {
    normalized = normalized.slice(0, -1)
  }
  return normalized
}

function parseUrlCandidate(url: string | null | undefined): URL | null {
  const trimmed = typeof url === 'string' ? url.trim() : ''
  if (!trimmed) return null
  const candidates = trimmed.includes('://')
    ? [trimmed]
    : [`https://${trimmed}`]
  for (const candidate of candidates) {
    try {
      return new URL(candidate)
    } catch {
      // try the next candidate
    }
  }
  return null
}

/**
 * Classify the primary site URL for the form's guidance alerts. Pure — no
 * side effects, safe to call on every keystroke.
 */
export function analyzePrimarySiteUrl(
  url: string | null | undefined
): PrimarySiteUrlAnalysis {
  const parsed = parseUrlCandidate(url)
  if (!parsed) {
    const trimmed =
      typeof url === 'string' ? url.trim().replace(/\/+$/, '') : ''
    return {
      canonicalUrl: trimmed,
      persistedUrl: trimmed,
      matchedPath: '',
      action: 'unchanged',
    }
  }

  parsed.search = ''
  parsed.hash = ''
  const matchedPath = normalizePathname(parsed.pathname)
  const canonicalUrl =
    matchedPath === '/' ? parsed.origin : `${parsed.origin}${matchedPath}`

  if (matchedPath === '/') {
    return {
      canonicalUrl,
      persistedUrl: canonicalUrl,
      matchedPath,
      action: 'unchanged',
    }
  }

  if (SEMANTIC_PRIMARY_SITE_PATHS.has(matchedPath)) {
    return {
      canonicalUrl,
      persistedUrl: canonicalUrl,
      matchedPath,
      action: 'preserve_semantic_path',
    }
  }

  if (AUTO_STRIP_PRIMARY_SITE_PATHS.has(matchedPath)) {
    return {
      canonicalUrl,
      persistedUrl: parsed.origin,
      matchedPath,
      action: 'auto_strip_known_api_suffix',
    }
  }

  if (matchedPath.startsWith('/api')) {
    return {
      canonicalUrl,
      persistedUrl: canonicalUrl,
      matchedPath,
      action: 'preserve_api_path',
    }
  }

  return {
    canonicalUrl,
    persistedUrl: canonicalUrl,
    matchedPath,
    action: 'preserve_unknown_path',
  }
}
