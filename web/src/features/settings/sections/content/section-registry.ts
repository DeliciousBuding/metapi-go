// metapi-go/features/settings/sections/content — Content subarea.
// Scope (plan §5.5.2): import/export + notification channels + risk-banner
// announcements. All three sections wired to real forms under ./components.

import { createElement } from 'react'

import { createSectionRegistry } from '../../utils/section-registry'
import type { SettingsSubarea } from '../../types'
import { AnnouncementsSection } from './components/announcements-section'
import { ImportExportSection } from './components/import-export-section'
import { NotificationsSection } from './components/notifications-section'

const CONTENT_SECTIONS = [
  {
    id: 'import-export',
    title: 'settings.content.importExport.title',
    description: 'settings.content.importExport.description',
    build: () => createElement(ImportExportSection),
  },
  {
    id: 'notifications',
    title: 'settings.content.notifications.title',
    description: 'settings.content.notifications.description',
    build: () => createElement(NotificationsSection),
  },
  {
    id: 'announcements',
    title: 'settings.content.announcements.title',
    description: 'settings.content.announcements.description',
    build: () => createElement(AnnouncementsSection),
  },
] as const

type ContentSectionId = (typeof CONTENT_SECTIONS)[number]['id']

const registry = createSectionRegistry<ContentSectionId>({
  sections: CONTENT_SECTIONS,
  defaultSection: 'import-export',
  basePath: '/settings/content',
})

export const contentSubarea: SettingsSubarea = {
  id: 'content',
  title: 'settings.subareas.content',
  basePath: '/settings/content',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as ContentSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as ContentSectionId),
}
