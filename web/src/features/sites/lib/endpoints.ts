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

const FORBIDDEN_METADATA_HOSTS = new Set([
  'metadata.google.internal',
  'metadata',
  'instance-data',
])

/** Matches a full IPv4 address (also usable for ::ffff:1.2.3.4 tails). */
const IPV4_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/

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
 * Cloud metadata / link-local targets the backend rejects as a first-hop
 * SSRF risk. Mirrors Go `IsForbiddenSiteTargetURL` — RFC1918 private and
 * localhost stay allowed for lab/docker operators.
 */
export function isForbiddenEndpointTargetHost(hostname: string): boolean {
  const bare = hostname
    .trim()
    .toLowerCase()
    .replaceAll(/^\[|\]$/g, '')
  if (FORBIDDEN_METADATA_HOSTS.has(bare)) return true
  const ipv4Tail = /::ffff:(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})$/.exec(bare)
  const v4 = ipv4Tail
    ? (ipv4Tail[1] as string).split('.')
    : IPV4_RE.exec(bare)?.slice(1)
  if (v4) {
    // Link-local IPv4 169.254.0.0/16 (includes AWS/GCP/Azure metadata).
    return Number(v4[0]) === 169 && Number(v4[1]) === 254
  }
  // IPv6 link-local unicast fe80::/10: the first hextet is fe8 */fe9 */fea */feb *.
  const firstSegment = bare.split(':')[0] ?? ''
  return /^fe[89ab][0-9a-f]$/.test(firstSegment)
}

/** Valid endpoint URL: http(s) and not a forbidden metadata/link-local target. */
export function isValidEndpointUrl(raw: string): boolean {
  const trimmed = raw.trim()
  if (trimmed === '') return false
  try {
    const parsed = new URL(trimmed)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return false
    }
    return !isForbiddenEndpointTargetHost(parsed.hostname)
  } catch {
    return false
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
