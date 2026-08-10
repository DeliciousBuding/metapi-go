import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import ModelVerifyDialog from './ModelVerifyDialog.js';

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    verifyModelsBatch: vi.fn(),
    getModelVerifyHistory: vi.fn(),
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

describe('ModelVerifyDialog', () => {
  const originalDocument = globalThis.document;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vi.clearAllMocks();
    // No document.body in the mock → CenteredModal renders inline (no portal).
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

  it('runs a verify batch and renders per-row results', async () => {
    apiMock.verifyModelsBatch.mockResolvedValue({
      success: true,
      batchId: 'vb-1',
      probed: 2,
      summary: { success: 1, failure: 1, inconclusive: 0, skipped: 0 },
      items: [
        { model: 'verify-model-a', siteName: 'VerifySite', status: 'success', latencyMs: 42, httpStatus: 200 },
        { model: 'verify-model-b', siteName: 'VerifySite', status: 'failure', latencyMs: 15, httpStatus: 429, errorText: 'rate limited' },
      ],
    });
    apiMock.getModelVerifyHistory.mockResolvedValue({ items: [] });
    let root!: ReactTestRenderer;

    await expect(act(async () => {
      root = create(<ModelVerifyDialog open models={['verify-model-a', 'verify-model-b']} onClose={() => {}} />);
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    // Click the verify button.
    const verifyButton = root!.root.find((node: ReactTestInstance) => (
      node.type === 'button'
      && typeof node.props.onClick === 'function'
      && collectText(node).includes('开始验证')
    ));
    await act(async () => {
      await verifyButton.props.onClick();
    });
    await flushMicrotasks();

    expect(apiMock.verifyModelsBatch).toHaveBeenCalledWith(['verify-model-a', 'verify-model-b']);
    const text = collectText(root!.root);
    expect(text).toContain('成功 1');
    expect(text).toContain('失败 1');
    expect(text).toContain('rate limited');
    expect(text).toContain('42 ms');

    root!.unmount();
  });

  it('switches to the history tab and lists past verifications', async () => {
    apiMock.getModelVerifyHistory.mockResolvedValue({
      items: [
        { model: 'verify-model-a', siteName: 'VerifySite', status: 'success', latencyMs: 42, httpStatus: 200, createdAt: '2026-08-01T01:00:00Z' },
        { model: 'verify-model-b', siteName: 'VerifySite', status: 'failure', latencyMs: 15, httpStatus: 429, createdAt: '2026-08-01T02:00:00Z' },
      ],
    });
    let root!: ReactTestRenderer;

    await expect(act(async () => {
      root = create(<ModelVerifyDialog open models={[]} onClose={() => {}} />);
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    // Switch to history tab.
    const historyTab = root!.root.find((node: ReactTestInstance) => (
      node.type === 'button'
      && typeof node.props.onClick === 'function'
      && collectText(node).includes('验证历史')
    ));
    await act(async () => {
      await historyTab.props.onClick();
    });
    await flushMicrotasks();

    expect(apiMock.getModelVerifyHistory).toHaveBeenCalledWith(50);
    const text = collectText(root!.root);
    expect(text).toContain('verify-model-a');
    expect(text).toContain('verify-model-b');
    expect(text).toContain('成功');
    expect(text).toContain('失败');

    root!.unmount();
  });
});
