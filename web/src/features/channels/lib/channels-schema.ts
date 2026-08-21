// metapi-go/features/channels — Zod schema for the list URL search state.
// Search / page / page-size / sorting / status are URL-backed; status is the
// only faceted filter and is applied client-side like the search box (the
// channels list is fetched in full and filtered by the data table).

import { z } from 'zod'

import {
  encodeSortingParam,
  stringSearchParam,
  tableSortingItemSchema,
} from '@/lib/helpers/searchParams'

export const channelsSearchSchema = z.object({
  q: stringSearchParam,
  page: z.coerce.number().int().min(0).catch(0).default(0),
  pageSize: z.coerce.number().int().min(1).max(200).catch(20).default(20),
  sort: z
    .union([z.string(), z.array(tableSortingItemSchema)])
    .optional()
    .transform((value) => encodeSortingParam(value))
    .catch(undefined),
  // Comma-separated `routing.ChannelRuntimeStatus` values (enabled /
  // cooldown / breaker_open / manually_disabled) for the toolbar status
  // facet, so a failing-channel view is shareable as a link.
  status: stringSearchParam,
  // One-shot drilldown from the proxy-log detail sheet: open the detail
  // view for this channel, then the page strips the param (same consume
  // pattern as the accounts `create`/`siteId` deep link).
  channelId: z.coerce.number().int().positive().optional().catch(undefined),
})
