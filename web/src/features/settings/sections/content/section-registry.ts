// metapi-go/features/settings/sections/content — Content subarea.
// Scope (plan §5.5.2): import/export + notification channels + risk-banner
// announcements. All three sections wired to real forms under ./components.
// Each section is React.lazy so its form/table dependencies land in a separate
// async chunk; the surrounding Suspense boundary lives in settings-page.tsx.

import { MessagesSquare } from 'lucide-react'
import { createElement, lazy } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'

const LazyImportExportSection = lazy(() =>
  import('./components/import-export-section').then((module) => ({
    default: module.ImportExportSection,
  }))
)
const LazyNotificationsSection = lazy(() =>
  import('./components/notifications-section').then((module) => ({
    default: module.NotificationsSection,
  }))
)
const LazyAnnouncementsSection = lazy(() =>
  import('./components/announcements-section').then((module) => ({
    default: module.AnnouncementsSection,
  }))
)

const CONTENT_SECTIONS = [
  {
    id: 'import-export',
    title: 'settings.content.importExport.title',
    group: 'settings.content.groups.data',
    description: 'settings.content.importExport.description',
    build: () => createElement(LazyImportExportSection),
  },
  {
    id: 'notifications',
    title: 'settings.content.notifications.title',
    group: 'settings.content.groups.messaging',
    description: 'settings.content.notifications.description',
    build: () => createElement(LazyNotificationsSection),
  },
  {
    id: 'announcements',
    title: 'settings.content.announcements.title',
    group: 'settings.content.groups.messaging',
    description: 'settings.content.announcements.description',
    build: () => createElement(LazyAnnouncementsSection),
  },
] as const

type ContentSectionId = (typeof CONTENT_SECTIONS)[number]['id']

const registry = createSectionRegistry<ContentSectionId>({
  sections: CONTENT_SECTIONS,
  defaultSection: 'notifications',
  basePath: '/settings/content',
})

export const contentSubarea: SettingsSubarea = {
  id: 'content',
  title: 'settings.subareas.content',
  description: 'settings.subareas.content.description',
  icon: MessagesSquare,
  basePath: '/settings/content',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as ContentSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as ContentSectionId),
}
