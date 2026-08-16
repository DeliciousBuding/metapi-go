// metapi-go/helpers — URL search-param decode helpers and shared Zod schema
// fragments used by the feature search schemas (sites / proxy-logs / models /
// oauth / channels).

import { z } from 'zod'
//
// TanStack Router parses `?sort=...` by attempting JSON.parse on each value
// and round-trips validated search objects back through the URL, so a sort
// param can arrive in three shapes:
//   - a hand-written comma list (`name:desc,url:asc`),
//   - a router-serialized JSON array (`[{"id":"name","desc":true}]`, or `[]`
//     when empty),
//   - after the page's own `window.location.search` read, the raw string of
//     either form.

export interface SortingItem {
  id: string
  desc: boolean
}

function encodeSortingItems(items: SortingItem[]): string {
  return items
    .map((item) => `${item.id}:${item.desc ? 'desc' : 'asc'}`)
    .join(',')
}

/**
 * Search-param that TanStack Router may hand over as a JSON-parsed primitive
 * (`?q=123` arrives as number `123`, `?enabled=true` as boolean `true`) even
 * though the page reads the same value back as a raw string via
 * `window.location.search`. Accepts all three and normalizes to string so
 * route `validateSearch` never throws on numeric/boolean-looking values.
 *
 * The `.catch(undefined)` is the resilience clause: `?q=null` JSON-parses to
 * `null` and duplicate params (`?q=a&q=b`) parse to an array — neither is
 * accepted by the union, and without the catch they would throw a
 * `validateSearch` error on URL entry. Degrade both to `undefined` instead.
 */
export const stringSearchParam = z
  .union([z.string(), z.number(), z.boolean()])
  .optional()
  .catch(undefined)

/**
 * Normalize a validated {@link stringSearchParam} value to a plain string.
 * At runtime the value is always a raw URL string (pages feed
 * `URLSearchParams.get` output into `safeParse`); the union simply keeps the
 * route's `validateSearch` from rejecting router-parsed primitives such as
 * `?q=123` (number) or `?enabled=true` (boolean).
 */
export function asStringParam(
  value: string | number | boolean | undefined
): string | undefined {
  if (typeof value === 'string') return value
  return value === undefined ? undefined : String(value)
}

/**
 * Normalize a sort param for the URL: accepts the raw string form, the
 * router-serialized JSON array form, or an already-parsed array, and always
 * returns the canonical comma-separated string (or undefined when empty).
 * Search schemas use this in `transform` so TanStack Router round-trips the
 * param back to the URL in the same shape it was read, instead of
 * re-serializing the parsed array as JSON (`?sort=%5B%5D` noise).
 */
export function encodeSortingParam(
  value: string | SortingItem[] | undefined
): string | undefined {
  if (!value) return undefined
  if (Array.isArray(value)) {
    return encodeSortingItems(value) || undefined
  }
  const trimmed = value.trim()
  if (trimmed.length === 0 || trimmed === '[]') return undefined
  return encodeSortingItems(parseSortingParam(trimmed)) || undefined
}

export function parseSortingParam(
  value: string | SortingItem[] | undefined
): SortingItem[] {
  if (!value) return []
  if (Array.isArray(value)) return [...value]
  const trimmed = value.trim()
  if (trimmed.length === 0 || trimmed === '[]') return []
  if (trimmed.startsWith('[')) {
    try {
      const parsed: unknown = JSON.parse(trimmed)
      if (Array.isArray(parsed)) {
        return parsed.filter(
          (item): item is SortingItem =>
            !!item &&
            typeof item === 'object' &&
            typeof (item as { id?: unknown }).id === 'string' &&
            typeof (item as { desc?: unknown }).desc === 'boolean'
        )
      }
    } catch {
      // Not JSON — fall through to the comma-separated form.
    }
  }
  return trimmed.split(',').map((segment) => {
    const [id, direction] = segment.split(':')
    return { id: id ?? '', desc: direction === 'desc' }
  })
}

export function parseStringListParam(
  value: string | string[] | undefined
): string[] {
  if (!value) return []
  if (Array.isArray(value)) return [...value]
  const trimmed = value.trim()
  if (trimmed.length === 0 || trimmed === '[]') return []
  if (trimmed.startsWith('[')) {
    try {
      const parsed: unknown = JSON.parse(trimmed)
      if (Array.isArray(parsed)) {
        return parsed.filter((item): item is string => typeof item === 'string')
      }
    } catch {
      // Not JSON — fall through to the comma-separated form.
    }
  }
  return trimmed
    .split(',')
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0)
}

/**
 * Normalize a string-list param for the URL: accepts the raw comma string,
 * the router-serialized JSON array form, or an already-parsed array, and
 * always returns the canonical comma-separated string (or undefined when
 * empty). Used in search-schema `transform` so TanStack Router round-trips
 * the param in the same shape it was read (no `?brand=%5B%5D` noise).
 */
export function encodeStringListParam(
  value: string | string[] | undefined
): string | undefined {
  if (!value) return undefined
  if (Array.isArray(value)) {
    return value.join(',') || undefined
  }
  const trimmed = value.trim()
  if (trimmed.length === 0 || trimmed === '[]') return undefined
  return parseStringListParam(trimmed).join(',') || undefined
}

/**
 * Shared Zod schema for a single TanStack sorting item (`{ id, desc }`).
 * The feature search schemas reuse this in their `sort` field instead of
 * re-declaring the shape per feature.
 */
export const tableSortingItemSchema = z.object({
  id: z.string(),
  desc: z.boolean(),
})
