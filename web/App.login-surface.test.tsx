import { describe, expect, it, vi } from 'vitest';
import { create, type ReactTestInstance } from 'react-test-renderer';
import { Login } from './App.js';
import { SITE_DOCS_URL, SITE_GITHUB_URL } from './docsLink.js';

function collectText(node: ReactTestInstance): string {
  return (node.children || []).map((child) => {
    if (typeof child === 'string') return child;
    return collectText(child);
  }).join('');
}

describe('Login surface', () => {
  it('uses the site root as the documentation URL', () => {
    expect(SITE_DOCS_URL).toBe('https://github.com/DeliciousBuding/metapi-go#readme');
  });

  it('uses the author github profile for the login github shortcut', () => {
    expect(SITE_GITHUB_URL).toBe('https://github.com/DeliciousBuding/metapi-go');
  });

  it('renders a single centered card with brand head and admin token form', () => {
    const root = create(
      <Login onLogin={vi.fn()} t={(text) => text} />,
    );

    try {
      const pageText = collectText(root.root);

      // Single centered card (no poster-style side panel anymore).
      const surface = root.root.find((node) => (
        node.type === 'div'
        && typeof node.props.className === 'string'
        && node.props.className.includes('login-surface')
      ));
      expect(surface).toBeTruthy();
      expect(root.root.findAll((node) => (
        node.type === 'section'
        && typeof node.props.className === 'string'
        && (node.props.className.includes('login-brand-panel')
          || node.props.className.includes('login-auth-stage'))
      ))).toHaveLength(0);

      // Brand head collapsed into the card: logo + name + kicker.
      const brandHead = root.root.find((node) => (
        node.type === 'div'
        && typeof node.props.className === 'string'
        && node.props.className.includes('login-brand-head')
      ));
      const brandMarkCanvas = root.root.find((node) => (
        node.type === 'div'
        && typeof node.props.className === 'string'
        && node.props.className.includes('brand-mark-canvas')
      ));
      expect(brandHead).toBeTruthy();
      expect(brandMarkCanvas).toBeTruthy();
      expect(pageText).toContain('Metapi');
      expect(pageText).toContain('中转站的中转站');

      // Value props kept, capability list removed with the side panel.
      expect(pageText).toContain('兼容 New API / One API / OneHub / DoneHub / Veloera / AnyRouter / Sub2API');
      expect(pageText).not.toContain('统一代理网关');
      expect(pageText).not.toContain('智能路由引擎');
      expect(pageText).not.toContain('自动模型发现');

      const docsLink = root.root.find((node) => (
        node.type === 'a'
        && node.props.href === SITE_DOCS_URL
      ));
      const tokenInput = root.root.find((node) => (
        node.type === 'input'
        && node.props.placeholder === '管理员令牌'
      ));
      const githubLink = root.root.find((node) => (
        node.type === 'a'
        && node.props.href === SITE_GITHUB_URL
      ));

      expect(docsLink.props.target).toBe('_blank');
      expect(githubLink.props['aria-label']).toBe('GitHub');
      expect(githubLink.props.target).toBe('_blank');
      expect(tokenInput.props.type).toBe('password');
    } finally {
      root?.unmount();
    }
  });
});
