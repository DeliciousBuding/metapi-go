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
    title: 'settings.systemInfo.programLogs.title',
    description: 'settings.systemInfo.programLogs.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.systemInfo.programLogs.title',
        description: 'settings.systemInfo.programLogs.description',
        legacyRef:
          'legacy page: /events (ProgramLogs.tsx) + api.getEvents/getEventCount',
      }),
  },
  {
    id: 'audit-logs',
    title: 'settings.systemInfo.auditLogs.title',
    description: 'settings.systemInfo.auditLogs.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.systemInfo.auditLogs.title',
        description: 'settings.systemInfo.auditLogs.description',
        legacyRef:
          'legacy Settings.tsx: AuditLogsSection (B1) — method/path/actor/IP',
      }),
  },
  {
    id: 'update-center',
    title: 'settings.systemInfo.updateCenter.title',
    description: 'settings.systemInfo.updateCenter.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.systemInfo.updateCenter.title',
        description: 'settings.systemInfo.updateCenter.description',
        legacyRef:
          'legacy Settings.tsx: UpdateCenterSection (UC-1) — getUpdateCenterStatus',
      }),
  },
  {
    id: 'database',
    title: 'settings.systemInfo.database.title',
    description: 'settings.systemInfo.database.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.systemInfo.database.title',
        description: 'settings.systemInfo.database.description',
        legacyRef:
          'legacy Settings.tsx: card 12 — dialect sqlite|postgres + connection + ssl + test/migrate',
      }),
  },
  {
    id: 'maintenance',
    title: 'settings.systemInfo.maintenance.title',
    description: 'settings.systemInfo.maintenance.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.systemInfo.maintenance.title',
        description: 'settings.systemInfo.maintenance.description',
        legacyRef:
          'legacy Settings.tsx: cards 13-14 — clear cache/routes + factory reset',
      }),
  },
] as const

type SystemInfoSectionId = (typeof SYSTEM_INFO_SECTIONS)[number]['id']

const registry = createSectionRegistry<SystemInfoSectionId>({
  sections: SYSTEM_INFO_SECTIONS,
  defaultSection: 'program-logs',
  basePath: '/settings/system-info',
})

export const systemInfoSubarea: SettingsSubarea = {
  id: 'system-info',
  title: 'settings.subareas.systemInfo',
  basePath: '/settings/system-info',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as SystemInfoSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as SystemInfoSectionId),
}
