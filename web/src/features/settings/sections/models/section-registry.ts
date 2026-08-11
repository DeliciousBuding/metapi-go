// metapi-go/features/settings/sections/models — Models subarea.
// Scope (plan §5.5.2): model-name redirects + rates/multipliers + global
// model allowlist & brand blocking. All three sections wired to real forms
// under ./components.

import { createElement } from 'react'

import { createSectionRegistry } from '../../utils/section-registry'
import type { SettingsSubarea } from '../../types'
import { AllowlistSection } from './components/allowlist-section'
import { RatesSection } from './components/rates-section'
import { RedirectsSection } from './components/redirects-section'

const MODELS_SECTIONS = [
  {
    id: 'redirects',
    title: 'settings.models.redirects.title',
    description: 'settings.models.redirects.description',
    build: () => createElement(RedirectsSection),
  },
  {
    id: 'rates',
    title: 'settings.models.rates.title',
    description: 'settings.models.rates.description',
    build: () => createElement(RatesSection),
  },
  {
    id: 'allowlist',
    title: 'settings.models.allowlist.title',
    description: 'settings.models.allowlist.description',
    build: () => createElement(AllowlistSection),
  },
] as const

type ModelsSectionId = (typeof MODELS_SECTIONS)[number]['id']

const registry = createSectionRegistry<ModelsSectionId>({
  sections: MODELS_SECTIONS,
  defaultSection: 'redirects',
  basePath: '/settings/models',
})

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
