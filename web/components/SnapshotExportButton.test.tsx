import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { ToastProvider } from './Toast.js';
import SnapshotExportButton from './SnapshotExportButton.js';

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getDashboardSnapshot: vi.fn(),
    getSiteDistribution: vi.fn(),
  },
}));

vi.mock('../api.js', () => ({
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

/** Canvas 2D context stub that records operations. */
function makeCtxStub() {
  return new Proxy({}, {
    get(_target, prop) {
      if (prop === 'measureText') return () => ({ width: 10 });
      return () => undefined;
    },
    set() {
      return true;
    },
  }) as unknown as CanvasRenderingContext2D;
}

describe('SnapshotExportButton', () => {
  const originalDocument = globalThis.document;
  const originalCreateElement = globalThis.document?.createElement;
  const originalCreateObjectURL = globalThis.URL?.createObjectURL;

  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.getDashboardSnapshot.mockResolvedValue({
      totalBalance: 123.45,
      totalUsed: 67.89,
      todaySpend: 3.21,
      activeAccounts: 4,
      proxy24h: { total: 100, success: 96, totalTokens: 50000 },
      generatedAt: '2026-08-01T00:00:00Z',
    });
    apiMock.getSiteDistribution.mockResolvedValue({
      distribution: [
        { siteName: 'Claude 站', totalSpend: 2.5 },
        { siteName: 'Gemini 站', totalSpend: 0.7 },
      ],
    });

    // Fake canvas element: getContext returns a stub, toBlob resolves.
    const fakeCanvas = {
      width: 0,
      height: 0,
      getContext: vi.fn(() => makeCtxStub()),
      toBlob: vi.fn((cb: (blob: Blob | null) => void) => {
        cb(new Blob(['png'], { type: 'image/png' }));
      }),
    };
    globalThis.document = {
      documentElement: { getAttribute: () => 'light' },
      createElement: vi.fn(() => fakeCanvas),
      createElementNS: originalCreateElement,
    } as unknown as Document;
    globalThis.MutationObserver = class {
      observe() {}
      disconnect() {}
    } as unknown as typeof MutationObserver;
    // URL.createObjectURL may be missing in jsdom — stub it.
    if (!globalThis.URL) {
      (globalThis as any).URL = {};
    }
    globalThis.URL.createObjectURL = vi.fn(() => 'blob:fake');
    globalThis.URL.revokeObjectURL = vi.fn();
  });

  afterEach(() => {
    globalThis.document = originalDocument;
    if (originalCreateObjectURL) globalThis.URL.createObjectURL = originalCreateObjectURL;
    else delete (globalThis.URL as any).createObjectURL;
  });

  it('exports a PNG snapshot from dashboard data', async () => {
    let root!: ReactTestRenderer;

    await expect(act(async () => {
      root = create(
        <ToastProvider>
          <SnapshotExportButton />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const button = root!.root.find((node) => (
      node.type === 'button'
      && node.props['data-testid'] === 'export-snapshot'
    ));
    expect(collectText(button)).toContain('导出快照');

    await act(async () => {
      await button.props.onClick();
    });
    await flushMicrotasks();

    expect(apiMock.getDashboardSnapshot).toHaveBeenCalled();
    expect(apiMock.getSiteDistribution).toHaveBeenCalled();
    expect(globalThis.URL.createObjectURL).toHaveBeenCalled();

    root!.unmount();
  });

  it('shows an error toast when toBlob is unavailable', async () => {
    // Override fake canvas so toBlob is missing.
    const bareCanvas = {
      width: 0,
      height: 0,
      getContext: vi.fn(() => makeCtxStub()),
    };
    globalThis.document.createElement = vi.fn(() => bareCanvas) as unknown as typeof document.createElement;

    let root!: ReactTestRenderer;

    await expect(act(async () => {
      root = create(
        <ToastProvider>
          <SnapshotExportButton />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const button = root!.root.find((node) => (
      node.type === 'button'
      && node.props['data-testid'] === 'export-snapshot'
    ));
    await act(async () => {
      await button.props.onClick();
    });
    await flushMicrotasks();

    expect(globalThis.URL.createObjectURL).not.toHaveBeenCalled();

    root!.unmount();
  });
});
