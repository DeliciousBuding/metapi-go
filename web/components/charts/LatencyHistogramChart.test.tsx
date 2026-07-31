import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import LatencyHistogramChart from './LatencyHistogramChart.js';

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
    getLatencyHistogram: vi.fn(),
  },
}));

vi.mock('../../api.js', () => ({
  api: apiMock,
}));

describe('LatencyHistogramChart', () => {
  const originalDocument = globalThis.document;
  const originalGetComputedStyle = globalThis.getComputedStyle;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vChartSpy.mockClear();
    apiMock.getLatencyHistogram.mockResolvedValue({
      days: 7,
      since: '2026-07-25T00:00:00Z',
      bucketMs: 500,
      total: 3,
      buckets: [
        { bucketStartMs: 0, bucketEndMs: 500, label: '0–500ms', count: 1, percent: 33.33 },
        { bucketStartMs: 500, bucketEndMs: 1000, label: '500–1000ms', count: 2, percent: 66.67 },
      ],
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

  it('renders a bar spec keyed on bucket label with request counts', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<LatencyHistogramChart days={7} bucketMs={500} />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(apiMock.getLatencyHistogram).toHaveBeenCalledWith(7, 500);
    const spec = vChartSpy.mock.calls[0]?.[0]?.spec as {
      type: string;
      xField: string;
      yField: string;
      data?: Array<{ values?: Array<{ label?: string; count?: number }> }>;
    };
    expect(spec.type).toBe('bar');
    expect(spec.xField).toBe('label');
    expect(spec.yField).toBe('count');
    const values = spec.data?.[0]?.values ?? [];
    expect(values).toHaveLength(2);
    expect(values[1]).toMatchObject({ label: '500–1000ms', count: 2 });

    renderer.unmount();
  });

  it('shows total request count in the header', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<LatencyHistogramChart />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const text = collectText(renderer!.root);
    expect(text).toContain('共 3 次请求');

    renderer.unmount();
  });
});
