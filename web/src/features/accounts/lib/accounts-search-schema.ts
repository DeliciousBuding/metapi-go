// metapi-go/features/accounts/lib — URL search-state contract for the accounts
// list. Keeping the schema in the feature layer lets both the route validator
// and the page serializer share one canonical contract without introducing a
// route <-> feature import cycle.

import { z } from 'zod'

import {
  encodeSortingParam,
  stringSearchParam,
} from '@/lib/helpers/searchParams'

export const DEFAULT_ACCOUNTS_PAGE_SIZE = 20

const sortingItemSchema = z.object({
  id: z.string(),
  desc: z.boolean(),
})

/**
 * Accounts uses a 1-based `page` URL for backwards-compatible bookmarks while
 * TanStack Table uses a 0-based page index internally. `sort` is normalized to
 * the same canonical `id:asc,id:desc` string used by the other list pages.
 */
export const accountsSearchSchema = z.object({
  page: z.coerce.number().int().positive().catch(1).default(1),
  pageSize: z.coerce
    .number()
    .int()
    .min(1)
    .max(200)
    .catch(DEFAULT_ACCOUNTS_PAGE_SIZE)
    .default(DEFAULT_ACCOUNTS_PAGE_SIZE),
  q: stringSearchParam,
  sort: z
    .union([z.string(), z.array(sortingItemSchema)])
    .optional()
    .transform((value) => encodeSortingParam(value))
    .catch(undefined),
  status: stringSearchParam,
  site: stringSearchParam,
})
