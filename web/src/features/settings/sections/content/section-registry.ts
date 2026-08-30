// metapi-go/features/settings/sections/content — Notify & Data subarea
// (wave 9 lane B): notification channels + risk-banner announcements
// (messages) + import/export backup (data). Retitled from "data & messages";
// the section set is unchanged, only the front-load order puts messaging
// first. Each section is React.lazy so its form/table dependencies land in a
// separate async chunk; the surrounding Suspense boundary lives in
// settings-page.tsx.

import { MessagesSquare } from 'lucide-react'
import { createElement, lazy } from 'react'

import { createSectionRegistry } from '@/lib/section-registry'

import type { SettingsSubarea } from '../../types'

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
const LazyImportExportSection = lazy(() =>
  import('./components/import-export-section').then((module) => ({
    default: module.ImportExportSection,
  }))
)

const CONTENT_SECTIONS = [
  {
    id: 'notifications',
    title: 'settings.content.notifications.title',
    description: 'settings.content.notifications.description',
    build: () => createElement(LazyNotificationsSection),
  },
  {
    id: 'announcements',
    title: 'settings.content.announcements.title',
    description: 'settings.content.announcements.description',
    build: () => createElement(LazyAnnouncementsSection),
  },
  {
    id: 'import-export',
    title: 'settings.content.importExport.title',
    description: 'settings.content.importExport.description',
    build: () => createElement(LazyImportExportSection),
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
