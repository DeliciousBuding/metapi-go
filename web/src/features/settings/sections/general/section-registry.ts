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
    title: 'Site & Branding',
    description: 'System name, logo, footer, about, and server address.',
    build: () =>
      createElement(StubSection, {
        title: 'Site & Branding',
        description: 'System name, logo, footer, about, and server address.',
        legacyRef:
          'legacy Settings.tsx: SystemName / Logo / Footer / About / HomePageContent / ServerAddress',
      }),
  },
  {
    id: 'authentication',
    title: 'Authentication',
    description: 'Admin token rotation and IP allowlist.',
    build: () =>
      createElement(StubSection, {
        title: 'Authentication',
        description: 'Admin token rotation and IP allowlist.',
        legacyRef:
          'legacy Settings.tsx: changeAuthToken + adminIpAllowlist (cards 1 + 15)',
      }),
  },
  {
    id: 'scheduling',
    title: 'Scheduled Tasks',
    description: 'Checkin, balance refresh, and log cleanup schedules.',
    build: () =>
      createElement(StubSection, {
        title: 'Scheduled Tasks',
        description: 'Checkin, balance refresh, and log cleanup schedules.',
        legacyRef:
          'legacy Settings.tsx: checkinScheduleMode / checkinCron / balanceRefreshCron / logCleanupCron (card 2)',
      }),
  },
  {
    id: 'proxy-transport',
    title: 'Proxy & Transport',
    description:
      'System proxy, payload rules, upstream concurrency, and probe.',
    build: () =>
      createElement(StubSection, {
        title: 'Proxy & Transport',
        description:
          'System proxy, payload rules, upstream concurrency, and probe.',
        legacyRef:
          'legacy Settings.tsx: systemProxyUrl + payloadRules + codexUpstream concurrency + modelAvailabilityProbe (cards 3-7)',
      }),
  },
  {
    id: 'routing',
    title: 'Routing Strategy',
    description: 'Fallback cost, weights, cooldown, and timeouts.',
    build: () =>
      createElement(StubSection, {
        title: 'Routing Strategy',
        description: 'Fallback cost, weights, cooldown, and timeouts.',
        legacyRef:
          'legacy Settings.tsx: routingFallbackUnitCost + routingWeights + routeFailureCooldown + proxyFirstByteTimeout (card 9)',
      }),
  },
] as const

export type GeneralSectionId = (typeof GENERAL_SECTIONS)[number]['id']

const registry = createSectionRegistry<GeneralSectionId>({
  sections: GENERAL_SECTIONS,
  defaultSection: 'site',
  basePath: '/settings/general',
})

export const GENERAL_SECTION_IDS = registry.sectionIds
export const GENERAL_DEFAULT_SECTION = registry.defaultSection
export const getGeneralSectionNavItems = registry.getSectionNavItems
export const getGeneralSectionContent = registry.getSectionContent
export const getGeneralSectionMeta = registry.getSectionMeta

/**
 * String-typed adapter consumed by SettingsPage + settings-config.
 * The `as GeneralSectionId` casts are safe (registry falls back to
 * sections[0] on unknown ids).
 */
export const generalSubarea: SettingsSubarea = {
  id: 'general',
  title: 'General',
  basePath: '/settings/general',
  defaultSection: registry.defaultSection,
  sectionIds: registry.sectionIds,
  getSectionNavItems: registry.getSectionNavItems,
  getSectionContent: (sectionId) =>
    registry.getSectionContent(sectionId as GeneralSectionId),
  getSectionMeta: (sectionId) =>
    registry.getSectionMeta(sectionId as GeneralSectionId),
}
