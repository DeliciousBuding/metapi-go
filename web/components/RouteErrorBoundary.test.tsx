import { afterAll, afterEach, describe, expect, it, vi } from 'vitest';
import { act, create } from 'react-test-renderer';
import { MemoryRouter } from 'react-router-dom';
import { RouteErrorBoundary } from './RouteErrorBoundary.js';

function Boom({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) throw new Error('boom-render');
  return <div>ok</div>;
}

// Silence React's console.error noise from the intentionally-throwing render.
const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

describe('RouteErrorBoundary', () => {
  afterEach(() => {
    consoleErrorSpy.mockClear();
  });

  afterAll(() => {
    consoleErrorSpy.mockRestore();
  });

  it('renders fallback on render error and stays stable on retry', async () => {
    let root!: ReturnType<typeof create>;
    try {
      await act(async () => {
        root = create(
          <MemoryRouter initialEntries={['/']}>
            <RouteErrorBoundary>
              <Boom shouldThrow />
            </RouteErrorBoundary>
          </MemoryRouter>,
        );
      });

      const rendered = JSON.stringify(root.toJSON());
      expect(rendered).toContain('页面渲染出错');
      expect(rendered).toContain('boom-render');
      expect(rendered).toContain('重试');

      const retry = root.root.find((node) => (
        node.type === 'button' && typeof node.props?.className === 'string'
        && node.props.className.includes('btn-primary')
      ));
      await act(async () => {
        retry.props.onClick();
      });
      // Boundary cleared; child still throws on next render, so fallback
      // remains — retry just re-runs the throwing render. Assert it stays
      // in the error view (no crash, no blank screen).
      expect(JSON.stringify(root.toJSON())).toContain('页面渲染出错');
    } finally {
      root?.unmount();
    }
  });

  it('renders children when no error', async () => {
    let root!: ReturnType<typeof create>;
    try {
      await act(async () => {
        root = create(
          <MemoryRouter initialEntries={['/']}>
            <RouteErrorBoundary>
              <Boom shouldThrow={false} />
            </RouteErrorBoundary>
          </MemoryRouter>,
        );
      });
      expect(JSON.stringify(root.toJSON())).toContain('ok');
    } finally {
      root?.unmount();
    }
  });
});
