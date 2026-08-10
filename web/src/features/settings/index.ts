// metapi-go/features/settings — barrel.
//
// Public API for the 5-subarea drill-in Settings workspace (plan §5.5.2).
// Consumers (route files, the main sidebar, future feature modules) import
// from `@/features/settings`.

// Types
export type {
  SettingsSection,
  SettingsSectionNavItem,
  SettingsSubarea,
  SettingsSubareaId,
} from './types'

// Generic factory + adapter types
export {
  createSectionRegistry,
  type SectionRegistry,
  type SectionRegistryConfig,
} from './utils/section-registry'

// Generic dispatcher + in-page sidebar + stub content
export { SettingsPage } from './components/settings-page'
export { SettingsSidebar } from './components/settings-sidebar'
export { StubSection } from './components/stub-section'

// 5-subarea manifest + validation helpers (route registration surface)
export {
  SETTINGS_SUBAREAS,
  SETTINGS_SUBAREA_IDS,
  getSettingsSubarea,
  resolveDefaultSection,
  isValidSection,
} from './config/settings-config'

// Per-subarea typed registries (for route files that want compile-time ids)
export {
  GENERAL_SECTION_IDS,
  GENERAL_DEFAULT_SECTION,
  getGeneralSectionNavItems,
  getGeneralSectionContent,
  getGeneralSectionMeta,
  generalSubarea,
  type GeneralSectionId,
} from './sections/general'
export {
  DOWNSTREAM_SECTION_IDS,
  DOWNSTREAM_DEFAULT_SECTION,
  getDownstreamSectionNavItems,
  getDownstreamSectionContent,
  getDownstreamSectionMeta,
  downstreamSubarea,
  type DownstreamSectionId,
} from './sections/downstream'
export {
  MODELS_SECTION_IDS,
  MODELS_DEFAULT_SECTION,
  getModelsSectionNavItems,
  getModelsSectionContent,
  getModelsSectionMeta,
  modelsSubarea,
  type ModelsSectionId,
} from './sections/models'
export {
  CONTENT_SECTION_IDS,
  CONTENT_DEFAULT_SECTION,
  getContentSectionNavItems,
  getContentSectionContent,
  getContentSectionMeta,
  contentSubarea,
  type ContentSectionId,
} from './sections/content'
export {
  SYSTEM_INFO_SECTION_IDS,
  SYSTEM_INFO_DEFAULT_SECTION,
  getSystemInfoSectionNavItems,
  getSystemInfoSectionContent,
  getSystemInfoSectionMeta,
  systemInfoSubarea,
  type SystemInfoSectionId,
} from './sections/system-info'
