import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import IncomeOutcomeChart from './IncomeOutcomeChart.js';

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
    getBalanceIncomeOutcome: vi.fn(),
  },
}));

vi.mock('../../api.js', () => ({
  api: apiMock,
}));

describe('IncomeOutcomeChart', () => {
  const originalDocument = globalThis.document;
  const originalGetComputedStyle = globalThis.getComputedStyle;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vChartSpy.mockClear();
    apiMock.getBalanceIncomeOutcome.mockResolvedValue({
      days: 30,
      points: [
        { day: '2026-07-01', income: 12, outcome: 0, net: 12 },
        { day: '2026-07-02', income: 1, outcome: 3, net: -2 },
        { day: '2026-07-03', income: 3, outcome: 0, net: 3 },
      ],
      summary: { totalIncome: 16, totalOutcome: 3, net: 13, accounts: 1 },
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

  it('renders a grouped bar spec with income/consumption series per day', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<IncomeOutcomeChart days={30} />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(apiMock.getBalanceIncomeOutcome).toHaveBeenCalledWith(30);
    const spec = vChartSpy.mock.calls[0]?.[0]?.spec as {
      type: string;
      xField: string;
      yField: string;
      seriesField: string;
      data?: Array<{ values?: Array<{ day: string; type: string; value: number }> }>;
    };
    expect(spec.type).toBe('bar');
    expect(spec.xField).toBe('day');
    expect(spec.yField).toBe('value');
    expect(spec.seriesField).toBe('type');
    const values = spec.data?.[0]?.values ?? [];
    expect(values).toHaveLength(6); // 3 days × (income + consumption)
    const firstIncome = values.find((v) => v.day === '2026-07-01' && v.type === '收入');
    const firstOutcome = values.find((v) => v.day === '2026-07-01' && v.type === '消费');
    expect(firstIncome?.value).toBe(12);
    expect(firstOutcome?.value).toBe(0);

    renderer.unmount();
  });

  it('shows income/consumption/net summary in the header', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<IncomeOutcomeChart days={30} />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const text = collectText(renderer!.root);
    expect(text).toContain('收入');
    expect(text).toContain('$16.00');
    expect(text).toContain('$3.00');
    expect(text).toContain('$13.00'); // net

    renderer.unmount();
  });

  it('shows empty state when there are no snapshots', async () => {
    apiMock.getBalanceIncomeOutcome.mockResolvedValue({
      days: 30,
      points: [],
      summary: { totalIncome: 0, totalOutcome: 0, net: 0, accounts: 0 },
    });
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<IncomeOutcomeChart days={30} />);
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(vChartSpy).not.toHaveBeenCalled();
    const text = collectText(renderer!.root);
    expect(text).toContain('暂无余额历史');

    renderer.unmount();
  });
});
