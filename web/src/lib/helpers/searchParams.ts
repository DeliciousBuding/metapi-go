// metapi-go/helpers — URL search-param decode helpers shared by the feature
// search schemas (sites / proxy-logs / models / oauth / site-announcements).
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
