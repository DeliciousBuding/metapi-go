import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { translateText } from './i18n.js';

/**
 * i18n coverage gate: every `t('…中文…')` literal in the app must translate
 * to English without Chinese residue or the `Untranslated` fallback in strict
 * English mode. Prevents new waves from shipping untranslated UI copy.
 */

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (['node_modules', 'dist', '.git'].includes(entry)) continue;
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) {
      walk(p, out);
    } else if (/\.tsx?$/.test(entry) && !/\.test\./.test(entry)) {
      out.push(p);
    }
  }
  return out;
}

function collectTLiterals(): string[] {
  const literals = new Set<string>();
  const re = /\bt\(\s*'([^']*[㐀-鿿][^']*)'\s*\)/g;
  for (const file of walk('.')) {
    const src = readFileSync(file, 'utf8');
    let m: RegExpExecArray | null;
    while ((m = re.exec(src))) literals.add(m[1]);
  }
  return [...literals];
}

describe('i18n coverage gate', () => {
  it('every t() Chinese literal has a usable English translation', () => {
    const bad: Array<[string, string]> = [];
    for (const literal of collectTLiterals()) {
      const out = translateText(literal, 'en');
      if (out === 'Untranslated' || /[㐀-鿿]/.test(out)) {
        bad.push([literal, out]);
      }
    }
    expect(bad).toEqual([]);
  });
});
