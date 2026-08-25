// metapi-go/features/dashboard/sections/availability — attention target parsing.
//
// The attention panel renders the backend `target` strings from
// GET /api/stats/attention as SPA links. The handler contract
// (handler/admin/stats_balance.go, attentionItem.Target) emits three shapes:
//
//   /accounts?accountId=N           — expired / low-balance accounts
//   /sites?edit=N                   — disabled sites
//   /settings/<subarea>/<section>   — warning/error events (program-logs)
//
// This module parses a raw target into the typed location the target page
// exposes (accounts consumes `accountId`, sites consumes `edit` — both
// one-shot deep links that open the referenced entity, then strip the param),
// so the panel can render a router `<Link>` instead of a full-page anchor.
// Unparseable targets resolve to null: the panel falls back to plain text
// rather than emitting a dead link.

export type AttentionTargetLocation =
  | { to: '/accounts'; search: { accountId: number } }
  | { to: '/sites'; search: { edit: number } }
  | { to: '/site-announcements' }
  | {
      to: '/settings/$subarea/$section'
      params: { subarea: string; section: string }
    }

/** Positive-int query param (mirrors the `z.coerce.number().int().positive()` route param contract). */
function parsePositiveInt(raw: string | null): number | undefined {
  if (raw === null) return undefined
  const value = Number(raw)
  return Number.isInteger(value) && value > 0 ? value : undefined
}

/** Parse a backend attention target into a typed router location (null when unrecognized). */
export function resolveAttentionTarget(
  target: string
): AttentionTargetLocation | null {
  if (!target) return null
  const queryStart = target.indexOf('?')
  const pathname = queryStart === -1 ? target : target.slice(0, queryStart)
  const params = new URLSearchParams(
    queryStart === -1 ? '' : target.slice(queryStart + 1)
  )

  switch (pathname) {
    case '/accounts': {
      const accountId = parsePositiveInt(params.get('accountId'))
      return accountId !== undefined
        ? { to: '/accounts', search: { accountId } }
        : null
    }
    case '/site-announcements':
      return { to: '/site-announcements' }
    case '/sites': {
      const edit = parsePositiveInt(params.get('edit'))
      return edit !== undefined ? { to: '/sites', search: { edit } } : null
    }
    default: {
      const match = /^\/settings\/([^/]+)\/([^/]+)$/.exec(pathname)
      if (match) {
        return {
          to: '/settings/$subarea/$section',
          params: { subarea: match[1], section: match[2] },
        }
      }
      return null
    }
  }
}
