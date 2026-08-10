// metapi-go/features/site-announcements — barrel re-exports.

export { AnnouncementsPage } from './components/announcements-page'
export { AnnouncementFormDialog } from './components/announcement-form-dialog'
export { useAnnouncementsColumns } from './components/announcements-columns'

export {
  useAnnouncements,
  useCreateAnnouncement,
  useUpdateAnnouncement,
  useDeleteAnnouncement,
} from './api'

export {
  announcementFormSchema,
  announcementsSearchSchema,
  ANNOUNCEMENT_FORM_DEFAULT_VALUES,
  type AnnouncementFormValues,
  type AnnouncementsSearch,
} from './lib/announcements-schema'

export {
  announcementsKeys,
  type SiteAnnouncement,
  type AnnouncementSeverity,
  type AnnouncementFormPayload,
} from './types'
