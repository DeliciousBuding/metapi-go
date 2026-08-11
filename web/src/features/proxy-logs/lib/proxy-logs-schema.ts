// metapi-go/features/proxy-logs — Zod schemas for the URL search state.
// i18n: status filter option labels are i18n keys.

import { z } from 'zod'

import { parseSortingParam } from '@/lib/helpers/searchParams'

const sortingItemSchema = z.object({ id: z.string(), desc: z.boolean() })

export const proxyLogsSearchSchema = z.object({
  q: z.string().optional(),
  page: z.coerce.number().int().min(0).optional(),
  pageSize: z.coerce.number().int().min(1).max(200).optional(),
  sort: z
    .union([z.string(), z.array(sortingItemSchema)])
    .optional()
    .transform((value) => parseSortingParam(value)),
  status: z.enum(['all', 'success', 'failed']).optional(),
  siteId: z.coerce.number().int().optional(),
  client: z.string().optional(),
  from: z.string().optional(),
  to: z.string().optional(),
  latencyMin: z.coerce.number().int().min(0).optional(),
  latencyMax: z.coerce.number().int().min(0).optional(),
})

export type ProxyLogsSearch = z.infer<typeof proxyLogsSearchSchema>
export const SORTING_ITEM_SCHEMA = sortingItemSchema

export const PROXY_LOG_STATUS_FILTER_OPTIONS = [
  { labelKey: 'proxyLogs.filter.all', value: 'all' },
  { labelKey: 'proxyLogs.filter.success', value: 'success' },
  { labelKey: 'proxyLogs.filter.failed', value: 'failed' },
] as const
