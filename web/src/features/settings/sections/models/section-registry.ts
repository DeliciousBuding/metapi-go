// metapi-go/features/settings/sections/models — Models subarea.
// Scope (plan §5.5.2): model-name redirects + rates/multipliers + global
// model allowlist & brand blocking. All three sections wired to real forms
// under ./components.
// Each section is React.lazy so its form/table dependencies land in a separate
// async chunk; the surrounding Suspense boundary lives in settings-page.tsx.

import { Boxes } from 'lucide-react'
import { createElement, lazy } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'

const LazyRedirectsSection = lazy(() =>
  import('./components/redirects-section').then((module) => ({
    default: module.RedirectsSection,
  }))
)
const LazyRatesSection = lazy(() =>
  import('./components/rates-section').then((module) => ({
    default: module.RatesSection,
  }))
)
const LazyAllowlistSection = lazy(() =>
  import('./components/allowlist-section').then((module) => ({
    default: module.AllowlistSection,
  }))
)
const LazyCatalogSourcesSection = lazy(() =>
  import('./components/catalog-sources-section').then((module) => ({
    default: module.CatalogSourcesSection,
  }))
)

const MODELS_SECTIONS = [
  {
    id: 'redirects',
    title: 'settings.models.redirects.title',
    description: 'settings.models.redirects.description',
    build: () => createElement(LazyRedirectsSection),
  },
  {
    id: 'rates',
    title: 'settings.models.rates.title',
    description: 'settings.models.rates.description',
    build: () => createElement(LazyRatesSection),
  },
  {
    id: 'allowlist',
    title: 'settings.models.allowlist.title',
    description: 'settings.models.allowlist.description',
    build: () => createElement(LazyAllowlistSection),
  },
  {
    id: 'catalog-sources',
    title: 'settings.models.catalogSources.title',
    description: 'settings.models.catalogSources.description',
    build: () => createElement(LazyCatalogSourcesSection),
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
  description: 'settings.subareas.models.description',
  icon: Boxes,
  basePath: '/settings/models',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as ModelsSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as ModelsSectionId),
}
