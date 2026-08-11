// metapi-go/features/sites — Zod schemas for the site form and URL search.
//
// Two schemas live here:
//   1. `siteFormSchema` — validates the add/edit dialog. Numeric fields use
//      plain `z.number()` (no `z.coerce`/`.default()`) so the zod resolver's
//      input and output types match and RHF typing stays clean; the form
//      binds them with an explicit `onChange` that converts via
//      `valueAsNumber`. Error messages are i18next keys (resolved by
//      `<FormMessage>`).
//   2. `sitesSearchSchema` — the URL search-state contract for the list page
//      (pagination / sorting / global filter / status faceted filter). This is
//      the schema a future `/sites` route file should pass to
//      `validateSearch`; the page also safe-parses `window.location.search`
//      directly so it works before the route file lands. `z.coerce` is fine
//      here because the schema is only consumed via `safeParse`.

import { z } from 'zod'

import { encodeSortingParam } from '@/lib/helpers/searchParams'

import type { SiteProbeScope } from '../types'

const HTTP_URL_MESSAGE_KEY = 'sites.form.errors.invalidUrl'
const HTTP_OR_EMPTY_MESSAGE_KEY = 'sites.form.errors.invalidUrlOrEmpty'
const JSON_OR_EMPTY_MESSAGE_KEY = 'sites.form.errors.invalidJson'

function isHttpUrl(value: string): boolean {
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function isEmptyOrHttpUrl(value: string): boolean {
  const trimmed = value.trim()
  if (trimmed.length === 0) return true
  return isHttpUrl(trimmed)
}

function isEmptyOrValidJson(value: string): boolean {
  const trimmed = value.trim()
  if (trimmed.length === 0) return true
  try {
    const parsed: unknown = JSON.parse(trimmed)
    return (
      parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)
    )
  } catch {
    return false
  }
}

export const siteFormSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'sites.form.errors.nameRequired')
    .max(120, 'sites.form.errors.nameTooLong'),
  url: z
    .string()
    .trim()
    .min(1, 'sites.form.errors.urlRequired')
    .refine(isHttpUrl, HTTP_URL_MESSAGE_KEY),
  externalCheckinUrl: z
    .string()
    .refine(isEmptyOrHttpUrl, HTTP_OR_EMPTY_MESSAGE_KEY),
  platform: z.string().trim().max(64, 'sites.form.errors.platformTooLong'),
  proxyUrl: z.string().refine(isEmptyOrHttpUrl, HTTP_OR_EMPTY_MESSAGE_KEY),
  useSystemProxy: z.boolean(),
  customHeaders: z
    .string()
    .refine(isEmptyOrValidJson, JSON_OR_EMPTY_MESSAGE_KEY),
  customHeadersOverrideRequestHeaders: z.boolean(),
  // Numeric fields use plain `z.number()` (no `z.coerce`/`.default()`) so the
  // zod resolver's input and output types match, keeping RHF typing clean.
  // Defaults come from `SITE_FORM_DEFAULT_VALUES`; the form binds these with
  // an explicit `onChange` that converts via `valueAsNumber`.
  globalWeight: z.number().min(0, 'sites.form.errors.globalWeightMin'),
  maxConcurrency: z
    .number()
    .int('sites.form.errors.maxConcurrencyInteger')
    .min(0, 'sites.form.errors.maxConcurrencyMin'),
  postRefreshProbeEnabled: z.boolean(),
  postRefreshProbeModel: z.string().trim(),
  postRefreshProbeScope: z.enum([
    'single',
    'all',
  ] as const satisfies readonly SiteProbeScope[]),
  postRefreshProbeLatencyThresholdMs: z
    .number()
    .int('sites.form.errors.latencyInteger')
    .min(0, 'sites.form.errors.latencyMin'),
})

export type SiteFormValues = z.infer<typeof siteFormSchema>

export const SITE_FORM_DEFAULT_VALUES: SiteFormValues = {
  name: '',
  url: '',
  externalCheckinUrl: '',
  platform: '',
  proxyUrl: '',
  useSystemProxy: false,
  customHeaders: '',
  customHeadersOverrideRequestHeaders: false,
  globalWeight: 1,
  maxConcurrency: 0,
  postRefreshProbeEnabled: false,
  postRefreshProbeModel: '',
  postRefreshProbeScope: 'single',
  postRefreshProbeLatencyThresholdMs: 0,
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

export const sitesSearchSchema = z.object({
  q: z.string().optional(),
  page: z.coerce.number().int().min(0).optional(),
  pageSize: z.coerce.number().int().min(1).max(200).optional(),
  sort: z
    .union([z.string(), z.array(sortingItemSchema)])
    .optional()
    .transform((value) => encodeSortingParam(value)),
  status: z.string().optional(),
})

export const SORTING_ITEM_SCHEMA = sortingItemSchema
export const PAGINATION_SCHEMA = paginationSchema
export const COLUMN_FILTER_ITEM_SCHEMA = columnFilterItemSchema
