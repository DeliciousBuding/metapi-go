// metapi-go/features/settings/operations/lib — program-log event row
// normalization. Kept out of the section component so the react-refresh rule
// (only-export-components) stays clean and the pure mapping is unit-testable.

import i18n from 'i18next'

import { toBcp47 } from '@/i18n/languages'
// Single source of truth for backend event title → i18n slug lives in
// lib/event-titles (shared with the attention pipeline); re-exported here so
// existing imports keep working.
export { eventTitleSlug } from '@/lib/event-titles'
import { formatDateTime } from '@/lib/format'

/**
 * A single operational event from GET /api/events.
 *
 * The handler returns raw DB rows (snake_case `created_at`, integer `read`);
 * {@link normalizeEvent} maps them to this camelCase shape.
 */
export type ProgramEvent = {
  id: number
  type: string
  title: string
  message?: string
  level?: 'info' | 'warning' | 'error'
  read?: boolean
  createdAt?: string
  /**
   * Structured-event fields (F5): rows emitted through the events registry
   * carry a stable titleKey plus typed params; legacy rows have neither and
   * render through the historical title-match path.
   */
  titleKey?: string
  params?: Record<string, string | number>
}

export type EventsResponse = {
  items: ProgramEvent[]
  total?: number
  limit?: number
}

/**
 * Normalize a raw events API row into the camelCase {@link ProgramEvent}
 * shape. The handler returns DB rows (snake_case `created_at`, integer
 * `read`), so without this mapping timestamps render as `—` and unread
 * detection relies on integer truthiness.
 */
export function normalizeEvent(raw: Record<string, unknown>): ProgramEvent {
  const titleKey =
    raw.title_key != null && raw.title_key !== ''
      ? String(raw.title_key)
      : undefined
  let params: Record<string, string | number> | undefined
  if (raw.params != null && raw.params !== '') {
    try {
      const parsed = JSON.parse(String(raw.params)) as unknown
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        params = parsed as Record<string, string | number>
      }
    } catch {
      // Malformed params degrade to the legacy title-match path.
      params = undefined
    }
  }
  return {
    id: Number(raw.id ?? 0),
    type: String(raw.type ?? ''),
    title: String(raw.title ?? ''),
    message: raw.message != null ? String(raw.message) : undefined,
    level: (raw.level as ProgramEvent['level']) || undefined,
    read:
      raw.read == null
        ? undefined
        : raw.read === true ||
          raw.read === 1 ||
          raw.read === '1' ||
          raw.read === 'true',
    createdAt: raw.created_at != null ? String(raw.created_at) : undefined,
    titleKey,
    params,
  }
}

/**
 * Render a raw event timestamp as a localized datetime string. Reads the
 * active i18next language at call time (the section re-renders on language
 * change), converting it to BCP-47 for the shared formatter.
 */
export function formatTimestamp(value?: string): string {
  if (!value) return '—'
  return formatDateTime(value, toBcp47(i18n.language || 'en'))
}

/** Structured parts of an enriched event message. */
export type EventMessageParts = {
  /** Remaining text after the enrichment lines are extracted. */
  base: string
  /** Comma-separated route names, or null. */
  routes: string | null
  /** Comma-separated site names, or null. */
  sites: string | null
  /** Internal panel path (e.g. /observability?section=health), or null. */
  panelPath: string | null
}

/**
 * Parse an enriched alert message into structured parts. The alert service
 * appends up to three lines ("Affected routes: …", "Alternative sites: …",
 * "Panel: …"); everything else is the base message. Plain messages (checkin
 * events) yield only `base`.
 */
export function parseEventMessage(message: string): EventMessageParts {
  const routesMatch = /(?:^|\n)Affected routes:\s*([^\n]*)$/m.exec(message)
  const sitesMatch = /(?:^|\n)Alternative sites:\s*([^\n]*)$/m.exec(message)
  const panelMatch = /(?:^|\n)Panel:\s*([^\s\n]+)$/m.exec(message)
  let base = message
  for (const match of [routesMatch, sitesMatch, panelMatch]) {
    if (match) base = base.replace(match[0], '')
  }
  return {
    base: base.replaceAll(/\n{2,}/g, '\n').trim(),
    routes: routesMatch?.[1]?.trim() || null,
    sites: sitesMatch?.[1]?.trim() || null,
    panelPath: panelMatch?.[1]?.trim() || null,
  }
}

/** Split a comma-separated enrichment name list into trimmed items. */
export function splitEnrichmentNames(value: string): string[] {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
}

/**
 * Parse an internal panel path ("/observability?section=health") into a
 * router-safe location. Returns null for anything that is not a simple
 * internal path, so the UI falls back to plain text instead of a dead link.
 */
export function parsePanelPath(
  path: string
): { to: string; search: Record<string, string> } | null {
  const queryStart = path.indexOf('?')
  const pathname = queryStart === -1 ? path : path.slice(0, queryStart)
  if (!pathname.startsWith('/') || /\s/.test(pathname)) return null
  const search: Record<string, string> = {}
  for (const [key, value] of new URLSearchParams(
    queryStart === -1 ? '' : path.slice(queryStart + 1)
  )) {
    search[key] = value
  }
  return { to: pathname, search }
}
