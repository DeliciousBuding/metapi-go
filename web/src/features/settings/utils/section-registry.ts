// metapi-go/features/settings — createSectionRegistry generic factory.
// Adapted from newapi's system-settings/utils/section-registry.ts per
// plan.md §5.5.2.
//
// Simplifications for the phase 2 skeleton:
//   - No `TSettings` type param (phase 3 will reintroduce the merged
//     /api/settings/runtime key-value map + react-hook-form sections; for
//     now `build` takes no args and stub sections render a TODO card).
//   - Nav items use plain `title` strings (no TFunction); i18n lands phase 2,
//     matching components/layout/config/system-settings.config.ts.
//   - urlStyle is always 'path' — section URLs are `${basePath}/${id}` so the
//     URL carries both subarea and section context during drill-in.

import type { ReactNode } from 'react'

import type {
  SettingsSection,
  SettingsSectionNavItem,
} from '../types'

/**
 * Registry config supplied to {@link createSectionRegistry}.
 */
export type SectionRegistryConfig<TSectionId extends string> = {
  sections: readonly SettingsSection[]
  defaultSection: TSectionId
  /** Base path, e.g. '/settings/general'. Section URLs become `${basePath}/${id}`. */
  basePath: string
}

/**
 * Registry returned by {@link createSectionRegistry}.
 */
export type SectionRegistry<TSectionId extends string> = {
  /** All section ids (typed as the subarea's literal union). */
  sectionIds: readonly TSectionId[]
  /** Section navigated to when no `$section` param is present. */
  defaultSection: TSectionId
  /** Sidebar nav items (one per section). */
  getSectionNavItems: () => SettingsSectionNavItem[]
  /** Render the content for a section (falls back to sections[0] on unknown id). */
  getSectionContent: (sectionId: TSectionId) => ReactNode
  /** Look up a section's metadata (falls back to sections[0] on unknown id). */
  getSectionMeta: (sectionId: TSectionId) => SettingsSection
}

/**
 * Create a section registry with helper functions.
 *
 * Each subarea (general/downstream/models/content/system-info) calls this once
 * to produce the nav + content surface consumed by `SettingsPage` and the
 * settings sidebar. The generic `TSectionId` lets each subarea narrow its
 * section ids to a literal union for compile-time safety at the call site.
 */
export function createSectionRegistry<TSectionId extends string>(
  config: SectionRegistryConfig<TSectionId>,
): SectionRegistry<TSectionId> {
  const { sections, defaultSection, basePath } = config

  const sectionIds = sections.map(
    (section) => section.id,
  ) as unknown as readonly TSectionId[]

  function getSectionMeta(sectionId: TSectionId): SettingsSection {
    return (
      sections.find((section) => section.id === sectionId) ?? sections[0]
    )
  }

  function getSectionNavItems(): SettingsSectionNavItem[] {
    return sections.map((section) => ({
      title: section.title,
      url: `${basePath}/${section.id}`,
    }))
  }

  function getSectionContent(sectionId: TSectionId): ReactNode {
    return getSectionMeta(sectionId).build()
  }

  return {
    sectionIds,
    defaultSection,
    getSectionNavItems,
    getSectionContent,
    getSectionMeta,
  }
}
