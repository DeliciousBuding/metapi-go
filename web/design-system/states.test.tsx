import { describe, expect, it, vi } from 'vitest';
import { act, create } from 'react-test-renderer';
import { ErrorState, LoadingState } from './index.js';

describe('design-system states', () => {
  it('ErrorState renders alert role + retry that fires onRetry', async () => {
    const onRetry = vi.fn();
    let root!: ReturnType<typeof create>;
    try {
      await act(async () => {
        root = create(
          <ErrorState title="加载失败" description="上游不可达" onRetry={onRetry} retryLabel="重试" />,
        );
      });
      const text = JSON.stringify(root.toJSON());
      expect(text).toContain('加载失败');
      expect(text).toContain('上游不可达');
      expect(text).toContain('重试');

      // EmptyState tone=danger => role=alert
      const well = root.root.find((node) => (
        typeof node.props?.className === 'string' && node.props.className.includes('ds-empty--danger')
      ));
      expect(well.props.role).toBe('alert');

      const retry = root.root.find((node) => node.type === 'button');
      await act(async () => {
        retry.props.onClick();
      });
      expect(onRetry).toHaveBeenCalledTimes(1);
    } finally {
      root?.unmount();
    }
  });

  it('ErrorState without onRetry renders no button', async () => {
    let root!: ReturnType<typeof create>;
    try {
      await act(async () => {
        root = create(<ErrorState title="无重试" />);
      });
      const buttons = root.root.findAll((node) => node.type === 'button');
      expect(buttons.length).toBe(0);
    } finally {
      root?.unmount();
    }
  });

  it('LoadingState block renders N skeleton lines with polite live region', async () => {
    let root!: ReturnType<typeof create>;
    try {
      await act(async () => {
        root = create(<LoadingState lines={4} label="正在加载日志" />);
      });
      const region = root.root.find((node) => (
        typeof node.props?.className === 'string' && node.props.className.includes('ds-loading')
      ));
      expect(region.props.role).toBe('status');
      expect(region.props['aria-live']).toBe('polite');
      expect(region.props['aria-label']).toBe('正在加载日志');
      const lines = root.root.findAll((node) => (
        typeof node.props?.className === 'string' && node.props.className.includes('ds-loading__line')
      ));
      expect(lines.length).toBe(4);
    } finally {
      root?.unmount();
    }
  });

  it('LoadingState inline renders a spinner + label', async () => {
    let root!: ReturnType<typeof create>;
    try {
      await act(async () => {
        root = create(<LoadingState inline label="同步中" />);
      });
      const inline = root.root.find((node) => (
        typeof node.props?.className === 'string' && node.props.className.includes('ds-loading-inline')
      ));
      expect(inline.props.role).toBe('status');
      expect(inline.props['aria-live']).toBe('polite');
      const text = JSON.stringify(root.toJSON());
      expect(text).toContain('同步中');
      expect(text).toContain('spinner');
    } finally {
      root?.unmount();
    }
  });
});
