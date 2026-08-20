// metapi-go/features/settings/system-info/lib — program-log event row
// normalization. Kept out of the section component so the react-refresh rule
// (only-export-components) stays clean and the pure mapping is unit-testable.

import i18n from 'i18next'

import { toBcp47 } from '@/i18n/languages'
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
