// metapi-go/features/settings/sections/models — Models subarea.
// Scope (plan §5.5.2): model-name redirects + rates/multipliers + global
// model allowlist & brand blocking.
// Sections: redirects, rates, allowlist.
// Phase 2 stubs; phase 3 migrates from the legacy appended sections +
// Settings.tsx cards 10-11.
//
// .ts (no JSX) so react/only-export-components does not apply; section content
// is built with React.createElement (hooks-safe, phase-3 ready).

import { createElement } from 'react'

import { StubSection } from '../../components/stub-section'
import { createSectionRegistry } from '../../utils/section-registry'
import type { SettingsSubarea } from '../../types'

const MODELS_SECTIONS = [
  {
    id: 'redirects',
    title: 'settings.models.redirects.title',
    description: 'settings.models.redirects.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.models.redirects.title',
        description: 'settings.models.redirects.description',
        legacyRef:
          'legacy Settings.tsx: ModelRedirectsSection (K1a) — generate / preview / apply / promote / delete',
      }),
  },
  {
    id: 'rates',
    title: 'settings.models.rates.title',
    description: 'settings.models.rates.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.models.rates.title',
        description: 'settings.models.rates.description',
        legacyRef:
          'legacy Settings.tsx: RatesOverviewSection (N9a) — accounts unitCost + channels weight inline edit',
      }),
  },
  {
    id: 'allowlist',
    title: 'settings.models.allowlist.title',
    description: 'settings.models.allowlist.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.models.allowlist.title',
        description: 'settings.models.allowlist.description',
        legacyRef:
          'legacy Settings.tsx: globalAllowedModels + globalBlockedBrands (cards 10-11)',
      }),
  },
] as const

export type ModelsSectionId = (typeof MODELS_SECTIONS)[number]['id']

const registry = createSectionRegistry<ModelsSectionId>({
  sections: MODELS_SECTIONS,
  defaultSection: 'redirects',
  basePath: '/settings/models',
})

export const MODELS_SECTION_IDS = registry.sectionIds
export const MODELS_DEFAULT_SECTION = registry.defaultSection
export const getModelsSectionNavItems = registry.getSectionNavItems
export const getModelsSectionContent = registry.getSectionContent
export const getModelsSectionMeta = registry.getSectionMeta

export const modelsSubarea: SettingsSubarea = {
  id: 'models',
  title: 'settings.subareas.models',
  basePath: '/settings/models',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as ModelsSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as ModelsSectionId),
}
