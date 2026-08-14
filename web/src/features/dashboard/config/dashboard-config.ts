// metapi-go/features/dashboard/config — 4-section manifest.
//
// Registers the 4 dashboard sections (overview / traffic / models /
// availability) per plan.md §5.5.1 and exposes the registry helpers consumed
// by `DashboardPage` (the section dispatcher) + route guards. Mirrors the
// settings feature's per-subarea section-registry pattern (a .ts module so
// the react/only-export-components fast-refresh rule does not apply — section
// content is built with React.createElement, hooks-safe).
//
// Sections are loaded via React.lazy + dynamic import() so each section's
// heavy dependencies (VChart for traffic/models, recharts for overview) land
// in separate async chunks instead of the main sync bundle. The lazy
// component references are created at module level so their identity is
// stable across renders (React.lazy requires this to avoid remounting).

import {
  createElement,
  lazy,
  Suspense,
  type ComponentType,
  type ReactNode,
} from 'react'

import { SectionSkeleton } from '@/components/ui/section-skeleton'

import type { DashboardSection, DashboardSectionId } from '../types'
import { createSectionRegistry } from '../utils/section-registry'

const LazyOverviewSection = lazy(() =>
  import('../sections/overview').then((module) => ({
    default: module.OverviewSection,
  }))
)
const LazyTrafficSection = lazy(() =>
  import('../sections/traffic').then((module) => ({
    default: module.TrafficSection,
  }))
)
const LazyModelsSection = lazy(() =>
  import('../sections/models').then((module) => ({
    default: module.ModelsSection,
  }))
)
const LazyAvailabilitySection = lazy(() =>
  import('../sections/availability').then((module) => ({
    default: module.AvailabilitySection,
  }))
)

/** Wrap a lazy section in a Suspense boundary so build() returns a ready node. */
function mountSection(component: ComponentType): () => ReactNode {
  return () =>
    createElement(
      Suspense,
      { fallback: createElement(SectionSkeleton) },
      createElement(component)
    )
}

const DASHBOARD_SECTIONS: readonly DashboardSection[] = [
  {
    id: 'overview',
    title: 'dashboard.sections.overview.title',
    description: 'dashboard.sections.overview.description',
    build: mountSection(LazyOverviewSection),
  },
  {
    id: 'traffic',
    title: 'dashboard.sections.traffic.title',
    description: 'dashboard.sections.traffic.description',
    build: mountSection(LazyTrafficSection),
  },
  {
    id: 'models',
    title: 'dashboard.sections.models.title',
    description: 'dashboard.sections.models.description',
    build: mountSection(LazyModelsSection),
  },
  {
    id: 'availability',
    title: 'dashboard.sections.availability.title',
    description: 'dashboard.sections.availability.description',
    build: mountSection(LazyAvailabilitySection),
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
