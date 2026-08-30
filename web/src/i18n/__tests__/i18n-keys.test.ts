// metapi-go/i18n — key coverage test.
// Scans every t() / i18n.t() call in web/src and verifies each key exists in
// BOTH en.json and zh-CN.json (translation namespace). Replaces the retired
// MutationObserver-dictionary coverage test from the pre-rewrite i18n stack.

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

type TranslationNode = Record<string, unknown>

interface Usage {
  rawKey: string
  location: string
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

function extractUsages(): Usage[] {
  const usages: Usage[] = []
  for (const filePath of collectSourceFiles()) {
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
        })
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
  const usages = extractUsages()

  it('scans a meaningful number of t() call sites (no vacuous pass)', () => {
    expect(usages.length).toBeGreaterThan(1000)
  })

  it('defines every t() key in both en.json and zh-CN.json', () => {
    const allowlisted = new Set(DYNAMIC_KEY_ALLOWLIST)
    const missingInEn: string[] = []
    const missingInZh: string[] = []
    const seen = new Set<string>()

    for (const { rawKey, location } of usages) {
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
        missingInEn.push(`${key}  (${location})`)
      }
      if (!covered(zhRoot, zhKeys)) {
        missingInZh.push(`${key}  (${location})`)
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
