import { describe, it, expect, beforeEach } from 'vitest';
import { act, create } from 'react-test-renderer';
import { CHART_COLORS_FALLBACK, useChartColors, useThemeLabelColor } from './useThemeLabelColor.js';

// Probe component renders hook values into a JSON string for assertion.
function ProbeColors() {
  const colors = useChartColors();
  return <span data-testid="colors">{JSON.stringify(colors)}</span>;
}

function ProbeLabel() {
  const label = useThemeLabelColor('#9ca3af');
  return <span data-testid="label">{label}</span>;
}

function readJson(renderer: ReturnType<typeof create>): { axisLabel: string; grid: string } {
  return JSON.parse(renderer.root.findByType('span').props.children);
}

describe('useChartColors', () => {
  beforeEach(() => {
    document.documentElement.style.setProperty('--color-text-secondary', '');
    document.documentElement.style.setProperty('--color-border-light', '');
  });

  it('falls back when CSS variables are unavailable', () => {
    let renderer!: ReturnType<typeof create>;
    act(() => {
      renderer = create(<ProbeColors />);
    });
    expect(readJson(renderer)).toEqual(CHART_COLORS_FALLBACK);
  });

  it('reads token values once defined on the root element', async () => {
    document.documentElement.style.setProperty('--color-text-secondary', '#5f6368');
    document.documentElement.style.setProperty('--color-border-light', '#f1f3f4');

    let renderer!: ReturnType<typeof create>;
    await act(async () => {
      renderer = create(<ProbeColors />);
    });
    expect(readJson(renderer)).toEqual({ axisLabel: '#5f6368', grid: '#f1f3f4' });
  });

  it('keeps previous values when a token is missing', async () => {
    document.documentElement.style.setProperty('--color-text-secondary', '#9aa0a6');

    let renderer!: ReturnType<typeof create>;
    await act(async () => {
      renderer = create(<ProbeColors />);
    });
    const colors = readJson(renderer);
    expect(colors.axisLabel).toBe('#9aa0a6');
    expect(colors.grid).toBe(CHART_COLORS_FALLBACK.grid);
  });
});

describe('useThemeLabelColor', () => {
  beforeEach(() => {
    document.documentElement.style.setProperty('--color-text-secondary', '');
  });

  it('keeps fallback when theme APIs are unavailable', async () => {
    let renderer!: ReturnType<typeof create>;
    await act(async () => {
      renderer = create(<ProbeLabel />);
    });
    expect(renderer.root.findByType('span').props.children).toBe('#9ca3af');
  });
});
