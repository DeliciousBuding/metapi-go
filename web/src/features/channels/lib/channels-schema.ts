// metapi-go/features/channels — Zod schema for the list URL search state.
// Only search / page / page-size / sorting are URL-backed; the channels list
// has no faceted filters today.

import { z } from 'zod'

import {
  encodeSortingParam,
  stringSearchParam,
} from '@/lib/helpers/searchParams'

export const channelsSearchSchema = z.object({
  q: stringSearchParam,
  page: z.coerce.number().int().min(0).catch(0).default(0),
  pageSize: z.coerce.number().int().min(1).max(200).catch(20).default(20),
  sort: z
    .union([
      z.string(),
      z.array(z.object({ id: z.string(), desc: z.boolean() })),
    ])
    .optional()
    .transform((value) => encodeSortingParam(value))
    .catch(undefined),
})
