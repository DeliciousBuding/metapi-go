// metapi-go/lib — createSectionRegistry generic factory (S8 三合一).
//
// Single owner for the section-registry pattern previously cloned three
// times (settings / dashboard / observability). Each feature registers a
// set of sections and gets back nav items plus a content/meta lookup; the
// generic `TSectionId` narrows the section ids to a literal union for
// compile-time safety at the call site.
//
// Two URL conventions exist in the product: path-style (`/settings/<sub>/
// <id>`, `/dashboard/<id>`) and query-style (`/observability?section=<id>`)
// — pick via `urlStyle` (default 'path').

import type { ReactNode } from 'react'

/**
 * The shape every registry section must satisfy. Features may carry extra
 * fields (the factory is generic over the concrete section type, so
 * `getSectionMeta` returns the feature's own section type).
 */
export type SectionDefinition = {
  /** Stable id used in the URL. */
  id: string
  /** Human label shown in the nav. */
  title: string
  /** Optional description shown under the page header. */
  description?: string
  /** Read-only / external surface — rendered as a nav badge when true. */
  readonly?: boolean
  /** Lazy content builder. */
  build: () => ReactNode
}

/** Nav item produced by the registry (one per section). */
type SectionNavItem = {
  title: string
  url: string
  /** Read-only / external surface — shown as a badge. */
  readonly?: boolean
}

/**
 * Registry config supplied to {@link createSectionRegistry}.
 */
export type SectionRegistryConfig<
  TSectionId extends string,
  TSection extends SectionDefinition = SectionDefinition,
> = {
  sections: readonly TSection[]
  defaultSection: TSectionId
  /** Base path, e.g. '/settings/general'. */
  basePath: string
  /**
   * 'path' (default): section URLs are `${basePath}/${id}`.
   * 'query': section URLs are `${basePath}?section=${id}`.
   */
  urlStyle?: 'path' | 'query'
}

/**
 * Registry returned by {@link createSectionRegistry}.
 */
export type SectionRegistry<
  TSectionId extends string,
  TSection extends SectionDefinition = SectionDefinition,
> = {
  /** All section ids (typed as the feature's literal union). */
  sectionIds: readonly TSectionId[]
  /** Section navigated to when no section param is present. */
  defaultSection: TSectionId
  /** Nav items (one per section). */
  getSectionNavItems: () => SectionNavItem[]
  /** Render the content for a section (falls back to sections[0] on unknown id). */
  getSectionContent: (sectionId: TSectionId) => ReactNode
  /** Look up a section's metadata (falls back to sections[0] on unknown id). */
  getSectionMeta: (sectionId: TSectionId) => TSection
}

/**
 * Create a section registry with helper functions.
 *
 * Each feature/subarea calls this once to produce the nav + content surface
 * consumed by its page component. The generic `TSectionId` lets each caller
 * narrow its section ids to a literal union for compile-time safety; the
 * concrete section type is inferred from `sections`.
 */
export function createSectionRegistry<
  TSectionId extends string,
  TSection extends SectionDefinition = SectionDefinition,
>(
  config: SectionRegistryConfig<TSectionId, TSection>
): SectionRegistry<TSectionId, TSection> {
  const { sections, defaultSection, basePath, urlStyle = 'path' } = config

  const sectionIds = sections.map(
    (section) => section.id
  ) as unknown as readonly TSectionId[]

  function getSectionMeta(sectionId: TSectionId): TSection {
    return sections.find((section) => section.id === sectionId) ?? sections[0]
  }

  function getSectionNavItems(): SectionNavItem[] {
    return sections.map((section) => ({
      title: section.title,
      url:
        urlStyle === 'query'
          ? `${basePath}?section=${section.id}`
          : `${basePath}/${section.id}`,
      ...(section.readonly ? { readonly: true } : {}),
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
