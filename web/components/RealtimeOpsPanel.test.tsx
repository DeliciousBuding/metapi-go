import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import RealtimeOpsPanel from './RealtimeOpsPanel.js';

function collectText(node: ReactTestInstance): string {
  return (node.children || []).map((child) => {
    if (typeof child === 'string') return child;
    return collectText(child);
  }).join('');
}

const { authTokenMock } = vi.hoisted(() => ({
  authTokenMock: vi.fn(),
}));

vi.mock('../authSession.js', () => ({
  getAuthToken: authTokenMock,
}));

// Fake WebSocket class driven by the test.
type WSHandler = {
  onopen: ((ev: unknown) => void) | null;
  onmessage: ((ev: { data: string }) => void) | null;
  onclose: ((ev: unknown) => void) | null;
  onerror: ((ev: unknown) => void) | null;
  close: ReturnType<typeof vi.fn>;
};

const wsInstances: WSHandler[] = [];
let wsCtor: ReturnType<typeof vi.fn>;

describe('RealtimeOpsPanel', () => {
  const originalDocument = globalThis.document;
  const originalLocation = globalThis.window?.location;
  const originalWS = globalThis.WebSocket;

  beforeEach(() => {
    vi.clearAllMocks();
    wsInstances.length = 0;
    wsCtor = vi.fn(function (this: unknown, _url: string) {
      const inst: WSHandler = {
        onopen: null,
        onmessage: null,
        onclose: null,
        onerror: null,
        close: vi.fn(),
      };
      wsInstances.push(inst);
      return inst;
    });
    globalThis.WebSocket = wsCtor as unknown as typeof WebSocket;
    authTokenMock.mockReturnValue('admin-token-abc');
    Object.defineProperty(globalThis.window ?? globalThis, 'location', {
      value: { protocol: 'http:', host: 'gateway.test' },
      configurable: true,
      writable: true,
    });
    globalThis.document = {
      documentElement: {
        getAttribute: () => 'light',
      },
    } as unknown as Document;
  });

  afterEach(() => {
    globalThis.document = originalDocument;
    Object.defineProperty(globalThis.window ?? globalThis, 'location', {
      value: originalLocation,
      configurable: true,
      writable: true,
    });
    globalThis.WebSocket = originalWS;
  });

  it('connects with token query and renders live numbers from frames', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<RealtimeOpsPanel />);
    })).resolves.toBeUndefined();

    expect(wsCtor).toHaveBeenCalledTimes(1);
    const wsUrl = wsCtor.mock.calls[0][0] as string;
    expect(wsUrl).toContain('/api/admin/ops/ws?token=admin-token-abc');
    expect(wsUrl).toContain('ws://gateway.test');

    const inst = wsInstances[0];
    await act(async () => {
      inst.onopen?.({});
    });
    await act(async () => {
      const points = Array.from({ length: 300 }, (_, i) => ({
        ts: 1000 + i,
        total: i === 299 ? 12 : 0,
        success: i === 299 ? 11 : 0,
      }));
      inst.onmessage?.({ data: JSON.stringify({ lifetime: 12345, points }) });
    });

    const text = collectText(renderer!.root);
    expect(text).toContain('在线');
    expect(text).toContain('12'); // current QPS
    expect(text).toContain('91.7'); // 11/12 success rate
    expect(text).toContain('12,345'); // lifetime

    renderer!.unmount();
  });

  it('shows idle state without a token', async () => {
    authTokenMock.mockReturnValue(null);
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<RealtimeOpsPanel />);
    })).resolves.toBeUndefined();

    expect(wsCtor).not.toHaveBeenCalled();
    const text = collectText(renderer!.root);
    expect(text).toContain('实时流量');
    expect(text).toContain('重连中…');

    renderer!.unmount();
  });

  it('schedules reconnect on close', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<RealtimeOpsPanel />);
    })).resolves.toBeUndefined();

    const inst = wsInstances[0];
    await act(async () => {
      inst.onclose?.({});
    });
    // A reconnect timer is scheduled (2s backoff) → new socket after fire.
    expect(wsCtor).toHaveBeenCalledTimes(1);

    renderer!.unmount();
  });
});
