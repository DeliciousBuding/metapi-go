// metapi-go/features/settings/sections/basic — Basic subarea (wave 9 lane B):
// site/branding + administrator authentication. These two sections were
// extracted from the retired `general` subarea, whose remaining sections now
// live in proxy-models (proxy-transport, routing) and operations (scheduling).
// Group labels are gone: the sidebar's collapsible tree shows one flat
// section list per subarea.
//
// Each section is React.lazy so its form dependencies land in a separate
// async chunk; the surrounding Suspense boundary lives in settings-page.tsx.

import { Settings } from 'lucide-react'
import { createElement, lazy } from 'react'

import type { SettingsSubarea } from '../../types'
import { createSectionRegistry } from '../../utils/section-registry'

const LazySiteSection = lazy(() =>
  import('./components/site-section').then((module) => ({
    default: module.SiteSection,
  }))
)
const LazyAuthenticationSection = lazy(() =>
  import('./components/authentication-section').then((module) => ({
    default: module.AuthenticationSection,
  }))
)

const BASIC_SECTIONS = [
  {
    id: 'site',
    title: 'settings.basic.site.title',
    description: 'settings.basic.site.description',
    build: () => createElement(LazySiteSection),
  },
  {
    id: 'authentication',
    title: 'settings.basic.authentication.title',
    description: 'settings.basic.authentication.description',
    build: () => createElement(LazyAuthenticationSection),
  },
] as const

type BasicSectionId = (typeof BASIC_SECTIONS)[number]['id']

const registry = createSectionRegistry<BasicSectionId>({
  sections: BASIC_SECTIONS,
  defaultSection: 'site',
  basePath: '/settings/basic',
})

export const basicSubarea: SettingsSubarea = {
  id: 'basic',
  title: 'settings.subareas.basic',
  description: 'settings.subareas.basic.description',
  icon: Settings,
  basePath: '/settings/basic',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as BasicSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as BasicSectionId),
}
