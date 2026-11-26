#!/usr/bin/env node
// Use Vitest's module graph, with conservative fallbacks for non-import edges.
import { readFileSync } from 'node:fs'
import { relative, resolve } from 'node:path'

import { createVitest } from 'vitest/node'

const input = JSON.parse(process.env.SELECTED_WEB_FILES ?? '[]')
if (
  !Array.isArray(input) ||
  input.some(
    (file) =>
      typeof file !== 'string' ||
      file.includes('..') ||
      file.startsWith('/') ||
      file.includes('\n')
  )
) {
  throw new Error('invalid frontend impact file list')
}
const feature = (file) => /^src\/features\/([^/]+)\//.exec(file)?.[1]
// Shared code, styles, routing, public assets, deletion and empty input cannot
// be bounded by a feature owner. Never use passWithNoTests.
const full = input.length === 0 || input.some((file) => !feature(file))
const ctx = await createVitest({ run: true, watch: false, allowOnly: false })
try {
  const all = await ctx.globTestSpecifications()
  if (all.length === 0) throw new Error('frontend inventory unexpectedly empty')
  let selected = all
  if (!full) {
    const owners = new Set(input.map(feature))
    // Vitest's module graph keys files with POSIX slashes (even on Windows via
    // pathe.resolve), while Node's path.resolve emits backslashes on win32 and
    // would miss every entry in `importers`.
    ctx.config.related = input.map((file) =>
      resolve(ctx.config.root, file).replaceAll('\\', '/')
    )
    const related = await ctx.getRelevantTestSpecifications()
    const ids = new Set(related.map((spec) => spec.moduleId))
    // Keep whole changed feature suites, plus filesystem contracts which read
    // source/assets directly and therefore are absent from the import graph.
    selected = all.filter((spec) => {
      const file = relative(ctx.config.root, spec.moduleId).replaceAll(
        '\\',
        '/'
      )
      const source = readFileSync(spec.moduleId, 'utf8')
      return (
        ids.has(spec.moduleId) ||
        owners.has(feature(file)) ||
        source.includes('node:fs') ||
        source.includes("from 'fs'")
      )
    })
    if (
      related.length === 0 ||
      input.some((file) => {
        try {
          readFileSync(file)
          return false
        } catch {
          return true
        }
      })
    ) {
      selected = all
      console.log(
        'frontend: unresolved/deleted source or empty related graph; full fallback'
      )
    }
    ctx.config.related = undefined
  }
  console.log(
    `frontend: ${selected.length}/${all.length} test files; feature owners + Vitest reverse imports + filesystem contracts`
  )
  if (process.env.METAPI_WEB_PLAN_ONLY === '1') {
    console.log(
      JSON.stringify(
        selected.map((spec) => relative(ctx.config.root, spec.moduleId))
      )
    )
  } else {
    await ctx.standalone()
    const result = await ctx.runTestSpecifications(selected)
    if (
      result.testModules.some((module) => module.state() === 'failed') ||
      result.unhandledErrors.length > 0
    )
      process.exitCode = 1
  }
} finally {
  await ctx.close()
}
