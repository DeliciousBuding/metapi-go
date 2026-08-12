// metapi-go/features/oauth — Zod schemas for the start form and URL search.
//
// `oauthStartSchema` validates the "start authorization" dialog. Numeric
// fields are absent here (the form has none). Error messages are i18next
// keys (resolved by `<FormMessage>`). The conditional "projectId required
// when provider.requiresProjectId" check is enforced in the dialog's
// submit handler (it needs the runtime provider list the schema cannot see),
// not in the schema itself.
//
// `oauthSearchSchema` is the URL search-state contract for the connections
// list page (pagination / sorting / global filter / status filter). The page
// safe-parses `window.location.search` directly so it works before the
// `/oauth` route file lands its own `validateSearch`.

import { z } from 'zod'

import {
  encodeSortingParam,
  stringSearchParam,
} from '@/lib/helpers/searchParams'

const HTTP_OR_EMPTY_MESSAGE_KEY = 'oauth.form.errors.invalidProxyUrl'

function isEmptyOrHttpUrl(value: string): boolean {
  const trimmed = value.trim()
  if (trimmed.length === 0) return true
  try {
    const parsed = new URL(trimmed)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

export const oauthStartSchema = z.object({
  provider: z.string().trim().min(1, 'oauth.form.errors.providerRequired'),
  projectId: z.string().trim(),
  proxyUrl: z.string().refine(isEmptyOrHttpUrl, HTTP_OR_EMPTY_MESSAGE_KEY),
  useSystemProxy: z.boolean(),
})

export type OAuthStartValues = z.infer<typeof oauthStartSchema>

export const OAUTH_START_DEFAULT_VALUES: OAuthStartValues = {
  provider: '',
  projectId: '',
  proxyUrl: '',
  useSystemProxy: false,
}

// --- URL search state -------------------------------------------------------

const sortingItemSchema = z.object({
  id: z.string(),
  desc: z.boolean(),
})

const paginationSchema = z.object({
  pageIndex: z.coerce.number().int().min(0).default(0),
  pageSize: z.coerce.number().int().min(1).max(200).default(20),
})

const columnFilterValueSchema = z.union([
  z.string(),
  z.array(z.string()),
  z.boolean(),
])

const columnFilterItemSchema = z.object({
  id: z.string(),
  value: columnFilterValueSchema,
})

export const oauthSearchSchema = z.object({
  q: stringSearchParam,
  page: z.coerce.number().int().min(0).optional(),
  pageSize: z.coerce.number().int().min(1).max(200).optional(),
  sort: z
    .union([z.string(), z.array(sortingItemSchema)])
    .optional()
    .transform((value) => encodeSortingParam(value)),
  status: stringSearchParam,
})

export const OAUTH_SORTING_ITEM_SCHEMA = sortingItemSchema
export const OAUTH_PAGINATION_SCHEMA = paginationSchema
export const OAUTH_COLUMN_FILTER_ITEM_SCHEMA = columnFilterItemSchema
