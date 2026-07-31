import { describe, expect, it } from 'vitest';
import { collectTags, encodeTags, parseTags, tagColor } from './tags.js';

describe('parseTags', () => {
  it('parses JSON array text from the backend', () => {
    expect(parseTags('["prod","priority"]')).toEqual(['prod', 'priority']);
  });

  it('tolerates null / empty / already-array values', () => {
    expect(parseTags(null)).toEqual([]);
    expect(parseTags('')).toEqual([]);
    expect(parseTags(undefined)).toEqual([]);
    expect(parseTags(['a', ' b '])).toEqual(['a', ' b ']);
  });

  it('falls back to comma-separated for non-JSON text', () => {
    expect(parseTags('prod, priority,prod')).toEqual(['prod', 'priority', 'prod']);
  });
});

describe('tagColor', () => {
  it('is deterministic per tag and always a chart token', () => {
    expect(tagColor('prod')).toBe(tagColor('prod'));
    expect(tagColor('hk3')).toBe(tagColor('hk3'));
    expect(tagColor('prod')).toMatch(/^var\(--color-chart-\d+\)$/);
    expect(tagColor('a')).not.toBe(tagColor('b'));
  });
});

describe('collectTags', () => {
  it('unions tags across rows, most frequent first', () => {
    const rows = [
      { tags: '["prod","hk3"]' },
      { tags: '["prod"]' },
      { tags: null },
      { tags: '["backup"]' },
    ];
    expect(collectTags(rows)).toEqual(['prod', 'backup', 'hk3']);
  });
});

describe('encodeTags', () => {
  it('dedupes, trims and serializes to JSON array text', () => {
    expect(encodeTags(['prod', ' prod ', 'hk3'])).toBe('["prod","hk3"]');
    expect(encodeTags([])).toBe('[]');
  });
});
