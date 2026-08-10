import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

/**
 * i18n text-node gate (2026-08-02 sweep).
 *
 * Raw han Chinese inside a JSX text node (`>中文<`) renders verbatim in EN
 * mode. The sweep wrapped 414 such nodes in tr('...'); this gate keeps the
 * file that way: every SHORT han text node (<= 8 chars — labels, headers,
 * buttons) must already be wrapped. Longer sentences are exempt because they
 * are usually prose and need human translation, not a mechanical wrap.
 */
const WEB_ROOT = dirname(fileURLToPath(import.meta.url));
const SCAN_DIRS = ['pages', 'components'];
const TEXT_NODE = />\s*([一-龥][一-龥0-9\s（）：·，、%$:：]*[一-龥0-9%$]?)\s*</;
const MAX_LEN = 8;
// DesignSystemGallery documents raw design tokens and intentionally shows
// untranslated example content; brand names are English.
const EXCLUDED_FILES = new Set(['DesignSystemGallery.tsx']);

function collectUnwrapped(): string[] {
  const offenders: string[] = [];
  for (const dir of SCAN_DIRS) {
    const dirPath = join(WEB_ROOT, dir);
    let entries: string[];
    try {
      entries = readdirSync(dirPath);
    } catch {
      continue;
    }
    for (const f of entries.filter((f) => f.endsWith('.tsx') && !f.endsWith('.test.tsx'))) {
      if (EXCLUDED_FILES.has(f)) continue;
      const src = readFileSync(join(dirPath, f), 'utf-8');
      const re = new RegExp(TEXT_NODE.source, 'g');
      for (const m of src.matchAll(re)) {
        const text = m[1].trim();
        if (text.length > MAX_LEN) continue;
        if (src.includes(`tr('${text}')`)) continue;
        offenders.push(`${dir}/${f}: "${text}"`);
      }
    }
  }
  return offenders;
}

describe('i18n text-node gate', () => {
  it('every short han JSX text node is wrapped in tr()', () => {
    const offenders = collectUnwrapped();
    expect(offenders).toEqual([]);
  });
});
