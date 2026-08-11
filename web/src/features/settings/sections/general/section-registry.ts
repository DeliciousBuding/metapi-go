// metapi-go/features/settings/sections/general — General subarea.
// Scope (plan §5.5.2): site/branding/authentication + core runtime config.
// Sections: site, authentication, scheduling, proxy-transport, routing.
// Phase 2 stubs; phase 3 migrates from the legacy Settings.tsx cards.
//
// This module is a .ts file (no JSX syntax) so the react/only-export-components
// fast-refresh rule does not apply — the registry exports config values, not
// components. Section content is built with React.createElement, which is
// hooks-safe and stays valid when phase 3 swaps StubSection for real forms.

import { createElement } from 'react'

import { StubSection } from '../../components/stub-section'
import { createSectionRegistry } from '../../utils/section-registry'
import type { SettingsSubarea } from '../../types'

const GENERAL_SECTIONS = [
  {
    id: 'site',
    title: 'settings.general.site.title',
    description: 'settings.general.site.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.general.site.title',
        description: 'settings.general.site.description',
        legacyRef:
          'legacy Settings.tsx: SystemName / Logo / Footer / About / HomePageContent / ServerAddress',
      }),
  },
  {
    id: 'authentication',
    title: 'settings.general.authentication.title',
    description: 'settings.general.authentication.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.general.authentication.title',
        description: 'settings.general.authentication.description',
        legacyRef:
          'legacy Settings.tsx: changeAuthToken + adminIpAllowlist (cards 1 + 15)',
      }),
  },
  {
    id: 'scheduling',
    title: 'settings.general.scheduling.title',
    description: 'settings.general.scheduling.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.general.scheduling.title',
        description: 'settings.general.scheduling.description',
        legacyRef:
          'legacy Settings.tsx: checkinScheduleMode / checkinCron / balanceRefreshCron / logCleanupCron (card 2)',
      }),
  },
  {
    id: 'proxy-transport',
    title: 'settings.general.proxyTransport.title',
    description: 'settings.general.proxyTransport.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.general.proxyTransport.title',
        description: 'settings.general.proxyTransport.description',
        legacyRef:
          'legacy Settings.tsx: systemProxyUrl + payloadRules + codexUpstream concurrency + modelAvailabilityProbe (cards 3-7)',
      }),
  },
  {
    id: 'routing',
    title: 'settings.general.routing.title',
    description: 'settings.general.routing.description',
    build: () =>
      createElement(StubSection, {
        title: 'settings.general.routing.title',
        description: 'settings.general.routing.description',
        legacyRef:
          'legacy Settings.tsx: routingFallbackUnitCost + routingWeights + routeFailureCooldown + proxyFirstByteTimeout (card 9)',
      }),
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
  basePath: '/settings/general',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as GeneralSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as GeneralSectionId),
}
