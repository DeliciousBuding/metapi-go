// metapi-go/features/proxy-logs — Zod schemas for the URL search state.
// i18n: status filter option labels are i18n keys.

import { z } from 'zod'

import {
  encodeSortingParam,
  stringSearchParam,
  tableSortingItemSchema,
} from '@/lib/helpers/searchParams'

/**
 * Tolerant URL search contract: the router JSON-parses search values, so
 * `?q=123` arrives as a number, `?status=true` as a boolean, and a
 * hand-written `?sort=123` or malformed JSON array as a non-string sort.
 * Invalid values fall back to sane defaults instead of throwing a route
 * error; valid values still validate exactly as before.
 */
export const proxyLogsSearchSchema = z.object({
  q: stringSearchParam,
  page: z.coerce.number().int().min(0).catch(0).default(0),
  pageSize: z.coerce.number().int().min(1).max(200).catch(20).default(20),
  sort: z
    .union([z.string(), z.array(tableSortingItemSchema)])
    .optional()
    .transform((value) => encodeSortingParam(value))
    .catch(undefined),
  status: z.enum(['all', 'success', 'failed']).catch('all').default('all'),
  siteId: z.coerce.number().int().optional().catch(undefined),
  // channelId scopes the list to the channel that served each request, so an
  // operator debugging "why is this channel failing" can drill straight in
  // instead of eyeballing a global search. Positive-only: the backend treats
  // 0 as "no channel filter", so a bogus `?channelId=0` degrades to unset.
  channelId: z.coerce.number().int().positive().optional().catch(undefined),
  // Client name and datetime-local bounds. Tolerant of router-parsed
  // primitives (null literals, duplicate-param arrays, numeric epochs) via
  // `stringSearchParam`; read sites normalize with `asStringParam`.
  client: stringSearchParam,
  from: stringSearchParam,
  to: stringSearchParam,
  latencyMin: z.coerce.number().int().min(0).optional().catch(undefined),
  latencyMax: z.coerce.number().int().min(0).optional().catch(undefined),
})

export type ProxyLogsSearch = z.infer<typeof proxyLogsSearchSchema>

export const PROXY_LOG_STATUS_FILTER_OPTIONS = [
  { labelKey: 'proxyLogs.filter.all', value: 'all' },
  { labelKey: 'proxyLogs.filter.success', value: 'success' },
  { labelKey: 'proxyLogs.filter.failed', value: 'failed' },
] as const
