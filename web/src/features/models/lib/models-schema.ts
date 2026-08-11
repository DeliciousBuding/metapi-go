// metapi-go/features/models — Zod schema for the marketplace URL search state.
//
// This is the URL contract for the models list page (search query / page /
// page-size / sorting / brand faceted filter / capability faceted filter).
// It mirrors `sitesSearchSchema` in shape: a future `/models` route file
// should pass this to TanStack Router `validateSearch`; the page also
// safe-parses `window.location.search` directly so it works before the
// route file lands. `z.coerce` is fine here because the schema is only
// consumed via `safeParse`.
//
// The brand and capability filters are multi-select faceted filters: their
// URL form is a comma-separated string (e.g. `brand=openai,anthropic`).
// `transform` turns them into arrays for the data-table column filter state.

import { z } from 'zod'

const sortingItemSchema = z.object({
  id: z.string(),
  desc: z.boolean(),
})

const paginationSchema = z.object({
  pageIndex: z.coerce.number().int().min(0).default(0),
  pageSize: z.coerce.number().int().min(1).max(200).default(20),
})

function decodeStringList(value: string | undefined): string[] {
  if (!value) return []
  return value
    .split(',')
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0)
}

export const modelsSearchSchema = z.object({
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
  // Multi-select faceted filters, URL-encoded as comma-separated strings.
  brand: z
    .string()
    .optional()
    .transform((value) => decodeStringList(value)),
  capability: z
    .string()
    .optional()
    .transform((value) => decodeStringList(value)),
})


export const SORTING_ITEM_SCHEMA = sortingItemSchema
export const PAGINATION_SCHEMA = paginationSchema
