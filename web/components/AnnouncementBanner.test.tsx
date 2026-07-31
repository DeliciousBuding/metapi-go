import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import AnnouncementBanner from './AnnouncementBanner.js';

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getActiveAnnouncements: vi.fn(),
    dismissAnnouncement: vi.fn(),
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

describe('AnnouncementBanner', () => {
  const originalDocument = globalThis.document;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.getActiveAnnouncements.mockResolvedValue({
      items: [
        {
          id: 1,
          title: '上游故障',
          message: 'Claude 上游 API 故障中',
          severity: 'critical',
          link: 'https://status.example.test',
          enabled: true,
          dismissed: false,
          createdAt: '2026-08-01T00:00:00Z',
          updatedAt: '2026-08-01T00:00:00Z',
        },
        {
          id: 2,
          title: '新功能',
          message: '已支持批量模型验证',
          severity: 'info',
          link: null,
          enabled: true,
          dismissed: false,
          createdAt: '2026-08-01T00:00:00Z',
          updatedAt: '2026-08-01T00:00:00Z',
        },
      ],
    });
    apiMock.dismissAnnouncement.mockResolvedValue({ success: true });
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

  it('renders active banners with severity content', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<AnnouncementBanner />);
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const text = collectText(renderer!.root);
    expect(text).toContain('上游故障');
    expect(text).toContain('Claude 上游 API 故障中');
    expect(text).toContain('详情');
    expect(text).toContain('已支持批量模型验证');

    renderer!.unmount();
  });

  it('dismisses a banner and removes it locally', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<AnnouncementBanner />);
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const dismissButtons = renderer!.root.findAll((node) => (
      node.type === 'button'
      && node.props['aria-label'] === '关闭公告'
    ));
    expect(dismissButtons.length).toBe(2);

    await act(async () => {
      await dismissButtons[0].props.onClick();
    });
    await flushMicrotasks();

    expect(apiMock.dismissAnnouncement).toHaveBeenCalledWith(1);
    const text = collectText(renderer!.root);
    expect(text).not.toContain('上游故障');
    expect(text).toContain('已支持批量模型验证');

    renderer!.unmount();
  });

  it('renders nothing when there are no active announcements', async () => {
    apiMock.getActiveAnnouncements.mockResolvedValue({ items: [] });
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(<AnnouncementBanner />);
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const text = collectText(renderer!.root);
    expect(text).toBe('');

    renderer!.unmount();
  });
});
