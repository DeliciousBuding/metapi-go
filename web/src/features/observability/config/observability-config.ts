// metapi-go/features/observability/config — 2-section manifest (Overview /
// Health). Registered through a local section-registry factory and consumed
// by ObservabilityPage + the route file + the sidebar drill-in. Proxy logs
// are intentionally not a section: the drill-in sidebar links straight to
// the dedicated `/proxy-logs` workspace (see observability-nav.ts).

import {
  createElement,
  lazy,
  Suspense,
  type ComponentType,
  type ReactNode,
} from 'react'

import { SectionSkeleton } from '@/components/ui/section-skeleton'

import type { ObservabilitySection, ObservabilitySectionId } from '../types'
import { createObservabilitySectionRegistry } from '../utils/section-registry'

const LazyOverviewSection = lazy(() =>
  import('../sections/overview').then((module) => ({
    default: module.OverviewSection,
  }))
)
const LazyHealthSection = lazy(() =>
  import('../sections/health').then((module) => ({
    default: module.HealthSection,
  }))
)

function mountSection(component: ComponentType): () => ReactNode {
  return () =>
    createElement(
      Suspense,
      { fallback: createElement(SectionSkeleton) },
      createElement(component)
    )
}

const OBSERVABILITY_SECTIONS: readonly ObservabilitySection[] = [
  {
    id: 'overview',
    title: 'observability.sections.overview.title',
    description: 'observability.sections.overview.description',
    build: mountSection(LazyOverviewSection),
  },
  {
    id: 'health',
    title: 'observability.sections.health.title',
    description: 'observability.sections.health.description',
    build: mountSection(LazyHealthSection),
  },
]

const observabilityRegistry =
  createObservabilitySectionRegistry<ObservabilitySectionId>({
    sections: OBSERVABILITY_SECTIONS,
    defaultSection: 'overview',
    basePath: '/observability',
  })

export const OBSERVABILITY_DEFAULT_SECTION =
  observabilityRegistry.defaultSection
export const getObservabilitySectionNavItems =
  observabilityRegistry.getSectionNavItems
export const getObservabilitySectionContent =
  observabilityRegistry.getSectionContent
