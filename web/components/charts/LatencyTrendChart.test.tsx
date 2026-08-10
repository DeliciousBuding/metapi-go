import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import LatencyTrendChart from './LatencyTrendChart.js';

function collectText(node: ReactTestInstance): string {
  return (node.children || []).map((child) => {
    if (typeof child === 'string') return child;
    return collectText(child);
  }).join('');
}

const vChartSpy = vi.fn();

vi.mock('@visactor/react-vchart', () => ({
  VChart: (props: Record<string, unknown>) => {
    vChartSpy(props);
    return null;
  },
}));

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getLatencyTrend: vi.fn(),
  },
}));

vi.mock('../../api.js', () => ({
  api: apiMock,
}));

describe('LatencyTrendChart', () => {
  const originalDocument = globalThis.document;
  const originalGetComputedStyle = globalThis.getComputedStyle;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vChartSpy.mockClear();
    apiMock.getLatencyTrend.mockResolvedValue({
      days: 7,
      points: [
        { date: '2026-07-26', requests: 2, avgLatencyMs: 150, maxLatencyMs: 200, avgFirstByteMs: 50, p95LatencyMs: 200, successRate: 1 },
        { date: '2026-07-27', requests: 1, avgLatencyMs: 3000, maxLatencyMs: 3000, avgFirstByteMs: 800, p95LatencyMs: 3000, successRate: 0 },
      ],
      p95SampleCap: 10000,
      truncatedDays: [],
    });
    globalThis.document = {
      documentElement: {
        getAttribute: vi.fn(),
      },
    } as unknown as Document;
    Reflect.deleteProperty(globalThis as typeof globalThis & Record<string, unknown>, 'getComputedStyle');
    Reflect.deleteProperty(globalThis as typeof globalThis & Record<string, unknown>, 'MutationObserver');
  });

  afterEach(() => {
    globalThis.document = originalDocument;
    globalThis.getComputedStyle = originalGetComputedStyle;
    globalThis.MutationObserver = originalMutationObserver;
  });

  it('flattens avg + p95 into a seriesField line spec', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<LatencyTrendChart days={7} />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(apiMock.getLatencyTrend).toHaveBeenCalledWith(7);
    const spec = vChartSpy.mock.calls[0]?.[0]?.spec as {
      type: string;
      xField: string;
      yField: string;
      seriesField: string;
      data?: Array<{ values?: Array<{ date?: string; metric?: string; latency?: number }> }>;
    };
    expect(spec.type).toBe('line');
    expect(spec.xField).toBe('date');
    expect(spec.yField).toBe('latency');
    expect(spec.seriesField).toBe('metric');

    const values = spec.data?.[0]?.values ?? [];
    expect(values).toHaveLength(4); // 2 days × 2 metrics
    expect(values[0]).toMatchObject({ date: '2026-07-26', metric: '平均延迟', latency: 150 });
    expect(values[1]).toMatchObject({ date: '2026-07-26', metric: 'P95', latency: 200 });
    expect(values[2]).toMatchObject({ date: '2026-07-27', metric: '平均延迟', latency: 3000 });

    renderer.unmount();
  });

  it('surfaces truncated p95 days as a header note', async () => {
    apiMock.getLatencyTrend.mockResolvedValue({
      days: 7,
      points: [
        { date: '2026-07-26', requests: 300000, avgLatencyMs: 800, maxLatencyMs: 90000, avgFirstByteMs: 100, p95LatencyMs: 5000, successRate: 0.99 },
      ],
      p95SampleCap: 10000,
      truncatedDays: ['2026-07-26'],
    });
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<LatencyTrendChart days={7} />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const text = collectText(renderer!.root);
    expect(text).toContain('P95 采样截断（1 天）');

    renderer.unmount();
  });
});
