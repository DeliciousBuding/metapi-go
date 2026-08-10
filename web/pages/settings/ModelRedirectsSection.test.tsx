import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { ToastProvider } from '../../components/Toast.js';
import ModelRedirectsSection from './ModelRedirectsSection.js';

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getModelRedirects: vi.fn(),
    generateModelRedirects: vi.fn(),
    applyModelRedirects: vi.fn(),
    updateModelRedirect: vi.fn(),
    deleteModelRedirect: vi.fn(),
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

describe('ModelRedirectsSection', () => {
  const originalDocument = globalThis.document;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.getModelRedirects.mockResolvedValue({
      items: [
        {
          id: 1,
          accountId: 10,
          username: 'redirect-user',
          siteName: 'RedirectSite',
          canonical: 'claude-3-5-sonnet',
          actual: 'claude-3-5-sonnet-20241022',
          source: 'sync',
          lastSeenAt: '2026-08-01T00:00:00Z',
          createdAt: '2026-08-01T00:00:00Z',
          updatedAt: '2026-08-01T00:00:00Z',
        },
      ],
    });
    apiMock.generateModelRedirects.mockResolvedValue({ success: true, created: 3, accounts: 2 });
    apiMock.applyModelRedirects.mockResolvedValue({
      success: true,
      dryRun: true,
      candidates: [
        {
          siteId: 1,
          siteName: 'RedirectSite',
          accountId: 10,
          modelName: 'claude-3-5-sonnet',
          canonical: 'claude-3-5-sonnet',
          actual: 'claude-3-5-sonnet-20241022',
        },
      ],
      count: 1,
    });
    apiMock.updateModelRedirect.mockResolvedValue({ success: true });
    apiMock.deleteModelRedirect.mockResolvedValue({ success: true });
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

  it('lists mappings and promotes one to manual', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(
        <ToastProvider>
          <ModelRedirectsSection />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const text = collectText(renderer!.root);
    expect(text).toContain('claude-3-5-sonnet');
    expect(text).toContain('claude-3-5-sonnet-20241022');
    expect(text).toContain('自动');

    const promoteButton = renderer!.root.find((node) => (
      node.type === 'button'
      && typeof node.props.onClick === 'function'
      && collectText(node) === '转手动'
    ));
    await act(async () => {
      await promoteButton.props.onClick();
    });
    await flushMicrotasks();

    expect(apiMock.updateModelRedirect).toHaveBeenCalledWith(1, { source: 'manual' });

    renderer!.unmount();
  });

  it('generates mappings and previews fixable disabled models', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(
        <ToastProvider>
          <ModelRedirectsSection />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    // Generate.
    const generateButton = renderer!.root.find((node) => (
      node.type === 'button'
      && node.props['data-testid'] === 'redirects-generate'
    ));
    await act(async () => {
      await generateButton.props.onClick();
    });
    await flushMicrotasks();
    expect(apiMock.generateModelRedirects).toHaveBeenCalledWith(0);

    // Preview fix candidates.
    apiMock.applyModelRedirects.mockResolvedValueOnce({
      success: true,
      dryRun: true,
      candidates: [
        {
          siteId: 1,
          siteName: 'RedirectSite',
          accountId: 10,
          modelName: 'claude-3-5-sonnet',
          canonical: 'claude-3-5-sonnet',
          actual: 'claude-3-5-sonnet-20241022',
        },
      ],
      count: 1,
    });
    const previewButton = renderer!.root.find((node) => (
      node.type === 'button'
      && node.props['data-testid'] === 'redirects-preview'
    ));
    await act(async () => {
      await previewButton.props.onClick();
    });
    await flushMicrotasks();

    const text = collectText(renderer!.root);
    expect(text).toContain('可修复的禁用模型');
    expect(text).toContain('确认修复');

    // Confirm apply (non-dry-run).
    apiMock.applyModelRedirects.mockResolvedValueOnce({
      success: true,
      dryRun: false,
      removed: 1,
      count: 1,
    });
    const applyButton = renderer!.root.find((node) => (
      node.type === 'button'
      && node.props['data-testid'] === 'redirects-apply'
    ));
    await act(async () => {
      await applyButton.props.onClick();
    });
    await flushMicrotasks();

    expect(apiMock.applyModelRedirects).toHaveBeenLastCalledWith(false);

    renderer!.unmount();
  });
});
