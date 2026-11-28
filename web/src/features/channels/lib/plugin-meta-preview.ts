import { z } from 'zod'

import {
  providerManifest,
  wireManifest,
  type ChannelPluginManifest,
} from '@/lib/api/channel-plugins'

import { parseStaticObject } from './static-js-literal-parser'

const manifestSchema = z.union([wireManifest, providerManifest])

export type PluginMetaPreviewResult =
  | { status: 'empty' }
  | { status: 'not_found' }
  | { status: 'dynamic'; reason: string }
  | {
      status: 'invalid_schema'
      rawMeta: Record<string, unknown>
      issues: string[]
    }
  | {
      status: 'valid'
      manifest: ChannelPluginManifest
      keyMismatch: boolean
    }

const EXPORT_META_RE = /export\s+const\s+meta\s*=/y

/**
 * Finds the start index right after `export const meta =` while ignoring
 * comments and string literals outside top-level declarations.
 */
function findExportMetaEqualsIndex(source: string): number {
  let i = 0
  while (i < source.length) {
    const ch = source[i]
    if (ch === '/' && source[i + 1] === '/') {
      i += 2
      while (i < source.length && source[i] !== '\n') i += 1
      continue
    }
    if (ch === '/' && source[i + 1] === '*') {
      const end = source.indexOf('*/', i + 2)
      if (end === -1) return -1
      i = end + 2
      continue
    }
    if (ch === '"' || ch === "'" || ch === '`') {
      const quote = ch
      i += 1
      while (i < source.length) {
        if (source[i] === '\\') {
          i += 2
          continue
        }
        if (source[i] === quote) {
          i += 1
          break
        }
        i += 1
      }
      continue
    }
    if (ch === 'e') {
      EXPORT_META_RE.lastIndex = i
      const match = EXPORT_META_RE.exec(source)
      if (match) {
        const prev = i > 0 ? source[i - 1] : ''
        if (!/[A-Za-z0-9_$]/.test(prev)) {
          return i + match[0].length
        }
      }
    }
    i += 1
  }
  return -1
}

export function extractStaticPluginMetaPreview(
  source: string,
  expectedPluginKey?: string
): PluginMetaPreviewResult {
  if (!source.trim()) {
    return { status: 'empty' }
  }

  const startIndex = findExportMetaEqualsIndex(source)
  if (startIndex === -1) {
    return { status: 'not_found' }
  }

  let rawMeta: Record<string, unknown>
  try {
    rawMeta = parseStaticObject(source, startIndex)
  } catch (error) {
    return {
      status: 'dynamic',
      reason:
        error instanceof Error
          ? error.message
          : 'Dynamic syntax is not supported in static preview',
    }
  }

  const parsed = manifestSchema.safeParse(rawMeta)
  if (!parsed.success) {
    const issues = parsed.error.issues.map((issue) => {
      const path = issue.path.length > 0 ? `${issue.path.join('.')}: ` : ''
      return `${path}${issue.message}`
    })
    return {
      status: 'invalid_schema',
      rawMeta,
      issues: issues.slice(0, 6),
    }
  }

  const manifest = parsed.data
  const keyMismatch = Boolean(
    expectedPluginKey &&
    expectedPluginKey.trim() &&
    manifest.key !== expectedPluginKey.trim()
  )

  return {
    status: 'valid',
    manifest,
    keyMismatch,
  }
}
