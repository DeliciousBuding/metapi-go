// metapi-go/features/settings/sections/downstream — Downstream Keys subarea.
// Scope (plan §5.5.2): downstream API keys + the global PROXY_TOKEN.
// Sections: keys, proxy-token. Both wired to real forms under ./components.

import { KeyRound } from 'lucide-react'
import { createElement } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'
import { KeysSection } from './components/keys-section'
import { ProxyTokenSection } from './components/proxy-token-section'

const DOWNSTREAM_SECTIONS = [
  {
    id: 'keys',
    title: 'settings.downstream.keys.title',
    description: 'settings.downstream.keys.description',
    build: () => createElement(KeysSection),
  },
  {
    id: 'proxy-token',
    title: 'settings.downstream.proxyToken.title',
    description: 'settings.downstream.proxyToken.description',
    build: () => createElement(ProxyTokenSection),
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
