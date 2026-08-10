import { describe, it, expect, afterEach } from 'vitest';
import { prefersReducedMotion } from './motion.js';

function stubMatchMedia(matches: boolean) {
  globalThis.matchMedia = ((query: string) => ({
    matches: query === '(prefers-reduced-motion: reduce)' ? matches : false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    onchange: null,
    dispatchEvent: () => false,
  })) as typeof globalThis.matchMedia;
}

const originalMatchMedia = globalThis.matchMedia;

afterEach(() => {
  if (originalMatchMedia) {
    globalThis.matchMedia = originalMatchMedia;
  } else {
    Reflect.deleteProperty(globalThis, 'matchMedia');
  }
});

describe('prefersReducedMotion', () => {
  it('returns true when the OS prefers reduced motion', () => {
    stubMatchMedia(true);
    expect(prefersReducedMotion()).toBe(true);
  });

  it('returns false when motion is allowed', () => {
    stubMatchMedia(false);
    expect(prefersReducedMotion()).toBe(false);
  });

  it('returns false when matchMedia is unavailable', () => {
    Reflect.deleteProperty(globalThis, 'matchMedia');
    expect(prefersReducedMotion()).toBe(false);
  });
});
