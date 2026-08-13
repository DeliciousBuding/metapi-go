// metapi-go/features/observability — createSectionRegistry generic factory.
//
// Mirrors the dashboard feature's section-registry: each section is a lazy
// builder returning a ReactNode; the registry derives nav items
// (`/observability?section=<id>`) and the active section's content. The
// generic `TSectionId` narrows the literal union for compile-time safety.

import type { ReactNode } from 'react'

import type {
  ObservabilitySection,
  ObservabilitySectionNavItem,
} from '../types'

export type ObservabilitySectionRegistryConfig<TSectionId extends string> = {
  sections: readonly ObservabilitySection[]
  defaultSection: TSectionId
  basePath: string
}

export type ObservabilitySectionRegistry<TSectionId extends string> = {
  sectionIds: readonly TSectionId[]
  defaultSection: TSectionId
  getSectionNavItems: () => ObservabilitySectionNavItem[]
  getSectionContent: (sectionId: TSectionId) => ReactNode
  getSectionMeta: (sectionId: TSectionId) => ObservabilitySection
}

export function createObservabilitySectionRegistry<TSectionId extends string>(
  config: ObservabilitySectionRegistryConfig<TSectionId>
): ObservabilitySectionRegistry<TSectionId> {
  const { sections, defaultSection, basePath } = config

  const sectionIds = sections.map(
    (section) => section.id
  ) as unknown as readonly TSectionId[]

  function getSectionMeta(sectionId: TSectionId): ObservabilitySection {
    return sections.find((section) => section.id === sectionId) ?? sections[0]
  }

  function getSectionNavItems(): ObservabilitySectionNavItem[] {
    return sections.map((section) => ({
      title: section.title,
      url: `${basePath}?section=${section.id}`,
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
