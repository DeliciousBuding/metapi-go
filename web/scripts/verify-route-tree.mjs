#!/usr/bin/env node
// Verifies src/routeTree.gen.ts is in sync with src/routes/** before vitest
// runs. Vitest does NOT run @tanstack/router-plugin (the rsbuild config is the
// only consumer that regenerates the tree), so a stale routeTree.gen.ts would
// silently test yesterday's routing table (issue #1035 S1, prerequisite guard
// for S9).
//
// Checks both directions:
//   1. every route file under src/routes/ is imported by routeTree.gen.ts
//   2. every route import in routeTree.gen.ts resolves to an existing file
//
// Exit code 0 = in sync, 1 = drift detected or generated file missing.
// Zero dependencies (node stdlib only).

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)))
const srcDir = path.join(webRoot, 'src')
const routesDir = path.join(srcDir, 'routes')
const genFile = path.join(srcDir, 'routeTree.gen.ts')

const ROUTE_EXTS = ['.tsx', '.ts', '.jsx', '.js']

function fail(lines) {
  for (const line of lines) console.error(`route-tree guard: ${line}`)
  console.error(
    'route-tree guard: regenerate with `bun run dev` or `bun run build` ' +
      '(rsbuild.config.ts runs @tanstack/router-plugin), then re-run the tests.'
  )
  process.exit(1)
}

function walkRouteFiles(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const full = path.join(dir, entry)
    if (statSync(full).isDirectory()) {
      walkRouteFiles(full, out)
      continue
    }
    const ext = path.extname(entry)
    if (!ROUTE_EXTS.includes(ext)) continue
    if (entry.includes('.test.') || entry.includes('.spec.')) continue
    out.push(full)
  }
  return out
}

const slash = (p) => p.replaceAll(path.sep, '/')

// './routes/_authenticated/about' → 'src/routes/_authenticated/about'
// (extension stripped; the generated file imports without extensions).
function specifierFor(file) {
  const rel = slash(path.relative(srcDir, file))
  return `./${rel.replace(/\.(tsx|ts|jsx|js)$/, '')}`
}

// Resolve a generated import specifier to an existing route file.
function resolveSpecifier(spec) {
  const base = path.resolve(srcDir, spec)
  if (ROUTE_EXTS.some((ext) => existsSync(base + ext))) return true
  return ROUTE_EXTS.some((ext) => existsSync(path.join(base, `index${ext}`)))
}

if (!existsSync(genFile)) {
  fail([
    `generated route tree is missing: ${slash(path.relative(webRoot, genFile))}`,
  ])
}

const genSource = readFileSync(genFile, 'utf-8')
const importSpecifiers = [
  ...genSource.matchAll(
    /import\s*\{\s*Route\s+as\s+\w+\s*\}\s*from\s+'([^']+)'/g
  ),
].map((m) => m[1])
if (importSpecifiers.length === 0) {
  fail([
    'routeTree.gen.ts contains no route imports — it looks corrupt or empty',
  ])
}

const routeFiles = walkRouteFiles(routesDir)
const expectedSpecifiers = new Set(routeFiles.map(specifierFor))
const importedSpecifiers = new Set(importSpecifiers)

const missingFromTree = [...expectedSpecifiers]
  .filter((s) => !importedSpecifiers.has(s))
  .sort()
const staleImports = importSpecifiers.filter((s) => !resolveSpecifier(s))
const notInTree = importSpecifiers.filter(
  (s) => !expectedSpecifiers.has(s) && resolveSpecifier(s)
)

const problems = []
if (missingFromTree.length > 0) {
  problems.push(
    `route files absent from routeTree.gen.ts: ${missingFromTree.join(', ')}`
  )
}
if (notInTree.length > 0) {
  problems.push(
    `routeTree.gen.ts imports files outside src/routes/: ${notInTree.join(', ')}`
  )
}
if (staleImports.length > 0) {
  problems.push(
    `routeTree.gen.ts imports that no longer resolve: ${staleImports.join(', ')}`
  )
}
if (problems.length > 0) fail(problems)

console.log(
  `route-tree guard: OK (${routeFiles.length} route files, ${importSpecifiers.length} generated imports in sync)`
)
