import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { ToastProvider } from '../../components/Toast.js';
import AuditLogsSection from './AuditLogsSection.js';

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getAdminAuditLogs: vi.fn(),
  },
}));

vi.mock('../../api.js', () => ({
  api: apiMock,
}));

function collectText(node: ReactTestInstance): string {
  return (node.children || []).map((child) => {
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

describe('AuditLogsSection', () => {
  const originalDocument = globalThis.document;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.getAdminAuditLogs.mockResolvedValue({
      items: [
        {
          id: 3,
          actor: 'aabbccdd',
          method: 'DELETE',
          path: '/api/sites/3',
          status: 204,
          requestId: 'req-3',
          remoteIp: '10.0.0.1',
          createdAt: '2026-08-01T03:00:00Z',
        },
        {
          id: 2,
          actor: 'aabbccdd',
          method: 'POST',
          path: '/api/accounts',
          status: 200,
          requestId: 'req-2',
          remoteIp: '10.0.0.1',
          createdAt: '2026-08-01T02:00:00Z',
        },
        {
          id: 1,
          actor: '11223344',
          method: 'PUT',
          path: '/api/accounts/1/tags',
          status: 500,
          requestId: 'req-1',
          remoteIp: '10.0.0.2',
          createdAt: '2026-08-01T01:00:00Z',
        },
      ],
      total: 3,
      limit: 50,
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

  it('renders audit rows with method/status/actor and total count', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(
        <ToastProvider>
          <AuditLogsSection />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    expect(apiMock.getAdminAuditLogs).toHaveBeenCalledWith(expect.any(URLSearchParams));
    const text = collectText(renderer!.root);
    expect(text).toContain('共');
    expect(text).toContain('3');
    expect(text).toContain('DELETE');
    expect(text).toContain('/api/sites/3');
    expect(text).toContain('aabbccdd');
    expect(text).toContain('10.0.0.2');

    renderer!.unmount();
  });

  it('renders empty state when no audit rows exist', async () => {
    apiMock.getAdminAuditLogs.mockResolvedValue({ items: [], total: 0, limit: 50 });
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(
        <ToastProvider>
          <AuditLogsSection />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const text = collectText(renderer!.root);
    expect(text).toContain('暂无审计记录');

    renderer!.unmount();
  });
});
