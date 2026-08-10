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
    title: 'Import / Export',
    description: 'Backup and restore runtime configuration.',
    build: () =>
      createElement(StubSection, {
        title: 'Import / Export',
        description: 'Backup and restore runtime configuration.',
        legacyRef: 'legacy page: /settings/import-export (ImportExport.tsx)',
      }),
  },
  {
    id: 'notifications',
    title: 'Notification Channels',
    description:
      'Webhook / Bark / Telegram / SMTP / Feishu / DingTalk / WeCom / NTFY.',
    build: () =>
      createElement(StubSection, {
        title: 'Notification Channels',
        description:
          'Webhook / Bark / Telegram / SMTP / Feishu / DingTalk / WeCom / NTFY.',
        legacyRef:
          'legacy page: /settings/notify (NotificationSettings.tsx) + RuntimeSettingsPayload notify fields',
      }),
  },
  {
    id: 'announcements',
    title: 'Risk Banner Announcements',
    description: 'Product risk banners (H1) — draft / severity / enable.',
    build: () =>
      createElement(StubSection, {
        title: 'Risk Banner Announcements',
        description: 'Product risk banners (H1) — draft / severity / enable.',
        legacyRef:
          'legacy Settings.tsx: AnnouncementsSection (H1) — CRUD + revision resets dismissals',
      }),
  },
] as const

export type ContentSectionId = (typeof CONTENT_SECTIONS)[number]['id']

const registry = createSectionRegistry<ContentSectionId>({
  sections: CONTENT_SECTIONS,
  defaultSection: 'import-export',
  basePath: '/settings/content',
})

export const CONTENT_SECTION_IDS = registry.sectionIds
export const CONTENT_DEFAULT_SECTION = registry.defaultSection
export const getContentSectionNavItems = registry.getSectionNavItems
export const getContentSectionContent = registry.getSectionContent
export const getContentSectionMeta = registry.getSectionMeta

export const contentSubarea: SettingsSubarea = {
  id: 'content',
  title: 'Content',
  basePath: '/settings/content',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as ContentSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as ContentSectionId),
}
