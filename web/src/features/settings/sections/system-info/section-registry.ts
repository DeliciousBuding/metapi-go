// metapi-go/features/settings/sections/system-info — System Info subarea.
// Scope (plan §5.5.2): program logs + audit logs + update center + database
// migration + maintenance tools + danger zone. All six sections wired to real
// surfaces under ./components.
// Each section is React.lazy so its form/table dependencies land in a separate
// async chunk; the surrounding Suspense boundary lives in settings-page.tsx.

import { ServerCog } from 'lucide-react'
import { createElement, lazy } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'

const LazyProgramLogsSection = lazy(() =>
  import('./components/program-logs-section').then((module) => ({
    default: module.ProgramLogsSection,
  }))
)
const LazyAuditLogsSection = lazy(() =>
  import('./components/audit-logs-section').then((module) => ({
    default: module.AuditLogsSection,
  }))
)
const LazyUpdateCenterSection = lazy(() =>
  import('./components/update-center-section').then((module) => ({
    default: module.UpdateCenterSection,
  }))
)
const LazyDatabaseSection = lazy(() =>
  import('./components/database-section').then((module) => ({
    default: module.DatabaseSection,
  }))
)
const LazyDatabaseMigrationSection = lazy(() =>
  import('./components/database-migration-section').then((module) => ({
    default: module.DatabaseMigrationSection,
  }))
)
const LazyMaintenanceSection = lazy(() =>
  import('./components/maintenance-section').then((module) => ({
    default: module.MaintenanceSection,
  }))
)
const LazyDangerZoneSection = lazy(() =>
  import('./components/danger-zone-section').then((module) => ({
    default: module.DangerZoneSection,
  }))
)

const SYSTEM_INFO_SECTIONS = [
  {
    id: 'program-logs',
    title: 'settings.systemInfo.programLogs.title',
    group: 'settings.systemInfo.groups.logs',
    description: 'settings.systemInfo.programLogs.description',
    build: () => createElement(LazyProgramLogsSection),
  },
  {
    id: 'audit-logs',
    title: 'settings.systemInfo.auditLogs.title',
    group: 'settings.systemInfo.groups.logs',
    description: 'settings.systemInfo.auditLogs.description',
    readonly: true,
    build: () => createElement(LazyAuditLogsSection),
  },
  {
    id: 'update-center',
    title: 'settings.systemInfo.updateCenter.title',
    group: 'settings.systemInfo.groups.system',
    description: 'settings.systemInfo.updateCenter.description',
    readonly: true,
    build: () => createElement(LazyUpdateCenterSection),
  },
  {
    id: 'database',
    title: 'settings.systemInfo.database.title',
    group: 'settings.systemInfo.groups.system',
    description: 'settings.systemInfo.database.description',
    build: () => createElement(LazyDatabaseSection),
  },
  {
    id: 'data-migration',
    title: 'settings.systemInfo.dataMigration.title',
    group: 'settings.systemInfo.groups.system',
    description: 'settings.systemInfo.dataMigration.description',
    build: () => createElement(LazyDatabaseMigrationSection),
  },
  {
    id: 'maintenance',
    title: 'settings.systemInfo.maintenance.title',
    group: 'settings.systemInfo.groups.system',
    description: 'settings.systemInfo.maintenance.description',
    build: () => createElement(LazyMaintenanceSection),
  },
  {
    id: 'danger-zone',
    title: 'settings.systemInfo.dangerZone.title',
    group: 'settings.systemInfo.groups.dangerZone',
    description: 'settings.systemInfo.dangerZone.description',
    build: () => createElement(LazyDangerZoneSection),
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
  description: 'settings.subareas.systemInfo.description',
  icon: ServerCog,
  basePath: '/settings/system-info',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as SystemInfoSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as SystemInfoSectionId),
}
