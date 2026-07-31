import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { ToastProvider } from '../../components/Toast.js';
import RatesOverviewSection from './RatesOverviewSection.js';

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getRateOverview: vi.fn(),
  },
}));

vi.mock('../../api.js', () => ({
  api: apiMock,
}));

function collectText(node: ReactTestInstance): string {
  const children = node.children || [];
  return children.map((child) => {
    if (typeof child === 'string') return child;
    return collectText(child);
  }).join('');
}

async function flushMicrotasks() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe('RatesOverviewSection', () => {
  const originalDocument = globalThis.document;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.getRateOverview.mockResolvedValue({
      generatedAt: '2026-08-01T00:00:00Z',
      summary: { accountsWithUnitCost: 1, accountsTotal: 2, channelsTotal: 3, channelsEnabled: 2 },
      accounts: [
        { accountId: 1, username: 'rate-user', siteId: 1, siteName: 'RateSite', unitCost: 0.0042, channelCount: 1, totalWeight: 30 },
        { accountId: 2, username: 'plain-user', siteId: 1, siteName: 'RateSite', unitCost: null, channelCount: 0, totalWeight: 0 },
      ],
      channels: [
        { channelId: 1, routeId: 1, routePattern: 'gpt-4o', accountId: 1, username: 'rate-user', modelName: 'gpt-4o', weight: 30, enabled: true },
      ],
      sites: [{ siteId: 1, siteName: 'RateSite', globalWeight: 2.5 }],
      keys: [{ keyId: 1, name: 'rate-key', keyWeight: 1.7 }],
      models: [{ model: 'gpt-4o', calls: 100, spend: 0.21, tokens: 5000 }],
    });
    globalThis.document = {
      documentElement: {
        getAttribute: () => 'light',
      },
    } as unknown as Document;
    globalThis.MutationObserver = class {
      observe() {}
      disconnect() {}
    } as unknown as typeof MutationObserver;
  });

  afterEach(() => {
    globalThis.document = originalDocument;
    globalThis.MutationObserver = originalMutationObserver;
  });

  it('renders all multiplier surfaces from the overview', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(
        <ToastProvider>
          <RatesOverviewSection />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    expect(apiMock.getRateOverview).toHaveBeenCalled();
    const text = collectText(renderer!.root);
    expect(text).toContain('$0.0042'); // account unit cost
    expect(text).toContain('30.00'); // channel weight
    expect(text).toContain('2.50'); // site global weight
    expect(text).toContain('1.70'); // key weight
    expect(text).toContain('$0.21'); // observed model spend
    expect(text).toContain('2/3'); // enabled/total channels

    renderer!.unmount();
  });

  it('renders empty state for no data', async () => {
    apiMock.getRateOverview.mockResolvedValue({
      generatedAt: '2026-08-01T00:00:00Z',
      summary: { accountsWithUnitCost: 0, accountsTotal: 0, channelsTotal: 0, channelsEnabled: 0 },
      accounts: [],
      channels: [],
      sites: [],
      keys: [],
      models: [],
    });
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(
        <ToastProvider>
          <RatesOverviewSection />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const text = collectText(renderer!.root);
    expect(text).toContain('倍率与权重总览');

    renderer!.unmount();
  });
});
