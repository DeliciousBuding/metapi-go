#!/usr/bin/env node
// metapi-go/web — package boundary gate.
//
// Mechanically enforces the import rules of
// docs/internal/web-package-boundaries.md:
//
// Rule numbers below are the doc's (src/lib/*.ts cites them by number, so the
// doc is the authority and this script must not invent a second numbering):
//   rule 1: src/lib/ never imports from features/ or routes/.
//   rule 2: src/components/ never imports from features/ or routes/.
//   rule 3: a feature imports another feature through its barrel
//           (`@/features/<name>`) and never a path below it. Route files are
//           the composition root and are exempt.
//   rule 6: a subsystem that publishes a barrel is imported through that
//           barrel and never through a subdirectory (see BARRELS).
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
// Exceptions: EXCEPTIONS (cross-layer edges) and FEATURE_EXCEPTIONS (feature
// barrel edges) below — explicit registries of grandfathered imports. Every
// entry must carry a reason AND match a real import in the tree; stale entries
// fail the gate (no speculative whitelisting). Both are empty: an entry is a
// debt with an owner and an issue, not a way to land a change.

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const WEB_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const SRC = join(WEB_ROOT, 'src')
const DOC = 'docs/internal/web-package-boundaries.md'

// --- exceptions registry -----------------------------------------------------
const EXCEPTIONS = []
const FEATURE_EXCEPTIONS = []

// --- layer rules ----------------------------------------------------------------
// layer of the importing file -> layers it must not import from.
const FORBIDDEN = {
  components: new Set(['features', 'routes']),
  lib: new Set(['features', 'routes']),
}

// --- barrel rules --------------------------------------------------------------
// A subsystem that publishes a barrel owns its internals: everything outside
// the package imports the barrel and nothing deeper. That is the only thing
// which makes the inside refactorable — data-table/index.ts states the deal
// explicitly ("anything not exported here is free to be reorganised"), and it
// holds only while no outside file reaches into core/, layout/, toolbar/ or
// hooks/. It has been broken once already: features/proxy-logs deep-imported
// DataTableRow from core/data-table-row.
const BARRELS = [
  {
    package: 'components/data-table',
    reason: 'index.ts is the whole public surface (data-table/README.md)',
  },
]

// --- feature barrel rule ------------------------------------------------------
// A feature's public surface is its `index.ts`. Code in another feature imports
// `@/features/<name>` and never a path below it — that is what makes the
// feature's internals refactorable, and it is what lets a consumer take the
// contract without pulling the other feature's page into its chunk.
//
// Route files (`src/routes/`) are the composition root and are exempt: they
// load page components directly on purpose, and no barrel re-exports them.
const FEATURES = 'features/'
const ROUTES = 'routes/'

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
const barrelViolations = []
const featureViolations = []
const matchedFeatureExceptions = new Set()
let featureBarrelEdges = 0
let featureDeepEdges = 0
const matchedExceptions = new Set()
// External files that import each barrel the sanctioned way. A barrel with no
// consumer would make its rule untestable, so this is asserted below rather
// than trusted.
const barrelConsumers = new Map(BARRELS.map((b) => [b.package, new Set()]))
let checkedEdges = 0
let barrelEdges = 0

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

/**
 * Rule 3: outside a barrel's own package, only the barrel itself may be
 * imported. Applies to every layer, unlike FORBIDDEN, which is keyed on the
 * importing file's layer.
 */
function checkBarrelEdge(fileAbsRel, lineNumber, specifier, fileDirRel) {
  const resolved = resolveSpecifier(specifier, fileDirRel)
  if (resolved === null) return
  for (const barrel of BARRELS) {
    const inside = `${barrel.package}/`
    const isBarrelItself =
      resolved === barrel.package || resolved === `${barrel.package}/index`
    // Inside the package the barrel re-exports its own subdirectories and
    // siblings import each other directly: both are the point of a barrel.
    if (fileAbsRel.startsWith(inside)) continue
    if (isBarrelItself) {
      barrelConsumers.get(barrel.package).add(fileAbsRel)
      return
    }
    if (!resolved.startsWith(inside)) continue
    barrelEdges += 1
    barrelViolations.push({
      location: `src/${fileAbsRel}:${lineNumber}`,
      specifier,
      resolved,
      barrel,
    })
  }
}

/**
 * Rule 4: outside its own feature, only the barrel may be imported.
 */
function checkFeatureEdge(fileAbsRel, lineNumber, specifier, fileDirRel) {
  if (!fileAbsRel.startsWith(FEATURES) || fileAbsRel.startsWith(ROUTES)) return
  const owner = fileAbsRel.split('/')[1]
  const resolved = resolveSpecifier(specifier, fileDirRel)
  if (resolved === null || !resolved.startsWith(FEATURES)) return
  const parts = resolved.split('/')
  if (parts[1] === owner) return
  if (parts.length <= 2) {
    featureBarrelEdges += 1
    return
  }
  featureDeepEdges += 1
  const exceptionIndex = FEATURE_EXCEPTIONS.findIndex(
    (entry) =>
      entry.file === `src/${fileAbsRel}` && entry.specifier === specifier
  )
  if (exceptionIndex >= 0) {
    matchedFeatureExceptions.add(exceptionIndex)
    return
  }
  featureViolations.push({
    location: `src/${fileAbsRel}:${lineNumber}`,
    specifier,
    resolved,
    owner,
    target: parts[1],
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
      checkBarrelEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
      checkFeatureEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
    }
    DYNAMIC_RES.lastIndex = 0
    while ((match = DYNAMIC_RES.exec(line)) !== null) {
      checkEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
      checkBarrelEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
      checkFeatureEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
    }
    match = SIDE_EFFECT_RES.exec(line)
    if (match !== null) {
      checkEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
      checkBarrelEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
      checkFeatureEdge(fileAbsRel, lineNumber, match[1], fileDirRel)
    }
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

for (const violation of barrelViolations) {
  failed = true
  console.error(
    `✗ barrel violation: ${violation.location}\n` +
      `    imports '${violation.specifier}' → src/${violation.resolved}\n` +
      `    rule: import '@/components/data-table' instead — ${violation.barrel.reason}.\n` +
      `    If the symbol is missing from the barrel, export it there; do not\n` +
      `    reach into the subsystem, or its internals stop being refactorable.`
  )
}

for (const violation of featureViolations) {
  failed = true
  console.error(
    `✗ feature barrel violation: ${violation.location}\n` +
      `    imports '${violation.specifier}' → src/${violation.resolved}\n` +
      `    rule: features/${violation.owner} must import '@/features/${violation.target}',\n` +
      `    not a path below it. Export the symbol from that feature's index.ts;\n` +
      `    if two features need the same contract, it belongs in the feature that\n` +
      `    owns the domain (or in src/lib/ when neither does).`
  )
}

FEATURE_EXCEPTIONS.forEach((entry, index) => {
  if (!matchedFeatureExceptions.has(index)) {
    failed = true
    console.error(
      `✗ stale feature exception: ${entry.file} → '${entry.specifier}'\n` +
        `    no matching import exists anymore — delete the FEATURE_EXCEPTIONS\n` +
        `    entry (reason was: ${entry.reason})`
    )
  }
})

// Same invariant as the layer rules: a gate that scans nothing and reports no
// violations is not a lenient gate, it is an absent one. Both ways this one can
// go quiet are checked — the package disappearing (rename) and the rule losing
// its only consumer (nothing left to protect).
for (const barrel of BARRELS) {
  const barrelFile = join(SRC, `${barrel.package}/index.ts`)
  if (!existsSync(barrelFile)) {
    failed = true
    console.error(
      `✗ barrel gate would pass vacuously: src/${barrel.package}/index.ts is\n` +
        `    missing. Was the package renamed? Update BARRELS in this script.`
    )
  }
  if (featureBarrelEdges === 0) {
    failed = true
    console.error(
      `✗ feature barrel gate would pass vacuously: no feature imports another\n` +
        `    feature's barrel, so rule 3 has no live surface. Either the features\n` +
        `    stopped talking to each other or the specifier resolution in this\n` +
        `    script stopped matching '@/features/<name>'.`
    )
  }
  if (barrelConsumers.get(barrel.package).size === 0) {
    failed = true
    console.error(
      `✗ barrel gate would pass vacuously: no file outside\n` +
        `    src/${barrel.package}/ imports '@/components/data-table', so the rule\n` +
        `    protects nothing. Either the barrel lost its consumers or the\n` +
        `    specifier resolution in this script stopped matching it.`
    )
  }
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
    `${BARRELS.length} barrel rule(s) held by ` +
    `${[...barrelConsumers.values()].reduce((n, s) => n + s.size, 0)} consumer(s), ` +
    `${barrelEdges} deep import(s), ` +
    `features: ${featureBarrelEdges} barrel edge(s) / ${featureDeepEdges} deep, ` +
    `${FEATURE_EXCEPTIONS.length} feature exception(s) all matched, ` +
    `${EXCEPTIONS.length} registered exception(s) all matched`
)
