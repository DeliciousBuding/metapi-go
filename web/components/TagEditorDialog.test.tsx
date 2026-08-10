import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import TagEditorDialog from './TagEditorDialog.js';

function collectText(node: ReactTestInstance): string {
  const children = node.children || [];
  return children.map((child) => {
    if (typeof child === 'string') return child;
    return collectText(child);
  }).join('');
}

describe('TagEditorDialog', () => {
  const originalDocument = globalThis.document;
  const originalMutationObserver = globalThis.MutationObserver;

  beforeEach(() => {
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

  it('pre-fills current tags and saves the edited list', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const onClose = vi.fn();
    let root!: ReactTestRenderer;

    await expect(act(async () => {
      root = create(
        <TagEditorDialog
          open
          onClose={onClose}
          title="账号标签"
          initialTags={'["prod","alpha"]'}
          allTags={['prod', 'alpha', 'backup']}
          onSave={onSave}
        />,
      );
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
    });

    // Quick-add a suggestion chip.
    const backupChip = root!.root.find((node) => (
      node.type === 'button'
      && typeof node.props.onClick === 'function'
      && collectText(node) === '+ backup'
    ));
    await act(async () => {
      await backupChip.props.onClick();
    });

    // Save.
    const saveButton = root!.root.find((node) => (
      node.type === 'button'
      && typeof node.props.onClick === 'function'
      && collectText(node) === '保存'
    ));
    await act(async () => {
      await saveButton.props.onClick();
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave.mock.calls[0][0]).toEqual(['prod', 'alpha', 'backup']);
    expect(onClose).toHaveBeenCalled();

    root!.unmount();
  });

  it('removes a tag when its × button is clicked', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    let root!: ReactTestRenderer;

    await expect(act(async () => {
      root = create(
        <TagEditorDialog
          open
          onClose={() => {}}
          title="站点标签"
          initialTags={'["prod","alpha"]'}
          allTags={[]}
          onSave={onSave}
        />,
      );
    })).resolves.toBeUndefined();
    await act(async () => {
      await Promise.resolve();
    });

    const removeProd = root!.root.find((node) => (
      node.type === 'button'
      && node.props['aria-label'] === '移除标签 prod'
    ));
    await act(async () => {
      await removeProd.props.onClick();
    });

    const saveButton = root!.root.find((node) => (
      node.type === 'button'
      && typeof node.props.onClick === 'function'
      && collectText(node) === '保存'
    ));
    await act(async () => {
      await saveButton.props.onClick();
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(onSave.mock.calls[0][0]).toEqual(['alpha']);

    root!.unmount();
  });
});
