// metapi-go/features/settings/sections/downstream — Downstream subarea
// (wave 9 lane B): the global PROXY_TOKEN. The downstream API keys section
// was promoted to a first-class left-nav route (/downstream-keys) in wave 10,
// so this subarea now hosts only the proxy-token surface.
// Each section is React.lazy so its form/table dependencies land in a separate
// async chunk; the surrounding Suspense boundary lives in settings-page.tsx.

import { KeyRound } from 'lucide-react'
import { createElement, lazy } from 'react'

import { createSectionRegistry } from '@/lib/section-registry'

import type { SettingsSubarea } from '../../types'

const LazyProxyTokenSection = lazy(() =>
  import('./components/proxy-token-section').then((module) => ({
    default: module.ProxyTokenSection,
  }))
)

const DOWNSTREAM_SECTIONS = [
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
  defaultSection: 'proxy-token',
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
