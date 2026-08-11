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
    title: 'settings.downstream.keys.title',
    description: 'settings.downstream.keys.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.downstream.keys.title',
        description: 'settings.downstream.keys.description',
        legacyRef: 'legacy page: /downstream-keys (DownstreamKeys.tsx)',
      }),
  },
  {
    id: 'proxy-token',
    title: 'settings.downstream.proxyToken.title',
    description: 'settings.downstream.proxyToken.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.downstream.proxyToken.title',
        description: 'settings.downstream.proxyToken.description',
        legacyRef: 'legacy Settings.tsx: proxyToken (card 8)',
      }),
  },
] as const

type DownstreamSectionId = (typeof DOWNSTREAM_SECTIONS)[number]['id']

const registry = createSectionRegistry<DownstreamSectionId>({
  sections: DOWNSTREAM_SECTIONS,
  defaultSection: 'keys',
  basePath: '/settings/downstream',
})

export const downstreamSubarea: SettingsSubarea = {
  id: 'downstream',
  title: 'settings.subareas.downstream',
  basePath: '/settings/downstream',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as DownstreamSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as DownstreamSectionId),
}
