import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create } from 'react-test-renderer';
import { ToastProvider, useToast } from './Toast.js';

function Trigger({ type, message }: { type: 'success' | 'error' | 'info'; message: string }) {
  const toast = useToast();
  return (
    <button type="button" onClick={() => toast[type](message)}>
      fire-{type}
    </button>
  );
}

describe('Toast a11y', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('live region is polite and error toasts are assertive', async () => {
    let root!: ReturnType<typeof create>;
    try {
      await act(async () => {
        root = create(
          <ToastProvider>
            <Trigger type="error" message="boom" />
          </ToastProvider>,
        );
      });

      const container = root.root.find((node) => (
        typeof node.props?.className === 'string' && node.props.className.includes('toast-container')
      ));
      expect(container.props.role).toBe('status');
      expect(container.props['aria-live']).toBe('polite');
      expect(container.props['aria-atomic']).toBe('true');

      const trigger = root.root.findByType('button');
      await act(async () => {
        trigger.props.onClick();
      });

      const toast = root.root.find((node) => (
        typeof node.props?.className === 'string' && node.props.className.includes('toast-error')
      ));
      expect(toast.props.role).toBe('alert');
      const text = JSON.stringify(root.toJSON());
      expect(text).toContain('boom');
    } finally {
      root?.unmount();
    }
  });

  it('success/info toasts use status role (polite, not assertive)', async () => {
    let root!: ReturnType<typeof create>;
    try {
      await act(async () => {
        root = create(
          <ToastProvider>
            <Trigger type="success" message="ok" />
          </ToastProvider>,
        );
      });
      const trigger = root.root.findByType('button');
      await act(async () => {
        trigger.props.onClick();
      });
      const toast = root.root.find((node) => (
        typeof node.props?.className === 'string' && node.props.className.includes('toast-success')
      ));
      expect(toast.props.role).toBe('status');
    } finally {
      root?.unmount();
    }
  });
});
