// metapi-go/features/settings/sections/system-info — System Info subarea.
// Scope (plan §5.5.2): program logs + audit logs + update center + database
// migration + maintenance tools + danger zone. All six sections wired to real
// surfaces under ./components.

import { ServerCog } from 'lucide-react'
import { createElement } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'
import { AuditLogsSection } from './components/audit-logs-section'
import { DangerZoneSection } from './components/danger-zone-section'
import { DatabaseSection } from './components/database-section'
import { MaintenanceSection } from './components/maintenance-section'
import { ProgramLogsSection } from './components/program-logs-section'
import { UpdateCenterSection } from './components/update-center-section'

const SYSTEM_INFO_SECTIONS = [
  {
    id: 'program-logs',
    title: 'settings.systemInfo.programLogs.title',
    group: 'settings.systemInfo.groups.logs',
    description: 'settings.systemInfo.programLogs.description',
    build: () => createElement(ProgramLogsSection),
  },
  {
    id: 'audit-logs',
    title: 'settings.systemInfo.auditLogs.title',
    group: 'settings.systemInfo.groups.logs',
    description: 'settings.systemInfo.auditLogs.description',
    readonly: true,
    build: () => createElement(AuditLogsSection),
  },
  {
    id: 'update-center',
    title: 'settings.systemInfo.updateCenter.title',
    group: 'settings.systemInfo.groups.system',
    description: 'settings.systemInfo.updateCenter.description',
    readonly: true,
    build: () => createElement(UpdateCenterSection),
  },
  {
    id: 'database',
    title: 'settings.systemInfo.database.title',
    group: 'settings.systemInfo.groups.system',
    description: 'settings.systemInfo.database.description',
    build: () => createElement(DatabaseSection),
  },
  {
    id: 'maintenance',
    title: 'settings.systemInfo.maintenance.title',
    group: 'settings.systemInfo.groups.system',
    description: 'settings.systemInfo.maintenance.description',
    build: () => createElement(MaintenanceSection),
  },
  {
    id: 'danger-zone',
    title: 'settings.systemInfo.dangerZone.title',
    group: 'settings.systemInfo.groups.dangerZone',
    description: 'settings.systemInfo.dangerZone.description',
    build: () => createElement(DangerZoneSection),
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
