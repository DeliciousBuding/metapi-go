import { describe, expect, it } from 'vitest';
import {
  DENSITY_STORAGE_KEY,
  THEME_MODE_KEY,
  LEGACY_THEME_KEY,
  THEME_ACCENT_KEY,
  canvasBackgroundForTheme,
  resolveInitialAccent,
  resolveInitialDataTheme,
  resolveInitialDensity,
} from './themeBootstrap.js';

function storageFrom(map: Record<string, string | null | undefined>) {
  return (key: string): string | null => {
    if (!(key in map)) return null;
    const value = map[key];
    return value == null ? null : value;
  };
}

describe('themeBootstrap', () => {
  describe('resolveInitialDataTheme', () => {
    it('uses theme_mode=dark regardless of system preference', () => {
      const getItem = storageFrom({ [THEME_MODE_KEY]: 'dark' });
      expect(resolveInitialDataTheme(getItem, false)).toBe('dark');
      expect(resolveInitialDataTheme(getItem, true)).toBe('dark');
    });

    it('uses theme_mode=light regardless of system preference', () => {
      const getItem = storageFrom({ [THEME_MODE_KEY]: 'light' });
      expect(resolveInitialDataTheme(getItem, true)).toBe('light');
      expect(resolveInitialDataTheme(getItem, false)).toBe('light');
    });

    it('resolves theme_mode=system from prefersDark', () => {
      const getItem = storageFrom({ [THEME_MODE_KEY]: 'system' });
      expect(resolveInitialDataTheme(getItem, true)).toBe('dark');
      expect(resolveInitialDataTheme(getItem, false)).toBe('light');
    });

    it('ignores legacy theme when theme_mode=system', () => {
      const getItem = storageFrom({
        [THEME_MODE_KEY]: 'system',
        [LEGACY_THEME_KEY]: 'light',
      });
      expect(resolveInitialDataTheme(getItem, true)).toBe('dark');
    });

    it('falls back to legacy theme when theme_mode is missing', () => {
      expect(
        resolveInitialDataTheme(storageFrom({ [LEGACY_THEME_KEY]: 'dark' }), false),
      ).toBe('dark');
      expect(
        resolveInitialDataTheme(storageFrom({ [LEGACY_THEME_KEY]: 'light' }), true),
      ).toBe('light');
    });

    it('falls back to prefersDark when neither key is set', () => {
      const getItem = storageFrom({});
      expect(resolveInitialDataTheme(getItem, true)).toBe('dark');
      expect(resolveInitialDataTheme(getItem, false)).toBe('light');
    });

    it('treats invalid theme_mode as missing and uses legacy', () => {
      const getItem = storageFrom({
        [THEME_MODE_KEY]: 'auto',
        [LEGACY_THEME_KEY]: 'dark',
      });
      expect(resolveInitialDataTheme(getItem, false)).toBe('dark');
    });

    it('treats invalid theme_mode and legacy as system preference', () => {
      const getItem = storageFrom({
        [THEME_MODE_KEY]: 'nope',
        [LEGACY_THEME_KEY]: 'nope',
      });
      expect(resolveInitialDataTheme(getItem, true)).toBe('dark');
      expect(resolveInitialDataTheme(getItem, false)).toBe('light');
    });
  });

  describe('canvasBackgroundForTheme', () => {
    it('returns design canvas colors for light and dark', () => {
      expect(canvasBackgroundForTheme('light')).toBe('#f8f9fa');
      expect(canvasBackgroundForTheme('dark')).toBe('#202124');
    });
  });

  describe('resolveInitialAccent ', () => {
    it('returns stored preset', () => {
      const getItem = storageFrom({ [THEME_ACCENT_KEY]: 'indigo' });
      expect(resolveInitialAccent(getItem)).toBe('indigo');
      expect(resolveInitialAccent(storageFrom({ [THEME_ACCENT_KEY]: 'teal' }))).toBe('teal');
    });

    it('falls back to blue for missing or unknown values', () => {
      expect(resolveInitialAccent(storageFrom({}))).toBe('blue');
      expect(resolveInitialAccent(storageFrom({ [THEME_ACCENT_KEY]: 'pink' }))).toBe('blue');
    });
  });

  describe('resolveInitialDensity ', () => {
    it('returns stored mode', () => {
      expect(resolveInitialDensity(storageFrom({ [DENSITY_STORAGE_KEY]: 'compact' }))).toBe('compact');
      expect(resolveInitialDensity(storageFrom({ [DENSITY_STORAGE_KEY]: 'comfortable' }))).toBe('comfortable');
    });

    it('falls back to comfortable for missing or unknown values', () => {
      expect(resolveInitialDensity(storageFrom({}))).toBe('comfortable');
      expect(resolveInitialDensity(storageFrom({ [DENSITY_STORAGE_KEY]: 'ultra' }))).toBe('comfortable');
    });
  });
});
