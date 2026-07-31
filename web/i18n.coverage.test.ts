import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { translateText } from './i18n.js';

/**
 * i18n coverage gate: every user-visible Chinese copy must have a usable
 * English translation in strict English mode — either wrapped in t() (checked
 * via literals) or covered by the zhToEn dictionary (checked via raw JSX).
 *
 * - t('…') literals: must translate without Chinese residue or the
 *   `Untranslated` fallback.
 * - Raw JSX (attributes + text nodes, not wrapped in t()): the runtime
 *   MutationObserver translates these via the dictionary — the gate asserts
 *   the dictionary actually covers them (no `Untranslated` / residue).
 *
 * Prevents new waves from shipping untranslated UI copy.
 */

const CJK_RE = /[㐀-鿿]/;

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

function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');
}

/** t('…中文…') literals (wrapped — must translate cleanly). */
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

/** Raw JSX-visible Chinese: attribute strings and text nodes (not wrapped). */
function collectRawJSX(): string[] {
  const literals = new Set<string>();
  for (const file of walk('.')) {
    const src = stripComments(readFileSync(file, 'utf8'));
    const attrRe = /\b(placeholder|title|aria-label|alt|description)="([^"]*[㐀-鿿][^"]*)"/g;
    let m: RegExpExecArray | null;
    while ((m = attrRe.exec(src))) {
      const before = src.slice(Math.max(0, m.index - 40), m.index);
      if (/\bt\(\s*$/.test(before)) continue;
      literals.add(m[2]);
    }
    const textRe = />([^<>{}]*[㐀-鿿][^<>{}]*)<\//g;
    while ((m = textRe.exec(src))) {
      const lit = m[1].trim();
      if (lit) literals.add(lit);
    }
  }
  return [...literals];
}

describe('i18n coverage gate', () => {
  it('every t() Chinese literal has a usable English translation', () => {
    const bad: Array<[string, string]> = [];
    for (const literal of collectTLiterals()) {
      const out = translateText(literal, 'en');
      if (out === 'Untranslated' || CJK_RE.test(out)) {
        bad.push([literal, out]);
      }
    }
    expect(bad).toEqual([]);
  });

  it('raw JSX Chinese is covered by the dictionary (runtime DOM translation)', () => {
    const bad: Array<[string, string]> = [];
    for (const literal of collectRawJSX()) {
      const out = translateText(literal, 'en');
      if (out === 'Untranslated' || CJK_RE.test(out)) {
        bad.push([literal, out]);
      }
    }
    expect(bad).toEqual([]);
  });
});
