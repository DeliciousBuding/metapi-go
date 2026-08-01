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
 * useThemeLabelColor (2026-08-01 chart contrast pass, ui-ux-refresh residual).
 *
 * Contrast (WCAG, on card bg):
 *  - light: axisLabel #5f6368 on #fff = 6.05:1 AA ✓ · grid #f1f3f4 decorative
 *  - dark:  axisLabel #9aa0a6 on #202124 = 6.09:1 AA ✓ · grid #3c4043 decorative
 */
export interface ChartColors {
  /** axis/legend label fill — resolves to --color-text-secondary */
  axisLabel: string;
  /** grid/tick/domainLine stroke — resolves to --color-border-light */
  grid: string;
}

export const CHART_COLORS_FALLBACK: ChartColors = {
  axisLabel: '#9ca3af',
  grid: '#e5e7eb',
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
      if (axisLabel || grid) {
        setColors((prev) => ({
          axisLabel: axisLabel || prev.axisLabel,
          grid: grid || prev.grid,
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
