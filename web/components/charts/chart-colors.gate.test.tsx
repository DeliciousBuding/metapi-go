import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

/**
 * Chart canvas color gate (2026-08-01 chart contrast pass).
 *
 * VChart renders to <canvas> where `fill: 'var(--color-*)'` is not a valid
 * color — it silently falls back to VChart's dark default, which is
 * unreadable on dark themes. All chart axis/legend colors must therefore be
 * JS-resolved via useChartColors() (getComputedStyle), never CSS var().
 *
 * This static gate scans every chart component and fails when:
 *   1. any `var(--color-` remains inside a VChart spec (canvas-ineffective)
 *   2. an axis-bearing chart never references the resolved colors
 *   3. a chart that should resolve colors lacks the hook import
 */
const CHARTS_DIR = dirname(fileURLToPath(import.meta.url));
const files = readdirSync(CHARTS_DIR)
  .filter((f) => f.endsWith('.tsx') && !f.endsWith('.test.tsx') && !f.endsWith('.tmp.mjs'));

describe('chart canvas color gate', () => {
  it('no chart spec uses CSS var() colors (canvas cannot resolve them)', () => {
    const offenders: string[] = [];
    for (const f of files) {
      const src = readFileSync(join(CHARTS_DIR, f), 'utf-8');
      // React inline styles (DOM) legitimately use var(); only canvas paint
      // attributes (fill/stroke inside VChart specs) are broken by var().
      const m = src.match(/(?:fill|stroke): 'var\(--color[^)]*\)'/g);
      if (m) offenders.push(`${f}: ${m.join(', ')}`);
    }
    expect(offenders).toEqual([]);
  });

  it('every chart resolves colors via the hook import', () => {
    const missing: string[] = [];
    for (const f of files) {
      const src = readFileSync(join(CHARTS_DIR, f), 'utf-8');
      if (!src.includes('useChartColors')) {
        // SiteDistributionChart predates the hook and uses
        // useThemeLabelColor directly — the only allowed exception.
        if (f === 'SiteDistributionChart.tsx' && src.includes('useThemeLabelColor')) continue;
        missing.push(f);
      }
    }
    expect(missing).toEqual([]);
  });

  it('axis-bearing charts reference the resolved label/grid colors', () => {
    const missing: string[] = [];
    for (const f of files) {
      const src = readFileSync(join(CHARTS_DIR, f), 'utf-8');
      // Pie charts (Cost/SiteDistribution) have no axes and may skip grid.
      if (f.includes('CostDistribution') || f.includes('SiteDistribution')) continue;
      if (!src.includes('colors.axisLabel')) missing.push(`${f}: no axisLabel`);
      if (!src.includes('colors.grid')) missing.push(`${f}: no grid color`);
    }
    expect(missing).toEqual([]);
  });
});
