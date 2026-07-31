import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import CostDistributionChart from './CostDistributionChart.js';

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
    getModelCostDistribution: vi.fn(),
  },
}));

vi.mock('../../api.js', () => ({
  api: apiMock,
}));

describe('CostDistributionChart', () => {
  const originalDocument = globalThis.document;
  const originalGetComputedStyle = globalThis.getComputedStyle;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vChartSpy.mockClear();
    apiMock.getModelCostDistribution.mockResolvedValue({
      days: 30,
      since: '2026-07-02T00:00:00Z',
      topN: 8,
      items: [
        { model: 'gpt-5', label: 'gpt-5', cost: 5, calls: 10, tokens: 1000 },
        { model: 'other', label: '其他模型', cost: 0.5, calls: 1, tokens: 50 },
      ],
      totals: { cost: 5.5, calls: 11, tokens: 1050 },
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

  it('renders a pie spec keyed on cost with topN + Other items', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<CostDistributionChart days={30} topN={8} />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(apiMock.getModelCostDistribution).toHaveBeenCalledWith(30, 8);
    const spec = vChartSpy.mock.calls[0]?.[0]?.spec as {
      type: string;
      valueField: string;
      categoryField: string;
      data?: Array<{ values?: Array<{ model?: string; cost?: number }> }>;
    };
    expect(spec.type).toBe('pie');
    expect(spec.valueField).toBe('cost');
    expect(spec.categoryField).toBe('label');
    const values = spec.data?.[0]?.values ?? [];
    expect(values).toHaveLength(2);
    expect(values[0]).toMatchObject({ model: 'gpt-5', cost: 5 });
    expect(values[1]).toMatchObject({ model: 'other' });

    renderer.unmount();
  });

  it('shows total cost in the header', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<CostDistributionChart />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const text = collectText(renderer!.root);
    expect(text).toContain('总成本 $5.50');

    renderer.unmount();
  });
});
