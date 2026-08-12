// metapi-go/features/dashboard/lib — VChart spec builders.
//
// Pure functions that turn stub data + resolved {@link ChartColors} into
// VChart specs. Ported from the legacy web/components/charts/* family
// (IncomeOutcome / SiteTrend / SiteDistribution / CostDistribution) per
// research/06-motion-icons-charts-responsive.md §5.3. Every spec sets concrete
// colors (never var()) and `background: 'transparent'` so the card chrome
// shows through the VChart canvas.
//
// Phase 2: builders are wired but fed empty arrays (the sections pass []).
// Phase 3: sections will pass real api.ts response rows (reshaped to the
// types declared in features/dashboard/types.ts) — the spec builders stay
// unchanged.

import type { ChartColors } from '../hooks/use-chart-colors'
import type {
  IncomeOutcomePoint,
  ModelCostRow,
  SiteDistributionSlice,
  SiteTrendPoint,
  VChartSpec,
} from '../types'

/** Respect the OS-level reduced-motion preference (matches styles/index.css). */
function prefersReducedMotion(): boolean {
  if (
    typeof window === 'undefined' ||
    typeof window.matchMedia !== 'function'
  ) {
    return false
  }
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

/** Shared VChart runtime option — desktop-browser mode for all charts. */
export const VCHART_OPTION = { mode: 'desktop-browser' as const }

/** Common axis block reused across the line / bar specs. */
function buildAxes(colors: ChartColors) {
  return [
    {
      orient: 'bottom',
      label: { style: { fontSize: 11, fill: colors.axisLabel } },
      domainLine: { style: { stroke: colors.grid } },
      tick: { style: { stroke: colors.grid } },
    },
    {
      orient: 'left',
      label: { style: { fontSize: 11, fill: colors.axisLabel } },
      grid: { style: { stroke: colors.grid, lineDash: [4, 4] } },
      domainLine: { visible: false },
    },
  ]
}

const ANIMATION_APPEAR = {
  line: { type: 'clipIn', duration: 800, easing: 'cubicOut' },
  area: { type: 'clipIn', duration: 800, easing: 'cubicOut' },
  bar: { type: 'growUp', duration: 600, easing: 'cubicOut' },
  pie: { type: 'growRadius', duration: 600, easing: 'cubicOut' },
} as const

const PADDING = { left: 8, right: 16, top: 8, bottom: 8 } as const

/** Adaptive currency formatting for tooltip values (mirrors the legacy charts). */
function formatCurrency(value: number): string {
  if (value >= 1000) return `$${value.toFixed(2)}`
  if (value >= 1) return `$${value.toFixed(3)}`
  return `$${value.toFixed(6)}`
}

/** Coerce a VChart tooltip datum to a record (mirrors the legacy charts). */
function coerceDatum(datum: unknown): Record<string, unknown> {
  return datum && typeof datum === 'object'
    ? (datum as Record<string, unknown>)
    : {}
}

/** Render the VChart-computed slice share as a percent string. */
function percentOf(datum: unknown): string {
  const pct = Number(coerceDatum(datum)._percent_ ?? 0)
  return `${pct.toFixed(1)}%`
}

/** Site-distribution donut tooltip labels (i18n supplied by the section). */
export type SiteDistributionTooltipLabels = {
  balance: string
  accounts: string
  share: string
}

/** Model-cost donut tooltip labels (i18n supplied by the section). */
export type ModelCostTooltipLabels = {
  cost: string
  calls: string
  tokens: string
  share: string
}

/**
 * Income vs outcome grouped bar chart (long-format rows: day/type/value).
 * Ported from legacy IncomeOutcomeChart. Series colors come from the chart
 * palette; bars are grouped (not stacked) with a maxWidth cap.
 */
export function buildIncomeOutcomeSpec(
  colors: ChartColors,
  data: IncomeOutcomePoint[]
): VChartSpec {
  return {
    type: 'bar',
    data: [{ id: 'income-outcome', values: data }],
    xField: 'day',
    yField: 'value',
    seriesField: 'type',
    stack: false,
    bar: { style: { maxWidth: 14 } },
    axes: buildAxes(colors),
    color: colors.series,
    legends: {
      visible: true,
      position: 'top',
      item: {
        label: { style: { fill: colors.axisLabel } },
      },
    },
    animation: !prefersReducedMotion(),
    animationAppear: ANIMATION_APPEAR.bar,
    background: 'transparent',
    padding: PADDING,
  }
}

/**
 * Per-site spend trend line chart (long-format rows: date/site/spend/calls).
 * Ported from legacy SiteTrendChart. One series per site; legends at bottom.
 */
export function buildSiteTrendSpec(
  colors: ChartColors,
  data: SiteTrendPoint[]
): VChartSpec {
  return {
    type: 'line',
    data: [{ id: 'site-trend', values: data }],
    xField: 'date',
    yField: 'spend',
    seriesField: 'site',
    axes: buildAxes(colors),
    color: colors.series,
    legends: {
      visible: true,
      position: 'bottom',
      item: {
        shape: { style: { symbolType: 'circle' } },
        label: { style: { fill: colors.axisLabel } },
      },
    },
    line: { style: { curveType: 'monotone' } },
    point: { style: { fill: colors.onPrimary, stroke: colors.series } },
    animation: !prefersReducedMotion(),
    animationAppear: ANIMATION_APPEAR.line,
    background: 'transparent',
    padding: PADDING,
  }
}

/**
 * Site balance distribution donut (slice per site). Ported from legacy
 * SiteDistributionChart. `labelColor` is the outside-label fill (resolved via
 * useThemeLabelColor in the legacy donut charts).
 */
export function buildSiteDistributionSpec(
  colors: ChartColors,
  labelColor: string,
  data: SiteDistributionSlice[],
  tooltipLabels: SiteDistributionTooltipLabels
): VChartSpec {
  const values = data.map((slice) => ({
    siteName: slice.siteName,
    value: slice.totalBalance,
    accountCount: slice.accountCount,
    totalSpend: slice.totalSpend,
  }))
  return {
    type: 'pie',
    data: [{ id: 'site-distribution', values }],
    valueField: 'value',
    categoryField: 'siteName',
    outerRadius: 0.8,
    innerRadius: 0.55,
    padAngle: 0.02,
    cornerRadius: 4,
    color: colors.series,
    label: {
      visible: true,
      position: 'outside',
      text: '{_percent_}%',
      style: { fontSize: 11, fill: labelColor },
    },
    pie: {
      style: {
        cornerRadius: 4,
        stroke: colors.grid,
        lineWidth: 1,
      },
    },
    tooltip: {
      mark: {
        title: {
          value: (datum: unknown) => String(coerceDatum(datum).siteName ?? '-'),
        },
        content: [
          {
            key: tooltipLabels.balance,
            value: (datum: unknown) =>
              formatCurrency(Number(coerceDatum(datum).value ?? 0)),
          },
          {
            key: tooltipLabels.accounts,
            value: (datum: unknown) =>
              String(coerceDatum(datum).accountCount ?? 0),
          },
          {
            key: tooltipLabels.share,
            value: (datum: unknown) => percentOf(datum),
          },
        ],
      },
    },
    legends: {
      visible: true,
      position: 'bottom',
      item: {
        label: { style: { fill: colors.axisLabel } },
      },
    },
    animation: !prefersReducedMotion(),
    animationAppear: ANIMATION_APPEAR.pie,
    background: 'transparent',
    padding: PADDING,
  }
}

/**
 * Latency histogram bar chart (one bar per latency bucket). Fed by
 * api.getLatencyHistogram(): each bucket is `{ label, count }`. Single-series
 * bar chart in the first series color (no seriesField).
 */
export function buildLatencyHistogramSpec(
  colors: ChartColors,
  data: Array<{ label: string; count: number }>
): VChartSpec {
  return {
    type: 'bar',
    data: [{ id: 'latency-histogram', values: data }],
    xField: 'label',
    yField: 'count',
    bar: { style: { maxWidth: 24, fill: colors.series[0] } },
    axes: buildAxes(colors),
    animation: !prefersReducedMotion(),
    animationAppear: ANIMATION_APPEAR.bar,
    background: 'transparent',
    padding: PADDING,
  }
}

/**
 * Latency trend dual-line chart (avg + p95 over time). Fed by
 * api.getLatencyTrend(): flattened to `{ date, metric, latency }` rows.
 */
export function buildLatencyTrendSpec(
  colors: ChartColors,
  data: Array<{ date: string; metric: string; latency: number }>
): VChartSpec {
  return {
    type: 'line',
    data: [{ id: 'latency-trend', values: data }],
    xField: 'date',
    yField: 'latency',
    seriesField: 'metric',
    axes: buildAxes(colors),
    color: colors.series,
    legends: {
      visible: true,
      position: 'bottom',
      item: {
        shape: { style: { symbolType: 'circle' } },
        label: { style: { fill: colors.axisLabel } },
      },
    },
    line: { style: { curveType: 'monotone' } },
    point: { style: { fill: colors.onPrimary, stroke: colors.series } },
    animation: !prefersReducedMotion(),
    animationAppear: ANIMATION_APPEAR.line,
    background: 'transparent',
    padding: PADDING,
  }
}

/**
 * Model cost distribution donut (slice per model). Ported from legacy
 * CostDistributionChart. Same donut shape as site distribution but keyed on
 * model name and valued by cost; tooltip carries calls / tokens / share.
 */
export function buildModelCostSpec(
  colors: ChartColors,
  labelColor: string,
  data: ModelCostRow[],
  tooltipLabels: ModelCostTooltipLabels
): VChartSpec {
  const values = data.map((row) => ({
    model: row.label || row.model,
    value: row.cost,
    calls: row.calls,
    tokens: row.tokens,
  }))
  return {
    type: 'pie',
    data: [{ id: 'model-cost', values }],
    valueField: 'value',
    categoryField: 'model',
    outerRadius: 0.85,
    innerRadius: 0.62,
    padAngle: 0.6,
    cornerRadius: 3,
    color: colors.series,
    label: {
      visible: true,
      position: 'outside',
      text: '{_percent_}%',
      style: { fontSize: 11, fill: labelColor },
    },
    pie: {
      style: {
        cornerRadius: 3,
        stroke: colors.grid,
        lineWidth: 1,
      },
    },
    tooltip: {
      mark: {
        title: {
          value: (datum: unknown) => String(coerceDatum(datum).model ?? '-'),
        },
        content: [
          {
            key: tooltipLabels.cost,
            value: (datum: unknown) =>
              formatCurrency(Number(coerceDatum(datum).value ?? 0)),
          },
          {
            key: tooltipLabels.calls,
            value: (datum: unknown) => String(coerceDatum(datum).calls ?? 0),
          },
          {
            key: tooltipLabels.tokens,
            value: (datum: unknown) =>
              Number(coerceDatum(datum).tokens ?? 0).toLocaleString(),
          },
          {
            key: tooltipLabels.share,
            value: (datum: unknown) => percentOf(datum),
          },
        ],
      },
    },
    legends: {
      visible: true,
      position: 'bottom',
      item: {
        label: { style: { fill: colors.axisLabel } },
      },
    },
    animation: !prefersReducedMotion(),
    animationAppear: ANIMATION_APPEAR.pie,
    background: 'transparent',
    padding: PADDING,
  }
}
