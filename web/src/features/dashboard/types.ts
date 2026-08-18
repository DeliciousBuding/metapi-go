// metapi-go/features/dashboard — shared types for the 4-section Dashboard
// workspace (plan.md §5.5.1). The dashboard is split into overview / traffic /
// models / availability; each section owns a lazy builder (`build`) returning
// its content ReactNode. Phase 2 ships chart wiring + stub data; phase 3
// swaps in real API data from lib/api.ts (getDashboardSnapshot /
// getBalanceIncomeOutcome / getSiteTrend / getActiveAnnouncements etc.).

import type { ReactNode } from 'react'

/**
 * The 4 dashboard sections, in main-sidebar / tab order.
 */
export type DashboardSectionId =
  | 'overview'
  | 'traffic'
  | 'models'
  | 'availability'

/**
 * A single dashboard section — the leaf unit of the Dashboard workspace.
 *
 * `build` is a lazy content builder. Sections render chart wiring over the
 * shared recharts-based Chart components (ChartContainer + CSS-var() palette)
 * fed by TanStack Query hooks over the stats/dashboard API surface.
 */
export type DashboardSection = {
  /** Stable id used in the URL (`/dashboard/<id>`). */
  id: string
  /** Human label shown in the section tabs + page header. */
  title: string
  /** Short description shown under the page header (optional). */
  description?: string
  /** Lazy content builder. */
  build: () => ReactNode
}

/**
 * Nav item produced by the section registry for the dashboard tabs.
 */
export type DashboardSectionNavItem = {
  title: string
  url: string
}

// ---------------------------------------------------------------------------
// Chart data shapes — contracts for the dashboard chart components. Aligned
// with the response types of the corresponding api.ts methods
// (getBalanceIncomeOutcome, getSiteTrend, getSiteDistribution,
// getModelCostDistribution). Kept minimal so the recharts chart components
// can be wired now and fed real data without reshaping.
// ---------------------------------------------------------------------------

/** Long-format row for the income vs outcome grouped bar chart. */
export type IncomeOutcomePoint = {
  day: string
  /** 'income' | 'outcome' — the series discriminator. */
  type: string
  value: number
}

/** Long-format row for the per-site trend line chart. */
export type SiteTrendPoint = {
  date: string
  site: string
  spend: number
  calls: number
}

/** Slice for the site distribution donut. */
export type SiteDistributionSlice = {
  siteName: string
  platform: string
  totalBalance: number
  totalSpend: number
  accountCount: number
}

/** Model cost row for the models-section distribution chart. */
export type ModelCostRow = {
  model: string
  label: string
  cost: number
  calls: number
  tokens: number
}

/** Realtime ops WebSocket frame — one point per second. */
export type RealtimeOpsFrame = {
  lifetime: number
  points: Array<{ ts: number; total: number; success: number }>
}

/**
 * One sparkline sample — a single second's derived traffic + the success
 * fraction observed at that second. The success fraction drives the bar's
 * health colour (healthy / degraded / unhealthy), so a slow degradation is
 * visible at a glance even while volume stays normal. The realtime ops
 * stream itself carries no latency (see handler/shared/realtime.go), so
 * volume + success are the only signals on the wire.
 */
export type RealtimeOpsSamplePoint = {
  qps: number
  successRate: number
}

/** Live traffic sparkline sample (rolling 60s window). */
export type RealtimeOpsSample = {
  qps: number
  successRate: number
  lifetime: number
  spark: RealtimeOpsSamplePoint[]
  connected: boolean
  gaveUp: boolean
}
