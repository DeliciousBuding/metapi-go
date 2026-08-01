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
    document.documentElement.style.setProperty('--color-chart-1', '#1a73e8');
    document.documentElement.style.setProperty('--color-chart-2', '#12b5cb');
    document.documentElement.style.setProperty('--color-on-primary', '#ffffff');
    expect(readJson(renderer)).toEqual({
      axisLabel: '#5f6368',
      grid: '#f1f3f4',
      series: ['#1a73e8', '#12b5cb', '#1e8e3e', '#f9ab00', '#d93025', '#a142f4', '#e52592', '#e8710a'],
      seriesSoft: ['#1a73e833', '#12b5cb33', '#1e8e3e33', '#f9ab0033', '#d9302533', '#a142f433', '#e5259233', '#e8710a33'],
      seriesFaint: ['#1a73e808', '#12b5cb08', '#1e8e3e08', '#f9ab0008', '#d9302508', '#a142f408', '#e5259208', '#e8710a08'],
      onPrimary: '#ffffff',
    });
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
