import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { ToastProvider } from '../../components/Toast.js';
import AnnouncementsSection from './AnnouncementsSection.js';

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getAnnouncements: vi.fn(),
    createAnnouncement: vi.fn(),
    updateAnnouncement: vi.fn(),
    deleteAnnouncement: vi.fn(),
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

describe('AnnouncementsSection', () => {
  const originalDocument = globalThis.document;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.getAnnouncements.mockResolvedValue({
      items: [
        {
          id: 1,
          title: '上游故障',
          message: 'Claude 上游 API 故障中',
          severity: 'critical',
          link: null,
          enabled: true,
          dismissed: false,
          createdAt: '2026-08-01T00:00:00Z',
          updatedAt: '2026-08-01T00:00:00Z',
        },
      ],
    });
    apiMock.createAnnouncement.mockResolvedValue({ items: [] });
    apiMock.updateAnnouncement.mockResolvedValue({ success: true, revision: false });
    apiMock.deleteAnnouncement.mockResolvedValue({ success: true });
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

  it('creates a new announcement from the form', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(
        <ToastProvider>
          <AnnouncementsSection />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    // Open the create form.
    const newButton = renderer!.root.find((node) => (
      node.type === 'button'
      && node.props['data-testid'] === 'new-announcement'
    ));
    await act(async () => {
      await newButton.props.onClick();
    });

    // Fill title + message.
    const titleInput = renderer!.root.find((node) => node.props['data-testid'] === 'announcement-title');
    const messageInput = renderer!.root.find((node) => node.props['data-testid'] === 'announcement-message');
    await act(async () => {
      await titleInput.props.onChange({ target: { value: '计划维护' } });
      await messageInput.props.onChange({ target: { value: '周六 02:00-03:00 维护' } });
    });

    const saveButton = renderer!.root.find((node) => node.props['data-testid'] === 'announcement-save');
    await act(async () => {
      await saveButton.props.onClick();
    });
    await flushMicrotasks();

    expect(apiMock.createAnnouncement).toHaveBeenCalledWith({
      title: '计划维护',
      message: '周六 02:00-03:00 维护',
      severity: 'info',
      link: null,
      enabled: true,
    });

    renderer!.unmount();
  });

  it('deletes an announcement', async () => {
    let renderer!: ReactTestRenderer;

    await expect(act(async () => {
      renderer = create(
        <ToastProvider>
          <AnnouncementsSection />
        </ToastProvider>,
      );
    })).resolves.toBeUndefined();
    await flushMicrotasks();

    const text = collectText(renderer!.root);
    expect(text).toContain('上游故障');

    const deleteButton = renderer!.root.findAll((node) => (
      node.type === 'button'
      && typeof node.props.onClick === 'function'
      && collectText(node) === '删除'
    ))[0];
    await act(async () => {
      await deleteButton.props.onClick();
    });
    await flushMicrotasks();

    expect(apiMock.deleteAnnouncement).toHaveBeenCalledWith(1);

    renderer!.unmount();
  });
});
