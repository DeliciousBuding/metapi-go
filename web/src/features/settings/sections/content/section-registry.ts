// metapi-go/features/settings/sections/content — Content subarea.
// Scope (plan §5.5.2): import/export + notification channels + risk-banner
// announcements.
// Sections: import-export, notifications, announcements.
// Phase 2 stubs; phase 3 migrates from the legacy /settings/import-export,
// /settings/notify pages, and AnnouncementsSection.
//
// .ts (no JSX) so react/only-export-components does not apply; section content
// is built with React.createElement (hooks-safe, phase-3 ready).

import { createElement } from 'react'

import { StubSection } from '../../components/stub-section'
import { createSectionRegistry } from '../../utils/section-registry'
import type { SettingsSubarea } from '../../types'

const CONTENT_SECTIONS = [
  {
    id: 'import-export',
    title: 'settings.content.importExport.title',
    description: 'settings.content.importExport.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.content.importExport.title',
        description: 'settings.content.importExport.description',
        legacyRef: 'legacy page: /settings/import-export (ImportExport.tsx)',
      }),
  },
  {
    id: 'notifications',
    title: 'settings.content.notifications.title',
    description: 'settings.content.notifications.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.content.notifications.title',
        description: 'settings.content.notifications.description',
        legacyRef:
          'legacy page: /settings/notify (NotificationSettings.tsx) + RuntimeSettingsPayload notify fields',
      }),
  },
  {
    id: 'announcements',
    title: 'settings.content.announcements.title',
    description: 'settings.content.announcements.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.content.announcements.title',
        description: 'settings.content.announcements.description',
        legacyRef:
          'legacy Settings.tsx: AnnouncementsSection (H1) — CRUD + revision resets dismissals',
      }),
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
