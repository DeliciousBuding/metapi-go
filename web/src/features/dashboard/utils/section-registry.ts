// metapi-go/features/dashboard/utils — createSectionRegistry generic factory.
// Adapted from newapi's features/system-settings/utils/section-registry.ts
// (itself ported into metapi-go by the settings feature) per plan.md §5.5.1.
//
// Simplifications for the phase 2 dashboard:
//   - No `TSettings` type param (dashboard sections are self-contained; they
//     fetch their own data via TanStack Query in phase 3, so `build` takes no
//     args and renders real chart wiring fed by stub arrays for now).
//   - Nav items use plain `title` strings (no TFunction); i18n lands phase 2,
//     matching the settings feature convention.
//   - urlStyle is always 'path' — section URLs are `${basePath}/${id}` so the
//     URL carries section context (e.g. /dashboard/traffic).

import type { ReactNode } from 'react'

import type {
  DashboardSection,
  DashboardSectionNavItem,
} from '../types'

/**
 * Registry config supplied to {@link createSectionRegistry}.
 */
export type SectionRegistryConfig<TSectionId extends string> = {
  sections: readonly DashboardSection[]
  defaultSection: TSectionId
  /** Base path, e.g. '/dashboard'. Section URLs become `${basePath}/${id}`. */
  basePath: string
}

/**
 * Registry returned by {@link createSectionRegistry}.
 */
export type SectionRegistry<TSectionId extends string> = {
  /** All section ids (typed as the dashboard's literal union). */
  sectionIds: readonly TSectionId[]
  /** Section navigated to when no `$section` param is present. */
  defaultSection: TSectionId
  /** Tab nav items (one per section). */
  getSectionNavItems: () => DashboardSectionNavItem[]
  /** Render the content for a section (falls back to sections[0] on unknown id). */
  getSectionContent: (sectionId: TSectionId) => ReactNode
  /** Look up a section's metadata (falls back to sections[0] on unknown id). */
  getSectionMeta: (sectionId: TSectionId) => DashboardSection
}

/**
 * Create a section registry with helper functions.
 *
 * The dashboard calls this once to produce the nav + content surface consumed
 * by `DashboardPage` (the section dispatcher). The generic `TSectionId` lets
 * the dashboard narrow its section ids to a literal union for compile-time
 * safety at the call site.
 */
export function createSectionRegistry<TSectionId extends string>(
  config: SectionRegistryConfig<TSectionId>,
): SectionRegistry<TSectionId> {
  const { sections, defaultSection, basePath } = config

  const sectionIds = sections.map(
    (section) => section.id,
  ) as unknown as readonly TSectionId[]

  function getSectionMeta(sectionId: TSectionId): DashboardSection {
    return (
      sections.find((section) => section.id === sectionId) ?? sections[0]
    )
  }

  function getSectionNavItems(): DashboardSectionNavItem[] {
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
