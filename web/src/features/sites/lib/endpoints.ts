// metapi-go/features/sites — pure helpers for the apiEndpoints editor.
//
// The wire format of `site.apiEndpoints` is an array of objects
// `{ url, enabled, sortOrder }` (mirrored by Go payloads.SiteAPIEndpointInput
// and the read-model `SiteApiEndpoint`), not a plain string array. The form
// dialog edits this list in a textarea: one entry per line — either a plain
// URL (defaults `enabled: true`, `sortOrder` = position) or a compact JSON
// object `{"url":"…","enabled":false,"sortOrder":5}`. The helpers below
// normalize and validate the lines with the same rules the Go service
// enforces (`service.NormalizeSiteAPIEndpointBaseUrl`,
// `IsValidAPIEndpointURL`, `IsForbiddenSiteTargetURL`) so the dialog rejects
// client-side without a server round-trip.

import { isValidEndpointUrl } from '@/lib/url-validation'

export {
  isForbiddenEndpointTargetHost,
  isValidEndpointUrl,
} from '@/lib/url-validation'

import type { SiteApiEndpoint } from '../types'

/** One parsed endpoint line — matches `SiteFormPayload.apiEndpoints` items. */
export type ParsedEndpoint = {
  url: string
  enabled: boolean
  sortOrder: number
}

export type EndpointsTextError =
  | 'invalidJson'
  | 'invalidEntry'
  | 'invalidUrl'
  | 'duplicate'

export type EndpointsParseResult =
  | { endpoints: ParsedEndpoint[] }
  | { error: EndpointsTextError }

/** http(s) URL check shared by the site form URL fields and the editor. */
export function isHttpUrl(value: string): boolean {
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

/**
 * Strip the search/hash and a trailing slash — mirrors Go
 * `NormalizeSiteAPIEndpointBaseUrl`, including the lenient trim fallback when
 * the URL cannot be parsed. Used for duplicate detection.
 */
export function normalizeEndpointBaseUrl(raw: string): string {
  const trimmed = raw.trim()
  if (trimmed === '') return ''
  try {
    const parsed = new URL(trimmed)
    parsed.search = ''
    parsed.hash = ''
    return parsed.toString().replace(/\/+$/, '')
  } catch {
    return trimmed.replace(/\/+$/, '')
  }
}

/**
 * Render a site's endpoints as textarea content: one compact JSON object per
 * line. Round-trips through `parseEndpointsEditorText` losslessly so the
 * editor can be pre-filled from an existing site without touching the
 * untouched-preserve path (an untouched submit never re-parses anyway).
 */
export function serializeEndpointsForEditor(
  endpoints: SiteApiEndpoint[] | undefined
): string {
  return (endpoints ?? [])
    .map((endpoint) =>
      JSON.stringify({
        url: endpoint.url,
        enabled: endpoint.enabled ?? true,
        sortOrder: endpoint.sortOrder ?? 0,
      })
    )
    .join('\n')
}

/**
 * Parse the textarea into endpoint objects. First error wins (top-down line
 * order) so the operator's fix location is deterministic. Rules:
 *  - blank / whitespace-only lines are ignored,
 *  - a line starting with `{` (or `[`) must be JSON: an object with a string
 *    `url` and optional boolean `enabled` + non-negative integer `sortOrder`
 *    (arrays are rejected),
 *  - any other line is treated as a bare URL (enabled defaults to true,
 *    sortOrder defaults to the entry's position in the list),
 *  - every URL must be a valid http(s) endpoint URL,
 *  - normalized URLs must be unique.
 */
export function parseEndpointsEditorText(text: string): EndpointsParseResult {
  const endpoints: ParsedEndpoint[] = []
  const seen = new Set<string>()
  for (const rawLine of text.split(/\r?\n/)) {
    const trimmed = rawLine.trim()
    if (trimmed === '') continue

    let url: string
    let enabled: boolean | undefined
    let sortOrder: number | undefined
    if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
      let parsed: unknown
      try {
        parsed = JSON.parse(trimmed)
      } catch {
        return { error: 'invalidJson' }
      }
      if (
        typeof parsed !== 'object' ||
        parsed === null ||
        Array.isArray(parsed)
      ) {
        return { error: 'invalidEntry' }
      }
      const record = parsed as Record<string, unknown>
      if (typeof record.url !== 'string') return { error: 'invalidEntry' }
      if (record.enabled !== undefined && typeof record.enabled !== 'boolean') {
        return { error: 'invalidEntry' }
      }
      if (
        record.sortOrder !== undefined &&
        (typeof record.sortOrder !== 'number' ||
          !Number.isInteger(record.sortOrder) ||
          record.sortOrder < 0)
      ) {
        return { error: 'invalidEntry' }
      }
      url = record.url
      enabled = record.enabled as boolean | undefined
      sortOrder = record.sortOrder as number | undefined
    } else {
      url = trimmed
    }

    if (!isValidEndpointUrl(url)) return { error: 'invalidUrl' }
    const normalized = normalizeEndpointBaseUrl(url)
    if (seen.has(normalized)) return { error: 'duplicate' }
    seen.add(normalized)

    endpoints.push({
      url: url.trim(),
      enabled: enabled ?? true,
      sortOrder: sortOrder ?? endpoints.length,
    })
  }
  return { endpoints }
}
