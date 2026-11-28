import { z } from 'zod'

import type { Announcement } from '@/lib/api'
import { isValidProductAnnouncementLink } from '@/lib/product-announcements'

export const announcementSchema = z.object({
  title: z
    .string()
    .min(1, 'settings.content.announcements.schema.titleRequired'),
  message: z
    .string()
    .min(1, 'settings.content.announcements.schema.messageRequired'),
  severity: z.enum(['info', 'warning', 'critical']),
  link: z
    .string()
    .refine(
      isValidProductAnnouncementLink,
      'settings.content.announcements.schema.linkInvalid'
    )
    .optional(),
  enabled: z.boolean().optional(),
})

export type AnnouncementFormValues = z.infer<typeof announcementSchema>

export type EditMode =
  | { kind: 'create' }
  | { kind: 'edit'; announcement: Announcement }
  | null
