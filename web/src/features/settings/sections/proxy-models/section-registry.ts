// metapi-go/features/settings/sections/proxy-models — Proxy & Models subarea
// (wave 9 lane B): transport/routing behaviour + model semantics
// (redirects, rates, allowlist) + the model-catalog source registry. Absorbs
// the retired `general` (proxy-transport, routing) and `models` subareas.
// Each section is React.lazy so its form/table dependencies land in a
// separate async chunk; the surrounding Suspense boundary lives in
// settings-page.tsx.

import { Boxes } from 'lucide-react'
import { createElement, lazy } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'

const LazyProxyTransportSection = lazy(() =>
  import('./components/proxy-transport-section').then((module) => ({
    default: module.ProxyTransportSection,
  }))
)
const LazyRoutingSection = lazy(() =>
  import('./components/routing-section').then((module) => ({
    default: module.RoutingSection,
  }))
)
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

const PROXY_MODELS_SECTIONS = [
  {
    id: 'proxy-transport',
    title: 'settings.proxyModels.proxyTransport.title',
    description: 'settings.proxyModels.proxyTransport.description',
    build: () => createElement(LazyProxyTransportSection),
  },
  {
    id: 'routing',
    title: 'settings.proxyModels.routing.title',
    description: 'settings.proxyModels.routing.description',
    build: () => createElement(LazyRoutingSection),
  },
  {
    id: 'redirects',
    title: 'settings.proxyModels.redirects.title',
    description: 'settings.proxyModels.redirects.description',
    build: () => createElement(LazyRedirectsSection),
  },
  {
    id: 'rates',
    title: 'settings.proxyModels.rates.title',
    description: 'settings.proxyModels.rates.description',
    build: () => createElement(LazyRatesSection),
  },
  {
    id: 'allowlist',
    title: 'settings.proxyModels.allowlist.title',
    description: 'settings.proxyModels.allowlist.description',
    build: () => createElement(LazyAllowlistSection),
  },
  {
    id: 'catalog-sources',
    title: 'settings.proxyModels.catalogSources.title',
    description: 'settings.proxyModels.catalogSources.description',
    build: () => createElement(LazyCatalogSourcesSection),
  },
] as const

type ProxyModelsSectionId = (typeof PROXY_MODELS_SECTIONS)[number]['id']

const registry = createSectionRegistry<ProxyModelsSectionId>({
  sections: PROXY_MODELS_SECTIONS,
  defaultSection: 'proxy-transport',
  basePath: '/settings/proxy-models',
})

export const proxyModelsSubarea: SettingsSubarea = {
  id: 'proxy-models',
  title: 'settings.subareas.proxyModels',
  description: 'settings.subareas.proxyModels.description',
  icon: Boxes,
  basePath: '/settings/proxy-models',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as ProxyModelsSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as ProxyModelsSectionId),
}
