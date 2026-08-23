// metapi-go/features/settings/sections/operations — System & Ops subarea
// (wave 9 lane B): scheduling + database/data-migration + maintenance +
// danger zone + logs + update center. Absorbs the retired `system-info`
// subarea and scheduling (from the retired `general` subarea). The data
// migration action was split out of the database page into its own section
// (wave 9 lane B, P1). Each section is React.lazy so its form/table
// dependencies land in a separate async chunk; the surrounding Suspense
// boundary lives in settings-page.tsx.

import { ServerCog } from 'lucide-react'
import { createElement, lazy } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'

const LazySchedulingSection = lazy(() =>
  import('./components/scheduling-section').then((module) => ({
    default: module.SchedulingSection,
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
const LazyDangerZoneSection = lazy(() =>
  import('./components/danger-zone-section').then((module) => ({
    default: module.DangerZoneSection,
  }))
)

const OPERATIONS_SECTIONS = [
  {
    id: 'scheduling',
    title: 'settings.operations.scheduling.title',
    description: 'settings.operations.scheduling.description',
    build: () => createElement(LazySchedulingSection),
  },
  {
    id: 'database',
    title: 'settings.operations.database.title',
    description: 'settings.operations.database.description',
    build: () => createElement(LazyDatabaseSection),
  },
  {
    id: 'data-migration',
    title: 'settings.operations.dataMigration.title',
    description: 'settings.operations.dataMigration.description',
    build: () => createElement(LazyDatabaseMigrationSection),
  },
  {
    id: 'maintenance',
    title: 'settings.operations.maintenance.title',
    description: 'settings.operations.maintenance.description',
    build: () => createElement(LazyMaintenanceSection),
  },
  {
    id: 'program-logs',
    title: 'settings.operations.programLogs.title',
    description: 'settings.operations.programLogs.description',
    build: () => createElement(LazyProgramLogsSection),
  },
  {
    id: 'audit-logs',
    title: 'settings.operations.auditLogs.title',
    description: 'settings.operations.auditLogs.description',
    readonly: true,
    build: () => createElement(LazyAuditLogsSection),
  },
  {
    id: 'update-center',
    title: 'settings.operations.updateCenter.title',
    description: 'settings.operations.updateCenter.description',
    readonly: true,
    build: () => createElement(LazyUpdateCenterSection),
  },
  {
    id: 'danger-zone',
    title: 'settings.operations.dangerZone.title',
    description: 'settings.operations.dangerZone.description',
    build: () => createElement(LazyDangerZoneSection),
  },
] as const

type OperationsSectionId = (typeof OPERATIONS_SECTIONS)[number]['id']

const registry = createSectionRegistry<OperationsSectionId>({
  sections: OPERATIONS_SECTIONS,
  defaultSection: 'program-logs',
  basePath: '/settings/operations',
})

export const operationsSubarea: SettingsSubarea = {
  id: 'operations',
  title: 'settings.subareas.operations',
  description: 'settings.subareas.operations.description',
  icon: ServerCog,
  basePath: '/settings/operations',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as OperationsSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as OperationsSectionId),
}
