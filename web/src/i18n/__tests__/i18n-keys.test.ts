// metapi-go/i18n — key coverage test.
// Scans every t() / i18n.t() call in web/src and verifies each key exists in
// BOTH en.json and zh-CN.json (translation namespace). Replaces the retired
// MutationObserver-dictionary coverage test from the pre-rewrite i18n stack.
//
// A `t('literal')` scan alone is not enough: two channels hand i18n keys to
// the translator INDIRECTLY, so the literal never sits inside a `t(` call —
//   1. `assertBusinessOk(result, 'feature.area.key')` — the shared envelope
//      guard renders its fallback through `i18n.t(fallbackI18nKey)`
//      (lib/assert-business-ok.ts);
//   2. modules that RETURN a key for a component to translate, e.g. the
//      model-tester error resolver (`return 'modelTester.error.notAvailable'`)
//      whose value the page feeds to `t(rawMessage)`.
// Both channels are covered below by collecting every locale-rooted,
// key-shaped string literal in the source tree.

import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import en from '../locales/en.json'
import zhCN from '../locales/zh-CN.json'

const SRC_ROOT = join(import.meta.dirname, '..', '..')
const SKIP_DIRS = new Set(['__tests__', 'node_modules', 'dist'])

// Matches `t('…')`, `t("…")`, `t(`…`)` and the `i18n.t(…)` / `i18next.t(…)`
// variants. The lookbehind excludes e.g. `get(`/`rotate(`/`toast.error(`.
const T_CALL_RE = /(?<![A-Za-z0-9_$])(?:i18n\.)?t\s*\(\s*(['"`])/g

// String literal matchers per quote char; double-quoted keys may contain
// escaped quotes, so the content regex must allow `\"`.
const STRING_RE = {
  "'": /'((?:[^'\\]|\\.)*)'/,
  '"': /"((?:[^"\\]|\\.)*)"/,
  '`': /`((?:[^`\\]|\\.)*)`/,
} as const

// --- indirect key channels -------------------------------------------------

/**
 * A string literal that looks like an i18n key: lowercase-led, dotted, with
 * identifier-shaped segments (`accounts.toast.pinFailed`). Deliberately strict
 * — no spaces, slashes, colons or leading digits — so URLs, MIME types,
 * CSS-ish tokens and `object.property` prose do not qualify.
 */
const KEY_SHAPED_RE = /^[a-z][A-Za-z0-9]*(?:\.[A-Za-z0-9_-]+)+$/

/**
 * Top-level locale namespaces (`accounts`, `modelTester`, `settings`, …).
 * A key-shaped literal only counts as an i18n key when its first segment is
 * one of these, which keeps the wide source scan free of false positives
 * (sentence-shaped top-level keys such as `No Data` can never match
 * KEY_SHAPED_RE's first segment anyway).
 */
const LOCALE_ROOTS = new Set(
  Object.keys(en.translation).filter((root) => /^[a-z][A-Za-z0-9]*$/.test(root))
)

/** Any single/double-quoted string literal (backticks are template literals). */
const ANY_STRING_RE = /(['"])((?:(?!\1)[^\\\n]|\\.)*)\1/g

/**
 * `assertBusinessOk(result, 'feature.area.key')` — the fallback key argument.
 * The argument list is matched without nested parens (every call site passes
 * plain identifiers + a literal), and `<…>` type arguments are skipped.
 */
const ASSERT_BUSINESS_OK_RE =
  /(?<![A-Za-z0-9_$])assertBusinessOk\s*(?:<[^<>]*>)?\s*\(([^()]*)\)/g

type TranslationNode = Record<string, unknown>

type UsageKind = 't-call' | 'assertBusinessOk-fallback' | 'key-literal'

interface Usage {
  rawKey: string
  location: string
  kind: UsageKind
}

/** Flatten a translation object into dotted leaf keys (`translation.` prefix excluded). */
function flattenLeaves(node: TranslationNode, prefix = ''): Set<string> {
  const leaves = new Set<string>()
  for (const [segment, value] of Object.entries(node)) {
    const key = prefix ? `${prefix}.${segment}` : segment
    if (value !== null && typeof value === 'object') {
      for (const leaf of flattenLeaves(value as TranslationNode, key)) {
        leaves.add(leaf)
      }
    } else {
      leaves.add(key)
    }
  }
  return leaves
}

/** Walk dotted segments and return the node if it exists (object or leaf). */
function resolveSegments(root: TranslationNode, dottedKey: string): unknown {
  let current: unknown = root
  for (const segment of dottedKey.split('.')) {
    if (current === null || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[segment]
  }
  return current
}

function collectSourceFiles(): string[] {
  const files: string[] = []
  const visit = (dir: string): void => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (entry.isDirectory()) {
        if (!SKIP_DIRS.has(entry.name)) visit(join(dir, entry.name))
      } else if (
        (entry.name.endsWith('.ts') || entry.name.endsWith('.tsx')) &&
        !/\.(test|spec)\.(ts|tsx)$/.test(entry.name)
      ) {
        files.push(join(dir, entry.name))
      }
    }
  }
  visit(SRC_ROOT)
  return files
}

/** Line number (1-based) of a character offset inside a whole-file string. */
function lineOfOffset(text: string, offset: number): number {
  let line = 1
  for (let index = 0; index < offset && index < text.length; index++) {
    if (text[index] === '\n') line++
  }
  return line
}

/** True when the literal sits in a comment (doc examples are not call sites). */
function isCommentContext(line: string, literalOffsetInLine: number): boolean {
  const trimmed = line.trimStart()
  if (
    trimmed.startsWith('*') ||
    trimmed.startsWith('//') ||
    trimmed.startsWith('/*')
  ) {
    return true
  }
  return line.slice(0, literalOffsetInLine).includes('//')
}

function extractTCallUsages(files: string[]): Usage[] {
  const usages: Usage[] = []
  for (const filePath of files) {
    const lines = readFileSync(filePath, 'utf8').split('\n')
    lines.forEach((line, index) => {
      for (const match of line.matchAll(T_CALL_RE)) {
        const beforeMatch = line.slice(0, match.index)
        // Skip doc examples inside comments (e.g. `// header: t('Name')`).
        const trimmed = line.trimStart()
        if (
          trimmed.startsWith('*') ||
          trimmed.startsWith('//') ||
          beforeMatch.includes('//')
        ) {
          continue
        }
        const quote = match[1] as keyof typeof STRING_RE
        const rest = line.slice(match.index + match[0].length - 1)
        const stringMatch = STRING_RE[quote].exec(rest)
        if (!stringMatch) continue
        // The key must be a standalone argument (`)` or `,` follows).
        const afterKey = rest.slice(stringMatch[0].length).trimStart()
        if (!afterKey.startsWith(')') && !afterKey.startsWith(',')) continue
        usages.push({
          rawKey: stringMatch[1],
          location: `${filePath.replaceAll('\\', '/')}:${index + 1}`,
          kind: 't-call',
        })
      }
    })
  }
  return usages
}

/**
 * Collect keys that reach the translator WITHOUT a `t('literal')` call site:
 * `assertBusinessOk(…, 'key')` fallbacks (tagged separately so the gate can
 * prove that channel is still scanned) plus every other locale-rooted,
 * key-shaped string literal in the tree — which is how returned/propagated
 * keys (model-tester error resolver) get caught.
 */
function extractIndirectUsages(files: string[]): Usage[] {
  const usages: Usage[] = []
  const seen = new Set<string>()
  const push = (rawKey: string, location: string, kind: UsageKind): void => {
    const id = `${location}::${rawKey}`
    if (seen.has(id)) return
    seen.add(id)
    usages.push({ rawKey, location, kind })
  }

  for (const filePath of files) {
    const relative = filePath.replaceAll('\\', '/')
    const text = readFileSync(filePath, 'utf8')
    const lines = text.split('\n')

    // Channel 1: assertBusinessOk fallback keys (may span several lines).
    for (const match of text.matchAll(ASSERT_BUSINESS_OK_RE)) {
      const startLine = lineOfOffset(text, match.index)
      const args = match[1] ?? ''
      const argsOffsetInLine = (lines[startLine - 1] ?? '').indexOf(
        'assertBusinessOk'
      )
      if (isCommentContext(lines[startLine - 1] ?? '', argsOffsetInLine)) {
        continue
      }
      for (const literal of args.matchAll(ANY_STRING_RE)) {
        const candidate = literal[2] ?? ''
        if (!KEY_SHAPED_RE.test(candidate)) continue
        if (!LOCALE_ROOTS.has(candidate.split('.')[0] ?? '')) continue
        // `args` starts on `startLine`, so the literal's own line is the
        // relative offset inside the argument list.
        const literalLine =
          lineOfOffset(args, literal.index ?? 0) + startLine - 1
        push(
          candidate,
          `${relative}:${literalLine}`,
          'assertBusinessOk-fallback'
        )
      }
    }

    // Channel 2: every locale-rooted key-shaped literal (returned keys,
    // constants handed to a component that calls `t(value)`).
    lines.forEach((line, index) => {
      for (const literal of line.matchAll(ANY_STRING_RE)) {
        if (isCommentContext(line, literal.index)) continue
        const candidate = literal[2] ?? ''
        if (!KEY_SHAPED_RE.test(candidate)) continue
        if (!LOCALE_ROOTS.has(candidate.split('.')[0] ?? '')) continue
        push(candidate, `${relative}:${index + 1}`, 'key-literal')
      }
    })
  }
  return usages
}

// Keys reachable only through runtime-computed variables that a static scan
// cannot resolve (e.g. `t(emptyText)` where emptyText is a component prop).
// Each entry is still asserted to exist in BOTH locales below.
// Currently empty: the sole entry ('No option found.') belonged to the
// removed ui/combobox-input.tsx default prop.
const DYNAMIC_KEY_ALLOWLIST: string[] = []

// Deliberate exceptions for the wide literal scan: strings that are shaped
// like `localeRoot.something` but are NOT i18n keys (e.g. an identifier used
// as a storage/event name). Entries are skipped by the coverage check; keep
// each one commented with the reason it is not a key.
// Currently empty — every locale-rooted literal in web/src is a real key.
const NON_KEY_LITERAL_ALLOWLIST: string[] = []

/**
 * Normalize a raw key into the form the locales are checked against:
 * - template literal `settings.x.${dynamic}.title` → static prefix node
 * - key with interpolation `key.with.{{var}}` → literal key, falling back to
 *   the dotted prefix without the interpolated segment
 */
function normalizeKey(rawKey: string, root: TranslationNode): string | null {
  if (rawKey.includes('${')) {
    const prefix = rawKey.split('${')[0]?.replace(/\.+$/, '')
    return prefix || null
  }
  if (rawKey.includes('{{')) {
    const literal = resolveSegments(root, rawKey)
    if (literal !== undefined) return rawKey
    const segments = rawKey.split('.').filter((s) => !s.includes('{{'))
    return segments.join('.') || null
  }
  return rawKey
}

describe('i18n key coverage', () => {
  const enRoot = en.translation as TranslationNode
  const zhRoot = zhCN.translation as TranslationNode
  const enKeys = flattenLeaves(enRoot)
  const zhKeys = flattenLeaves(zhRoot)
  const sourceFiles = collectSourceFiles()
  const tCallUsages = extractTCallUsages(sourceFiles)
  const indirectUsages = extractIndirectUsages(sourceFiles)
  const usages = [...tCallUsages, ...indirectUsages]

  it('scans a meaningful number of t() call sites (no vacuous pass)', () => {
    expect(tCallUsages.length).toBeGreaterThan(1000)
  })

  it('scans the indirect key channels (no vacuous pass)', () => {
    const fallbacks = indirectUsages.filter(
      (usage) => usage.kind === 'assertBusinessOk-fallback'
    )
    // The envelope guard is the single indirect entry point for mutation
    // failure copy — if this count collapses, the regex stopped matching and
    // every fallback key would silently escape the gate again.
    expect(fallbacks.length).toBeGreaterThanOrEqual(15)
    expect(indirectUsages.length).toBeGreaterThan(100)
    // Regression pin for channel 2: the model-tester resolver RETURNS its keys
    // (model-tester-page.tsx feeds them to `t(rawMessage)`), so a scan limited
    // to `t('…')` literals never sees them.
    expect(
      indirectUsages.some(
        (usage) =>
          usage.rawKey === 'modelTester.error.notAvailable' &&
          usage.kind === 'key-literal'
      ),
      'model-tester resolver keys must stay inside the scanned surface'
    ).toBe(true)
  })

  it('defines every referenced i18n key in both en.json and zh-CN.json', () => {
    const allowlisted = new Set([
      ...DYNAMIC_KEY_ALLOWLIST,
      ...NON_KEY_LITERAL_ALLOWLIST,
    ])
    const missingInEn: string[] = []
    const missingInZh: string[] = []
    const seen = new Set<string>()

    for (const { rawKey, location, kind } of usages) {
      if (allowlisted.has(rawKey)) continue
      const key = normalizeKey(rawKey, enRoot)
      if (key === null) {
        // A fully dynamic template prefix (e.g. `${x}.title`) cannot be
        // verified statically — surface it so it gets an allowlist entry.
        expect.fail(
          `unresolvable dynamic key ${JSON.stringify(rawKey)} at ${location}`
        )
      }
      if (seen.has(key)) continue
      seen.add(key)
      // Dynamic template bases are object nodes, plain keys are leaves —
      // accept either form in each locale. i18next plural calls write the
      // base key (`key`) but the locales define suffixed forms
      // (`key_one`/`key_other`), so accept those as coverage too.
      const covered = (root: TranslationNode, keys: Set<string>) =>
        keys.has(key) ||
        resolveSegments(root, key) !== undefined ||
        keys.has(`${key}_one`) ||
        keys.has(`${key}_other`)
      if (!covered(enRoot, enKeys)) {
        missingInEn.push(`${key}  (${kind} @ ${location})`)
      }
      if (!covered(zhRoot, zhKeys)) {
        missingInZh.push(`${key}  (${kind} @ ${location})`)
      }
    }

    expect(missingInEn, 'missing from en.json').toEqual([])
    expect(missingInZh, 'missing from zh-CN.json').toEqual([])
    expect(seen.size).toBeGreaterThan(900)
  })

  it('keeps allowlisted dynamic keys defined in both locales', () => {
    for (const key of DYNAMIC_KEY_ALLOWLIST) {
      expect(enKeys.has(key), `en.json missing allowlisted key ${key}`).toBe(
        true
      )
      expect(zhKeys.has(key), `zh-CN.json missing allowlisted key ${key}`).toBe(
        true
      )
    }
  })

  it('keeps en and zh-CN key sets identical (bidirectional zero missing)', () => {
    expect([...zhKeys].filter((k) => !enKeys.has(k))).toEqual([])
    expect([...enKeys].filter((k) => !zhKeys.has(k))).toEqual([])
  })

  // S10 bilingual CI: key-set parity alone cannot catch a translation that
  // drops an interpolation variable (e.g. zh rendering "余额不足：relay ()"
  // because {{amount}} was lost). Every leaf present in both locales must
  // declare the same placeholder set, so a dropped variable fails CI.
  it('keeps interpolation placeholders identical between en and zh-CN', () => {
    const PLACEHOLDER_RE = /\{\{\s*([^}]+?)\s*\}\}/g
    const placeholders = (value: string): string[] =>
      [...value.matchAll(PLACEHOLDER_RE)].map((m) => (m[1] ?? '').trim()).sort()
    const leafValue = (root: TranslationNode, key: string): string | null => {
      const value = resolveSegments(root, key)
      return typeof value === 'string' ? value : null
    }

    const mismatched: string[] = []
    for (const key of enKeys) {
      const enValue = leafValue(enRoot, key)
      const zhValue = leafValue(zhRoot, key)
      if (enValue === null || zhValue === null) continue
      const enVars = placeholders(enValue)
      const zhVars = placeholders(zhValue)
      if (enVars.join('') !== zhVars.join('')) {
        mismatched.push(`${key}  (en: ${enVars} | zh-CN: ${zhVars})`)
      }
    }
    expect(mismatched).toEqual([])
  })
})
