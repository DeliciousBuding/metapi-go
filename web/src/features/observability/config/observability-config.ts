// metapi-go/features/observability/config — 3-section manifest (Overview /
// Health / Proxy Logs). Registered through a local section-registry factory
// and consumed by ObservabilityPage + the route file + the sidebar drill-in.

import {
  createElement,
  lazy,
  Suspense,
  type ComponentType,
  type ReactNode,
} from 'react'

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
const LazyProxyLogsSection = lazy(() =>
  import('../sections/proxy-logs').then((module) => ({
    default: module.ProxyLogsSection,
  }))
)

function mountSection(component: ComponentType): () => ReactNode {
  return () =>
    createElement(Suspense, { fallback: null }, createElement(component))
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
  {
    id: 'proxy-logs',
    title: 'observability.sections.proxyLogs.title',
    description: 'observability.sections.proxyLogs.description',
    build: mountSection(LazyProxyLogsSection),
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
