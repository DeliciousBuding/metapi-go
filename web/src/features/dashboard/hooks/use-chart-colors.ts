// metapi-go/features/dashboard/hooks — JS color extraction for VChart canvas.
//
// Ported from the legacy web/components/useThemeLabelColor.ts (2026-08-01
// chart contrast pass). VChart renders to <canvas>, which cannot resolve CSS
// var() references — `fill: 'var(--muted-foreground)'` is not a valid canvas
// color and falls back to VChart's dark default, making axis/legend text
// invisible on dark themes (research/06-motion-icons-charts-responsive.md §5.3).
//
// This hook resolves the OKLCH theme tokens to concrete color strings via
// getComputedStyle, and re-reads whenever the theme flips. The re-read is
// driven by theme-provider's useTheme().resolvedTheme (the task requirement)
// plus a MutationObserver on <html> attributes that theme-preset / radius /
// scale switches mutate without flipping resolvedTheme (e.g. the Anthropic
// preset rewrites --chart-* via [data-theme-preset='anthropic']).
//
// Token mapping (legacy → new OKLCH theme):
//   --color-text-secondary  → --muted-foreground   (axis / legend label fill)
//   --color-border-light    → --border             (grid / tick / domainLine)
//   --color-on-primary      → --primary-foreground  (ink on primary fills)
//   --color-chart-1..8      → --chart-1..5          (5 ordinal series, not 8)
//   --color-chart-N-soft/-faint — no token in the new theme; derived in JS
//
// Contrast (WCAG, on card bg):
//   light: muted-foreground oklch(0.49 0 0) on card oklch(1 0 0) ≈ 4.5:1 AA ✓
//   dark:  muted-foreground oklch(0.78 0 0) on card oklch(0.285 0 0) ≈ 5.5:1 AA ✓

import { useEffect, useMemo, useState } from 'react'

import { useTheme } from '@/context/theme-provider'

/** Fallback ordinal palette (blue / cyan / violet / pink / emerald). */
const CHART_SERIES_FALLBACK = [
  '#3b82f6',
  '#06b6d4',
  '#8b5cf6',
  '#ec4899',
  '#10b981',
] as const

export interface ChartColors {
  /** axis / legend label fill — resolves --muted-foreground. */
  axisLabel: string
  /** grid / tick / domainLine stroke — resolves --border. */
  grid: string
  /** ink on primary fills (point strokes) — resolves --primary-foreground. */
  onPrimary: string
  /** ordinal series palette — resolves --chart-1..5 (canvas cannot resolve var()). */
  series: string[]
  /** derived soft fills (~20% alpha) for series accents. */
  seriesSoft: string[]
  /** Derived faint fills (~5% alpha) for translucent chart areas. */
  seriesFaint: string[]
}

export const CHART_COLORS_FALLBACK: ChartColors = {
  axisLabel: '#6b7280',
  grid: '#e5e7eb',
  onPrimary: '#ffffff',
  series: [...CHART_SERIES_FALLBACK],
  seriesSoft: CHART_SERIES_FALLBACK.map((color) => `${color}33`),
  seriesFaint: CHART_SERIES_FALLBACK.map((color) => `${color}0d`),
}

/**
 * Append an alpha channel to a concrete CSS color. Supports the three forms
 * the OKLCH theme emits: `oklch(L C H)`, `rgb(r g b)`, and `#hex`. Anything
 * unrecognised is returned unchanged (VChart will ignore it).
 */
function withAlpha(color: string, alpha: number): string {
  const trimmed = color.trim()
  if (trimmed.startsWith('oklch(') && trimmed.endsWith(')')) {
    const inner = trimmed.slice('oklch('.length, -1).trim()
    return `oklch(${inner} / ${alpha})`
  }
  if (trimmed.startsWith('rgb(') && trimmed.endsWith(')')) {
    const inner = trimmed.slice('rgb('.length, -1).trim()
    return `rgba(${inner}, ${alpha})`
  }
  if (trimmed.startsWith('#')) {
    const alphaHex = Math.round(alpha * 255)
      .toString(16)
      .padStart(2, '0')
    return `${trimmed}${alphaHex}`
  }
  return trimmed
}

/** Reads --chart-1..5 into concrete canvas colors. */
function readChartSeries(
  style: CSSStyleDeclaration,
  fallback: readonly string[]
): string[] {
  const resolved: string[] = []
  for (let index = 1; index <= fallback.length; index += 1) {
    const value = style.getPropertyValue(`--chart-${index}`).trim()
    resolved.push(value || fallback[index - 1])
  }
  return resolved
}

/** Read the full chart palette from the document root. */
function readChartColors(): ChartColors {
  if (
    typeof document === 'undefined' ||
    typeof globalThis.getComputedStyle !== 'function'
  ) {
    return CHART_COLORS_FALLBACK
  }

  const style = globalThis.getComputedStyle(document.documentElement)
  const axisLabel = style.getPropertyValue('--muted-foreground').trim()
  const grid = style.getPropertyValue('--border').trim()
  const onPrimary = style.getPropertyValue('--primary-foreground').trim()
  const series = readChartSeries(style, CHART_SERIES_FALLBACK)

  return {
    axisLabel: axisLabel || CHART_COLORS_FALLBACK.axisLabel,
    grid: grid || CHART_COLORS_FALLBACK.grid,
    onPrimary: onPrimary || CHART_COLORS_FALLBACK.onPrimary,
    series,
    seriesSoft: series.map((color) => withAlpha(color, 0.2)),
    seriesFaint: series.map((color) => withAlpha(color, 0.05)),
  }
}

/**
 * Resolve the chart palette to concrete colors for VChart canvas specs.
 *
 * Re-reads on every `resolvedTheme` change (the theme-provider flips the
 * `light` / `dark` class on <html>) and on theme-preset / radius / scale
 * attribute mutations (which rewrite chart tokens without flipping
 * resolvedTheme). The rAF deferral past the theme-provider's own effect
 * guarantees getComputedStyle sees the freshly-applied tokens.
 */
export function useChartColors(): ChartColors {
  const { resolvedTheme } = useTheme()
  const [colors, setColors] = useState<ChartColors>(readChartColors)

  useEffect(() => {
    const requestId = requestAnimationFrame(() => {
      setColors(readChartColors())
    })
    return () => cancelAnimationFrame(requestId)
  }, [resolvedTheme])

  useEffect(() => {
    const root =
      typeof document !== 'undefined' ? document.documentElement : null
    if (!root || typeof globalThis.MutationObserver !== 'function') {
      return undefined
    }
    const observer = new globalThis.MutationObserver(() => {
      setColors(readChartColors())
    })
    observer.observe(root, {
      attributes: true,
      attributeFilter: [
        'class',
        'data-theme-preset',
        'data-theme-radius',
        'data-theme-scale',
      ],
    })
    return () => observer.disconnect()
  }, [])

  return colors
}

/**
 * Single label color for donut / pie outside labels. In the new theme this is
 * the same token as {@link ChartColors.axisLabel}; kept as a separate hook to
 * preserve the legacy chart call sites (CostDistribution / SiteDistribution).
 */
export function useThemeLabelColor(fallback = '#6b7280'): string {
  const { axisLabel } = useChartColors()
  return useMemo(() => axisLabel || fallback, [axisLabel, fallback])
}
