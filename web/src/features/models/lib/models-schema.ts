// metapi-go/features/models — Zod schema for the marketplace URL search state.
//
// This is the URL contract for the models list page (search query / page /
// page-size / sorting / capability / endpoint-type / supply-source filters).
// It mirrors `sitesSearchSchema` in shape: a future `/models` route file
// should pass this to TanStack Router `validateSearch`; the page also
// safe-parses `window.location.search` directly so it works before the
// route file lands. `z.coerce` is fine here because the schema is only
// consumed via `safeParse`.
//
// The faceted filters are multi-select: their URL form is a comma-separated
// string (e.g. endpointType=openai,anthropic). The server owns filter
// semantics (OR within one dimension, AND across dimensions); these params
// are forwarded verbatim.
// `transform` turns them into arrays for the data-table column filter state.

import { z } from 'zod'

import {
  encodeSortingParam,
  encodeStringListParam,
  stringSearchParam,
  tableSortingItemSchema,
} from '@/lib/helpers/searchParams'

export const modelsSearchSchema = z.object({
  q: stringSearchParam,
  page: z.coerce.number().int().min(0).catch(0).default(0),
  pageSize: z.coerce.number().int().min(1).max(200).catch(20).default(20),
  sort: z
    .union([z.string(), z.array(tableSortingItemSchema)])
    .optional()
    .transform((value) => encodeSortingParam(value))
    .catch(undefined),
  // Multi-select faceted filters, URL-encoded as comma-separated strings.
  // The brand dimension is intentionally absent: the name-to-brand mapping is
  // a client-side visual identity (BrandIcon/getBrand) with no server owner,
  // so a server filter could not be truthful; the q search covers the intent.
  capability: z
    .union([z.string(), z.array(z.string())])
    .optional()
    .transform((value) => encodeStringListParam(value))
    .catch(undefined),
  endpointType: z
    .union([z.string(), z.array(z.string())])
    .optional()
    .transform((value) => encodeStringListParam(value))
    .catch(undefined),
  sourceKind: z
    .union([z.string(), z.array(z.string())])
    .optional()
    .transform((value) => encodeStringListParam(value))
    .catch(undefined),
})
