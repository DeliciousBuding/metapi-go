// metapi-go/features/proxy-logs — Zod schemas for the URL search state.
//
// `proxyLogsSearchSchema` is the URL contract for the proxy-logs list page:
// pagination / global search / status filter / site filter / client filter
// / date range / latency range. The schema is the "validateSearch" stage of
// the three-stage URL state sync pattern (route validateSearch -> feature
// useSearch -> useDataTable); the page also safe-parses
// `window.location.search` directly so it works before the `/proxy-logs`
// route file registers a typed search schema. `z.coerce` is fine here
// because the schema is only consumed via `safeParse`.

import { z } from 'zod'

// --- URL search state -----------------------------------------------------

const sortingItemSchema = z.object({
  id: z.string(),
  desc: z.boolean(),
})

export const proxyLogsSearchSchema = z.object({
  q: z.string().optional(),
  page: z.coerce.number().int().min(0).optional(),
  pageSize: z.coerce.number().int().min(1).max(200).optional(),
  sort: z
    .string()
    .optional()
    .transform((value) => {
      if (!value) return [] as z.infer<typeof sortingItemSchema>[]
      return value.split(',').map((segment) => {
        const [id, direction] = segment.split(':')
        return { id: id ?? '', desc: direction === 'desc' }
      })
    }),
  // `all` | `success` | `failed` — matches backend `ProxyLogStatusFilter`.
  status: z.enum(['all', 'success', 'failed']).optional(),
  siteId: z.coerce.number().int().optional(),
  client: z.string().optional(),
  // ISO datetime-local strings (`YYYY-MM-DDTHH:mm`); the backend accepts
  // either ISO 8601 or the loose datetime-local format.
  from: z.string().optional(),
  to: z.string().optional(),
  latencyMin: z.coerce.number().int().min(0).optional(),
  latencyMax: z.coerce.number().int().min(0).optional(),
})

export type ProxyLogsSearch = z.infer<typeof proxyLogsSearchSchema>

export const SORTING_ITEM_SCHEMA = sortingItemSchema

// --- Status filter options (UI) ------------------------------------------

export const PROXY_LOG_STATUS_FILTER_OPTIONS = [
  { label: '全部', value: 'all' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
] as const
