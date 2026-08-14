// metapi-go/features/settings/sections/downstream — Downstream Keys subarea.
// Scope (plan §5.5.2): downstream API keys + the global PROXY_TOKEN.
// Sections: keys, proxy-token. Both wired to real forms under ./components.
// Each section is React.lazy so its form/table dependencies land in a separate
// async chunk; the surrounding Suspense boundary lives in settings-page.tsx.

import { KeyRound } from 'lucide-react'
import { createElement, lazy } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'

const LazyKeysSection = lazy(() =>
  import('./components/keys-section').then((module) => ({
    default: module.KeysSection,
  }))
)
const LazyProxyTokenSection = lazy(() =>
  import('./components/proxy-token-section').then((module) => ({
    default: module.ProxyTokenSection,
  }))
)

const DOWNSTREAM_SECTIONS = [
  {
    id: 'keys',
    title: 'settings.downstream.keys.title',
    description: 'settings.downstream.keys.description',
    build: () => createElement(LazyKeysSection),
  },
  {
    id: 'proxy-token',
    title: 'settings.downstream.proxyToken.title',
    description: 'settings.downstream.proxyToken.description',
    build: () => createElement(LazyProxyTokenSection),
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
  description: 'settings.subareas.downstream.description',
  icon: KeyRound,
  basePath: '/settings/downstream',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as DownstreamSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as DownstreamSectionId),
}
