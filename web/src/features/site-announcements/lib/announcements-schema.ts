// metapi-go/features/site-announcements — Zod schemas for the form + URL
// search.
//
// `announcementFormSchema` validates the add/edit dialog. Error messages
// are i18next keys (resolved by `<FormMessage>`). The `severity` enum uses
// `as const satisfies readonly AnnouncementSeverity[]` so the schema and
// the API type stay in lockstep.
//
// `announcementsSearchSchema` is the URL search-state contract for the list
// page (pagination / sorting / global filter / severity faceted filter /
// enabled filter). The page safe-parses `window.location.search` directly
// so it works before the `/site-announcements` route file lands its own
// `validateSearch`.

import { z } from 'zod'

import type { AnnouncementSeverity } from '../types'

const HTTP_OR_EMPTY_MESSAGE_KEY = 'siteAnnouncements.form.errors.invalidLink'

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

export const announcementFormSchema = z.object({
  title: z
    .string()
    .trim()
    .min(1, 'siteAnnouncements.form.errors.titleRequired')
    .max(200, 'siteAnnouncements.form.errors.titleTooLong'),
  message: z
    .string()
    .trim()
    .min(1, 'siteAnnouncements.form.errors.messageRequired')
    .max(5000, 'siteAnnouncements.form.errors.messageTooLong'),
  severity: z.enum(
    ['info', 'warning', 'critical'] as const satisfies readonly AnnouncementSeverity[],
  ),
  link: z.string().refine(isEmptyOrHttpUrl, HTTP_OR_EMPTY_MESSAGE_KEY),
  enabled: z.boolean(),
})

export type AnnouncementFormValues = z.infer<typeof announcementFormSchema>

export const ANNOUNCEMENT_FORM_DEFAULT_VALUES: AnnouncementFormValues = {
  title: '',
  message: '',
  severity: 'info',
  link: '',
  enabled: true,
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

export const announcementsSearchSchema = z.object({
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
  severity: z.string().optional(),
  enabled: z.string().optional(),
})

export type AnnouncementsSearch = z.infer<typeof announcementsSearchSchema>

export const ANNOUNCEMENTS_SORTING_ITEM_SCHEMA = sortingItemSchema
export const ANNOUNCEMENTS_PAGINATION_SCHEMA = paginationSchema
export const ANNOUNCEMENTS_COLUMN_FILTER_ITEM_SCHEMA = columnFilterItemSchema
