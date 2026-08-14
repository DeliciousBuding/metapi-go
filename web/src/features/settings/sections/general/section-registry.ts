// metapi-go/features/settings/sections/general — General subarea.
// Scope (plan §5.5.2): site/branding/authentication + core runtime config.
// Sections: site, authentication, scheduling, proxy-transport, routing.
// Phase 3 wires the real RHF + Zod forms under ./components/* into the
// createSectionRegistry builders, replacing the phase-2 StubSection.
//
// This module is a .ts file (no JSX syntax) so the react/only-export-components
// fast-refresh rule does not apply — the registry exports config values, not
// components. Section content is built with React.createElement around
// React.lazy components, which is hooks-safe and keeps each section's heavy
// form/table dependencies in separate async chunks instead of the main bundle.
// The lazy component references are created at module level so their identity
// is stable across renders (React.lazy requires this to avoid remounting); the
// surrounding Suspense boundary lives in settings-page.tsx.

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
const LazySchedulingSection = lazy(() =>
  import('./components/scheduling-section').then((module) => ({
    default: module.SchedulingSection,
  }))
)
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

const GENERAL_SECTIONS = [
  {
    id: 'site',
    title: 'settings.general.site.title',
    group: 'settings.general.groups.brandSecurity',
    description: 'settings.general.site.description',
    build: () => createElement(LazySiteSection),
  },
  {
    id: 'authentication',
    title: 'settings.general.authentication.title',
    group: 'settings.general.groups.brandSecurity',
    description: 'settings.general.authentication.description',
    build: () => createElement(LazyAuthenticationSection),
  },
  {
    id: 'scheduling',
    title: 'settings.general.scheduling.title',
    group: 'settings.general.groups.runtimeStrategy',
    description: 'settings.general.scheduling.description',
    build: () => createElement(LazySchedulingSection),
  },
  {
    id: 'proxy-transport',
    title: 'settings.general.proxyTransport.title',
    group: 'settings.general.groups.runtimeStrategy',
    description: 'settings.general.proxyTransport.description',
    build: () => createElement(LazyProxyTransportSection),
  },
  {
    id: 'routing',
    title: 'settings.general.routing.title',
    group: 'settings.general.groups.runtimeStrategy',
    description: 'settings.general.routing.description',
    build: () => createElement(LazyRoutingSection),
  },
] as const

type GeneralSectionId = (typeof GENERAL_SECTIONS)[number]['id']

const registry = createSectionRegistry<GeneralSectionId>({
  sections: GENERAL_SECTIONS,
  defaultSection: 'site',
  basePath: '/settings/general',
})

/**
 * String-typed adapter consumed by SettingsPage + settings-config.
 * The `as GeneralSectionId` casts are safe (registry falls back to
 * sections[0] on unknown ids).
 */
export const generalSubarea: SettingsSubarea = {
  id: 'general',
  title: 'settings.subareas.general',
  description: 'settings.subareas.general.description',
  icon: Settings,
  basePath: '/settings/general',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as GeneralSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as GeneralSectionId),
}
