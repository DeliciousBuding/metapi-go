// metapi-go/features/settings/sections/downstream — Downstream Keys subarea.
// Scope (plan §5.5.2): downstream API keys + the global PROXY_TOKEN.
// Sections: keys, proxy-token.
// Phase 2 stubs; phase 3 migrates from the legacy /downstream-keys page +
// Settings.tsx card 8.
//
// .ts (no JSX) so react/only-export-components does not apply; section content
// is built with React.createElement (hooks-safe, phase-3 ready).

import { createElement } from 'react'

import { StubSection } from '../../components/stub-section'
import { createSectionRegistry } from '../../utils/section-registry'
import type { SettingsSubarea } from '../../types'

const DOWNSTREAM_SECTIONS = [
  {
    id: 'keys',
    title: 'Downstream Keys',
    description: 'Downstream API keys (per-site/per-account issue tokens).',
    build: () =>
      createElement(StubSection, {
        title: 'Downstream Keys',
        description: 'Downstream API keys (per-site/per-account issue tokens).',
        legacyRef: 'legacy page: /downstream-keys (DownstreamKeys.tsx)',
      }),
  },
  {
    id: 'proxy-token',
    title: 'Proxy Token',
    description: 'Global downstream PROXY_TOKEN (sk- prefix, random suffix).',
    build: () =>
      createElement(StubSection, {
        title: 'Proxy Token',
        description: 'Global downstream PROXY_TOKEN (sk- prefix, random suffix).',
        legacyRef: 'legacy Settings.tsx: proxyToken (card 8)',
      }),
  },
] as const

export type DownstreamSectionId = (typeof DOWNSTREAM_SECTIONS)[number]['id']

const registry = createSectionRegistry<DownstreamSectionId>({
  sections: DOWNSTREAM_SECTIONS,
  defaultSection: 'keys',
  basePath: '/settings/downstream',
})

export const DOWNSTREAM_SECTION_IDS = registry.sectionIds
export const DOWNSTREAM_DEFAULT_SECTION = registry.defaultSection
export const getDownstreamSectionNavItems = registry.getSectionNavItems
export const getDownstreamSectionContent = registry.getSectionContent
export const getDownstreamSectionMeta = registry.getSectionMeta

export const downstreamSubarea: SettingsSubarea = {
  id: 'downstream',
  title: 'Downstream Keys',
  basePath: '/settings/downstream',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as DownstreamSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as DownstreamSectionId),
}
