// metapi-go/features/dashboard/config — 4-section manifest.
//
// Registers the 4 dashboard sections (overview / traffic / models /
// availability) per plan.md §5.5.1 and exposes the registry helpers consumed
// by `DashboardPage` (the section dispatcher) + route guards. Mirrors the
// settings feature's per-subarea section-registry pattern (a .ts module so
// the react/only-export-components fast-refresh rule does not apply — section
// content is built with React.createElement, hooks-safe).

import { createElement } from 'react'

import { OverviewSection } from '../sections/overview'
import { TrafficSection } from '../sections/traffic'
import { ModelsSection } from '../sections/models'
import { AvailabilitySection } from '../sections/availability'
import type { DashboardSection, DashboardSectionId } from '../types'
import { createSectionRegistry } from '../utils/section-registry'

const DASHBOARD_SECTIONS: readonly DashboardSection[] = [
  {
    id: 'overview',
    title: 'dashboard.sections.overview.title',
    description: 'dashboard.sections.overview.description',
    build: () => createElement(OverviewSection),
  },
  {
    id: 'traffic',
    title: 'dashboard.sections.traffic.title',
    description: 'dashboard.sections.traffic.description',
    build: () => createElement(TrafficSection),
  },
  {
    id: 'models',
    title: 'dashboard.sections.models.title',
    description: 'dashboard.sections.models.description',
    build: () => createElement(ModelsSection),
  },
  {
    id: 'availability',
    title: 'dashboard.sections.availability.title',
    description: 'dashboard.sections.availability.description',
    build: () => createElement(AvailabilitySection),
  },
]

const dashboardRegistry = createSectionRegistry<DashboardSectionId>({
  sections: DASHBOARD_SECTIONS,
  defaultSection: 'overview',
  basePath: '/dashboard',
})

/** Stable id list (used by route validation + the section tabs). */
export const DASHBOARD_SECTION_IDS = dashboardRegistry.sectionIds

/** Section navigated to when no `$section` param is present. */
export const DASHBOARD_DEFAULT_SECTION = dashboardRegistry.defaultSection

/** Tab nav items (one per section). */
export const getDashboardSectionNavItems = dashboardRegistry.getSectionNavItems

/** Render the content for a section (falls back to overview on unknown id). */
export const getDashboardSectionContent = dashboardRegistry.getSectionContent

/** Look up a section's metadata (falls back to overview on unknown id). */
export const getDashboardSectionMeta = dashboardRegistry.getSectionMeta
