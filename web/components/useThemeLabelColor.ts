import { useEffect, useState } from 'react';

function readThemeLabelColor(): string | null {
  if (typeof document === 'undefined') return null;

  const root = document.documentElement;
  if (!root || typeof globalThis.getComputedStyle !== 'function') return null;

  const color = globalThis.getComputedStyle(root).getPropertyValue('--color-text-secondary').trim();
  return color || null;
}

export function useThemeLabelColor(fallback = '#9ca3af'): string {
  const [labelColor, setLabelColor] = useState(fallback);

  useEffect(() => {
    const root = typeof document !== 'undefined' ? document.documentElement : null;
    const read = () => {
      const color = readThemeLabelColor();
      if (color) setLabelColor(color);
    };

    read();

    if (!root || typeof globalThis.MutationObserver !== 'function') {
      return undefined;
    }

    const observer = new globalThis.MutationObserver(read);
    observer.observe(root, { attributes: true, attributeFilter: ['data-theme'] });
    return () => observer.disconnect();
  }, []);

  return labelColor;
}

/**
 * Chart token colors resolved via JS (canvas cannot resolve CSS var()).
 *
 * VChart renders to <canvas>; `fill: 'var(--color-text-muted)'` is not a valid
 * canvas color and falls back to VChart's dark default — invisible axis/legend
 * text on dark themes. Resolve tokens through getComputedStyle like
 * useThemeLabelColor (2026-08-01 chart contrast pass).
 *
 * Contrast (WCAG, on card bg):
 *  - light: axisLabel #5f6368 on #fff = 6.05:1 AA ✓ · grid #f1f3f4 decorative
 *  - dark:  axisLabel #9aa0a6 on #202124 = 6.09:1 AA ✓ · grid #3c4043 decorative
 */

/** Reads --color-chart-1..8 (+ optional suffix) into concrete canvas colors. */
function readChartSeries(
  style: CSSStyleDeclaration,
  fallback: string[],
  suffix = '',
): string[] {
  const out: string[] = [];
  for (let i = 1; i <= 8; i++) {
    const value = style
      .getPropertyValue(`--color-chart-${i}${suffix}`)
      .trim();
    out.push(value || fallback[i - 1] || '#9ca3af');
  }
  return out;
}

export interface ChartColors {
  /** axis/legend label fill — resolves to --color-text-secondary */
  axisLabel: string;
  /** grid/tick/domainLine stroke — resolves to --color-border-light */
  grid: string;
  /** series palette — resolves --color-chart-1..8 (canvas cannot resolve var()) */
  series: string[];
  /** derived soft fills for series accents — --color-chart-N-soft (color-mix resolved) */
  seriesSoft: string[];
  /** derived faint fills — --color-chart-N-faint */
  seriesFaint: string[];
  /** ink on primary fills (point strokes) — resolves --color-on-primary */
  onPrimary: string;
}

const CHART_SERIES_FALLBACK = [
  '#1a73e8', '#12b5cb', '#1e8e3e', '#f9ab00',
  '#d93025', '#a142f4', '#e52592', '#e8710a',
];

export const CHART_COLORS_FALLBACK: ChartColors = {
  axisLabel: '#9ca3af',
  grid: '#e5e7eb',
  series: CHART_SERIES_FALLBACK,
  seriesSoft: CHART_SERIES_FALLBACK.map((c) => `${c}33`), // ~20% alpha approximation
  seriesFaint: CHART_SERIES_FALLBACK.map((c) => `${c}08`), // ~3% alpha approximation
  onPrimary: '#ffffff',
};

export function useChartColors(): ChartColors {
  const [colors, setColors] = useState<ChartColors>(CHART_COLORS_FALLBACK);

  useEffect(() => {
    const root = typeof document !== 'undefined' ? document.documentElement : null;
    const read = () => {
      if (!root || typeof globalThis.getComputedStyle !== 'function') return;
      const style = globalThis.getComputedStyle(root);
      const axisLabel = style.getPropertyValue('--color-text-secondary').trim();
      const grid = style.getPropertyValue('--color-border-light').trim();
      const onPrimary = style.getPropertyValue('--color-on-primary').trim();
      if (axisLabel || grid) {
        setColors((prev) => ({
          axisLabel: axisLabel || prev.axisLabel,
          grid: grid || prev.grid,
          series: readChartSeries(style, prev.series),
          seriesSoft: readChartSeries(style, prev.seriesSoft, '-soft'),
          seriesFaint: readChartSeries(style, prev.seriesFaint, '-faint'),
          onPrimary: onPrimary || prev.onPrimary,
        }));
      }
    };

    read();

    if (!root || typeof globalThis.MutationObserver !== 'function') {
      return undefined;
    }

    const observer = new globalThis.MutationObserver(read);
    observer.observe(root, { attributes: true, attributeFilter: ['data-theme'] });
    return () => observer.disconnect();
  }, []);

  return colors;
}
