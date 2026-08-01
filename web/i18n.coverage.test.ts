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
  // Line comments: require a line start or preceding whitespace so URLs like
  // `https://api.example.com` inside string literals are NOT truncated at `//`.
  return src
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\/\/[^\n]*|\s\/\/[^\n]*/gm, '');
}

/**
 * t('…') / tr('…') literals (wrapped — must translate cleanly).
 * tr() was a blind spot of the gate: canvas/button copy that only used tr()
 * shipped as `Untranslated` (SnapshotExportButton); both call sites are now
 * scanned.
 */
function collectTLiterals(): string[] {
  const literals = new Set<string>();
  const re = /\b(?:t|tr)\(\s*'([^']*[㐀-鿿][^']*)'\s*\)/g;
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
    if (file.includes('i18n') || file.includes('charts')) continue;
    const src = stripComments(readFileSync(file, 'utf8'));
    const attrRe = /\b(placeholder|title|aria-label|alt|description)="([^"]*[㐀-鿿][^"]*)"/g;
    let m: RegExpExecArray | null;
    while ((m = attrRe.exec(src))) {
      const before = src.slice(Math.max(0, m.index - 40), m.index);
      if (/\b(?:t|tr)\(\s*$/.test(before)) continue;
      literals.add(m[2]);
    }
    // Expression-bound attributes (`placeholder={cond ? '中文' : '…'}`) —
    // not matched by the `attr="…"` form; extract any bare Chinese string
    // literals inside them (t()/tr()-wrapped ones are covered above).
    const exprRe = /\b(placeholder|title|aria-label)=\{([^}]*[㐀-鿿][^}]*)\}/g;
    while ((m = exprRe.exec(src))) {
      const expr = m[2];
      if (/\b(?:t|tr)\(\s*'/.test(expr)) continue;
      const litRe = /'([^']*[㐀-鿿][^']*)'/g;
      let lm: RegExpExecArray | null;
      while ((lm = litRe.exec(expr))) literals.add(lm[1]);
    }
    // Object-literal values (`label: '站点公告'`, `site_notice: '站点公告'`
    // in nav configs / option maps / status maps). These render as DOM text
    // and are translated by the observer — e2e found 200+ keys this scanner
    // had been missing entirely (2026-08-01).
    const objRe = /:\s*'([^']*[㐀-鿿][^']*)'/g;
    while ((m = objRe.exec(src))) literals.add(m[1]);
    const textRe = />([^<>{}]*[㐀-鿿][^<>{}]*)<\//g;
    while ((m = textRe.exec(src))) {
      const lit = m[1].trim();
      if (lit) literals.add(lit);
    }
  }
  return [...literals];
}

/**
 * JSX text nodes that contain interpolation (`{expr}`) — React splits these
 * into separate text-node fragments that the runtime DOM translator handles
 * individually; the dictionary must cover the Chinese fragments.
 */
function collectInterpolatedJSX(): string[] {
  const literals = new Set<string>();
  for (const file of walk('.')) {
    const src = stripComments(readFileSync(file, 'utf8'));
    const re = />([^<]*[㐀-鿿][^<]*)<\//g;
    let m: RegExpExecArray | null;
    while ((m = re.exec(src))) {
      const lit = m[1].trim();
      if (!lit.includes('{')) continue;
      literals.add(lit);
    }
  }
  return [...literals];
}

/**
 * VChart/canvas spec literals — these can render to canvas, out of reach of
 * the MutationObserver:
 * - `key: '中文'` (tooltip keys) MUST be wrapped in tr() — raw keys are an
 *   error (canvas shows Chinese otherwise).
 * - `type|label|metric|title` may render to DOM buttons the observer can
 *   translate, so they are only required to be dictionary-covered.
 *
 * Scope is limited to web/components/charts/ — object literals elsewhere
 * (e.g. nav menu `label: '中文'`) render as DOM and are covered by the raw
 * JSX gate instead.
 */
function collectChartSpecLiterals(): { raw: string[]; covered: string[] } {
  const raw = new Set<string>();
  const covered = new Set<string>();
  const rawRe = /\bkey:\s*'([^']*[㐀-鿿][^']*)'/g;
  const coveredRe = /\b(?:key|type|label|metric|title):\s*(?:tr\(\s*)?'([^']*[㐀-鿿][^']*)'/g;
  for (const file of walk('components/charts')) {
    const src = stripComments(readFileSync(file, 'utf8'));
    let m: RegExpExecArray | null;
    while ((m = rawRe.exec(src))) raw.add(m[1]);
    while ((m = coveredRe.exec(src))) covered.add(m[1]);
  }
  return { raw: [...raw], covered: [...covered] };
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

  // Dict grew to ~1500 keys; each fragment runs the full phrase-replacement
  // pass, which is slow on CI runners — budget generously.
  it('interpolated JSX text fragments are covered (React text-node splits)', () => {
    const bad: Array<[string, string]> = [];
    for (const literal of collectInterpolatedJSX()) {
      // Runtime splits `标题（近 {days} 天）` into fragments; each non-expression
      // fragment is its own text node and must translate independently.
      const fragments = literal.split(/\{[^}]*\}/).filter((s) => s.trim());
      for (const fragment of fragments) {
        const out = translateText(fragment.trim(), 'en');
        if (out === 'Untranslated' || CJK_RE.test(out)) {
          bad.push([fragment, out]);
        }
      }
    }
    expect(bad).toEqual([]);
  }, 20_000);

  it('chart spec literals are wrapped in tr() and covered by the dictionary', () => {
    const { raw, covered } = collectChartSpecLiterals();
    // Raw tooltip keys render Chinese to canvas — the observer cannot reach them.
    expect(raw, 'chart spec key must be wrapped in tr(): ' + raw.join(', ')).toEqual([]);
    const bad: Array<[string, string]> = [];
    for (const literal of covered) {
      const out = translateText(literal, 'en');
      if (out === 'Untranslated' || CJK_RE.test(out)) {
        bad.push([literal, out]);
      }
    }
    expect(bad).toEqual([]);
  });
});
