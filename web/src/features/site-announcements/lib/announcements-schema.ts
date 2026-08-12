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

import { encodeSortingParam, stringSearchParam } from '@/lib/helpers/searchParams'

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

export const announcementsSearchSchema = z.object({
  q: stringSearchParam,
  page: z.coerce.number().int().min(0).optional(),
  pageSize: z.coerce.number().int().min(1).max(200).optional(),
  sort: z
    .union([z.string(), z.array(sortingItemSchema)])
    .optional()
    .transform((value) => encodeSortingParam(value)),
  severity: stringSearchParam,
  enabled: stringSearchParam,
})


export const ANNOUNCEMENTS_PAGINATION_SCHEMA = paginationSchema
