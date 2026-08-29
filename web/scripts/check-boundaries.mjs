#!/usr/bin/env node
// metapi-go/web — package boundary gate (S5).
//
// Mechanically enforces the import rules of
// docs/internal/web-package-boundaries.md:
//
//   rule 1: src/lib/ never imports from features/ or routes/.
//   rule 2: src/components/ never imports from features/ or routes/.
//
// Layers are classified by the first path segment under src/. Imports may
// point downward through the layer table; the edges above are the two hard
// upward edges the gate closes today.
//
// Not enforced (documented residuals in the boundaries doc — widening the
// gate needs its own issue): lib → components/i18n (lib/router.ts fallback
// pages, http-client/assert-business-ok i18n), hooks/use-sidebar-* →
// components/layout, features/__tests__ → routes (test-only).
//
// Run: bun run check:boundaries (chained into `bun run lint`, so pre-push
// and CI frontend jobs both execute it).
//
// Exceptions: EXCEPTIONS below — explicit registry of grandfathered
// cross-layer edges. Every entry must carry a reason AND match a real import
// in the tree; stale entries fail the gate (no speculative whitelisting).

import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const WEB_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const SRC = join(WEB_ROOT, 'src')
const DOC = 'docs/internal/web-package-boundaries.md'

// --- exceptions registry -----------------------------------------------------
const EXCEPTIONS = []

// --- layer rules ----------------------------------------------------------------
// layer of the importing file -> layers it must not import from.
const FORBIDDEN = {
  components: new Set(['features', 'routes']),
  lib: new Set(['features', 'routes']),
}

// --- scanning ----------------------------------------------------------------------
// Static + dynamic import specifiers. oxfmt keeps every import/export `from`
// clause on its own line, so line-based extraction is sound.
const FROM_RES = /\bfrom\s+['"]([^'"]+)['"]/g
const SIDE_EFFECT_RES = /^\s*import\s+['"]([^'"]+)['"]/
const DYNAMIC_RES = /\bimport\(\s*['"]([^'"]+)['"]\s*\)/g

function walk(dir, out) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      walk(full, out)
      continue
    }
    if (entry.endsWith('.ts') || entry.endsWith('.tsx')) out.push(full)
  }
  return out
}

/** Resolve an import specifier to a src-relative posix path, or null. */
function resolveSpecifier(specifier, fileDirRel) {
  let candidate = null
  if (specifier.startsWith('@/')) {
    candidate = specifier.slice(2)
  } else if (specifier.startsWith('./') || specifier.startsWith('../')) {
    candidate = `${fileDirRel}/${specifier}`
  }
  if (candidate === null) return null
  const segments = []
  for (const segment of candidate.split('/')) {
    if (segment === '.' || segment === '') continue
    if (segment === '..') {
      if (segments.length === 0) return null // escapes src/
      segments.pop()
      continue
    }
    segments.push(segment)
  }
  return segments.join('/')
}

const files = walk(SRC, [])
const violations = []
const matchedExceptions = new Set()
let checkedEdges = 0

function checkEdge(fileAbsRel, lineNumber, specifier, fileDirRel) {
  const sourceLayer = fileAbsRel.split('/')[0]
  const forbidden = FORBIDDEN[sourceLayer]
  if (forbidden === undefined) return
  const resolved = resolveSpecifier(specifier, fileDirRel)
  if (resolved === null) return
  const targetLayer = resolved.split('/')[0]
  if (targetLayer === sourceLayer || !forbidden.has(targetLayer)) return
  checkedEdges += 1
  const exceptionIndex = EXCEPTIONS.findIndex(
    (entry) =>
      entry.file === `src/${fileAbsRel}` && entry.specifier === specifier
  )
  if (exceptionIndex >= 0) {
    matchedExceptions.add(exceptionIndex)
    return
  }
  violations.push({
    location: `src/${fileAbsRel}:${lineNumber}`,
    sourceLayer,
    targetLayer,
    specifier,
  })
}

for (const file of files) {
  const fileAbsRel = file.slice(SRC.length + 1).replace(/\\/g, '/')
  const fileDirRel = fileAbsRel.includes('/')
    ? fileAbsRel.slice(0, fileAbsRel.lastIndexOf('/'))
    : ''
  const lines = readFileSync(file, 'utf8').split('\n')
  lines.forEach((line, index) => {
    const lineNumber = index + 1
    FROM_RES.lastIndex = 0
    let match
    while ((match = FROM_RES.exec(line)) !== null) {
      checkEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
    }
    DYNAMIC_RES.lastIndex = 0
    while ((match = DYNAMIC_RES.exec(line)) !== null) {
      checkEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
    }
    match = SIDE_EFFECT_RES.exec(line)
    if (match !== null) checkEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
  })
}

// --- verdict --------------------------------------------------------------------------
let failed = false

for (const violation of violations) {
  failed = true
  console.error(
    `✗ boundary violation: ${violation.location}\n` +
      `    imports '${violation.specifier}' — ${violation.sourceLayer} ↛ ${violation.targetLayer}\n` +
      `    rule: ${DOC}; if the edge is legitimate, register it in\n` +
      `    web/scripts/check-boundaries.mjs EXCEPTIONS with a reason.`
  )
}

EXCEPTIONS.forEach((entry, index) => {
  if (!matchedExceptions.has(index)) {
    failed = true
    console.error(
      `✗ stale boundary exception: ${entry.file} → '${entry.specifier}'\n` +
        `    no matching import exists anymore — delete the EXCEPTIONS entry\n` +
        `    (reason was: ${entry.reason})`
    )
  }
})

if (failed) process.exit(1)

console.log(
  `✓ package boundaries clean: ${files.length} files scanned, ` +
    `${checkedEdges} cross-layer edge(s) checked, ` +
    `${EXCEPTIONS.length} registered exception(s) all matched`
)
