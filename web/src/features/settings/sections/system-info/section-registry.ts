// metapi-go/features/settings/sections/system-info — System Info subarea.
// Scope (plan §5.5.2): program logs + audit logs + update center + database
// migration + maintenance/danger zone.
// Sections: program-logs, audit-logs, update-center, database, maintenance.
// Phase 2 stubs; phase 3 migrates from the legacy /events page and the
// appended sections + Settings.tsx cards 12-14.
//
// .ts (no JSX) so react/only-export-components does not apply; section content
// is built with React.createElement (hooks-safe, phase-3 ready).

import { createElement } from 'react'

import { StubSection } from '../../components/stub-section'
import { createSectionRegistry } from '../../utils/section-registry'
import type { SettingsSubarea } from '../../types'

const SYSTEM_INFO_SECTIONS = [
  {
    id: 'program-logs',
    title: 'Program Logs',
    description: 'Operational event log (events stream).',
    build: () =>
      createElement(StubSection, {
        title: 'Program Logs',
        description: 'Operational event log (events stream).',
        legacyRef:
          'legacy page: /events (ProgramLogs.tsx) + api.getEvents/getEventCount',
      }),
  },
  {
    id: 'audit-logs',
    title: 'Admin Audit Logs',
    description: 'Read-only admin write-operation audit (B1).',
    build: () =>
      createElement(StubSection, {
        title: 'Admin Audit Logs',
        description: 'Read-only admin write-operation audit (B1).',
        legacyRef:
          'legacy Settings.tsx: AuditLogsSection (B1) — method/path/actor/IP',
      }),
  },
  {
    id: 'update-center',
    title: 'Update Center',
    description: 'Version + deploy/rollback status (UC-1, read-only).',
    build: () =>
      createElement(StubSection, {
        title: 'Update Center',
        description: 'Version + deploy/rollback status (UC-1, read-only).',
        legacyRef:
          'legacy Settings.tsx: UpdateCenterSection (UC-1) — getUpdateCenterStatus',
      }),
  },
  {
    id: 'database',
    title: 'Database Migration',
    description: 'Runtime DB dialect / connection / migrate (CLI-only).',
    build: () =>
      createElement(StubSection, {
        title: 'Database Migration',
        description: 'Runtime DB dialect / connection / migrate (CLI-only).',
        legacyRef:
          'legacy Settings.tsx: card 12 — dialect sqlite|postgres + connection + ssl + test/migrate',
      }),
  },
  {
    id: 'maintenance',
    title: 'Maintenance & Danger Zone',
    description: 'Cache rebuild, log purge, and factory reset.',
    build: () =>
      createElement(StubSection, {
        title: 'Maintenance & Danger Zone',
        description: 'Cache rebuild, log purge, and factory reset.',
        legacyRef:
          'legacy Settings.tsx: cards 13-14 — clear cache/routes + factory reset',
      }),
  },
] as const

export type SystemInfoSectionId = (typeof SYSTEM_INFO_SECTIONS)[number]['id']

const registry = createSectionRegistry<SystemInfoSectionId>({
  sections: SYSTEM_INFO_SECTIONS,
  defaultSection: 'program-logs',
  basePath: '/settings/system-info',
})

export const SYSTEM_INFO_SECTION_IDS = registry.sectionIds
export const SYSTEM_INFO_DEFAULT_SECTION = registry.defaultSection
export const getSystemInfoSectionNavItems = registry.getSectionNavItems
export const getSystemInfoSectionContent = registry.getSectionContent
export const getSystemInfoSectionMeta = registry.getSectionMeta

export const systemInfoSubarea: SettingsSubarea = {
  id: 'system-info',
  title: 'System Info',
  basePath: '/settings/system-info',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as SystemInfoSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as SystemInfoSectionId),
}
