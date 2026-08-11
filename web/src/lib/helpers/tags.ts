/* ------------------------------------------------------------------ */
/*  Tag helpers                                */
/* ------------------------------------------------------------------ */

/**
 * parseTags normalizes a tag value from the API. Backend stores tags as a
 * JSON array text (string) or empty; the site list also passes it through
 * as a string. Tolerates null / empty / already-parsed arrays.
 */
export function parseTags(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.filter(
      (t): t is string => typeof t === 'string' && t.trim() !== ''
    )
  }
  if (typeof value !== 'string' || value.trim() === '') return []
  try {
    const parsed = JSON.parse(value)
    if (Array.isArray(parsed)) {
      return parsed.filter(
        (t): t is string => typeof t === 'string' && t.trim() !== ''
      )
    }
  } catch {
    // Not JSON — fall through to comma-separated tolerance.
  }
  return value
    .split(/[,，]/)
    .map((t) => t.trim())
    .filter(Boolean)
}

/** Stable hash → design chart palette so the same tag always gets one color. */
export function tagColor(tag: string): string {
  let hash = 0
  for (let i = 0; i < tag.length; i += 1) {
    hash = (hash * 31 + tag.charCodeAt(i)) | 0
  }
  const idx = (Math.abs(hash) % 8) + 1
  return `var(--color-chart-${idx})`
}

/** Collect the union of tags across rows, most frequent first. */
export function collectTags(rows: Array<{ tags?: unknown }>): string[] {
  const counts = new Map<string, number>()
  for (const row of rows) {
    for (const tag of parseTags(row.tags)) {
      counts.set(tag, (counts.get(tag) ?? 0) + 1)
    }
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1))
    .map(([tag]) => tag)
}

/** Serialize a tag list to the JSON array text the backend stores. */
export function encodeTags(tags: string[]): string {
  const clean = [...new Set(tags.map((t) => t.trim()).filter(Boolean))]
  return JSON.stringify(clean)
}
