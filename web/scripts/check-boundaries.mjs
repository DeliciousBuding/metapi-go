#!/usr/bin/env node
// 前端导入门禁，契约见 docs/internal/web-package-boundaries.md。
// 检查 lib/components → features/routes 的禁止边、跨 feature 的公共入口、
// 已登记子系统的公共入口，以及 barrel 注释示例引用的导出名。
// 静态、动态与副作用导入均参与检查；routes 是 feature 的组合根。
// 未覆盖的层间关系见契约文档，不把本门禁当作完整依赖图检查。
// 例外必须匹配真实导入，公共入口与示例必须有实际检查面，避免空扫通过。
// 运行 bun run check:boundaries；bun run lint、pre-push 与 CI 均调用此入口。

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
// 包外只消费公共入口；包内仍可直接导入自己的实现文件。
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
 * 包外不得深入已登记子系统；此检查适用于所有导入方层级。
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
 * feature 之间只消费公共入口；同一 feature 内部不受此限制。
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

// --- barrel doc examples ---------------------------------------------------------
// 注释中的命名导入也必须引用目标 barrel 真正导出的名字；类型检查不读注释。
const BARREL_FILE_RES = /^(?:features|components)\/[^/]+\/index\.ts$/

/** Names a barrel publishes: re-export lists plus its own declarations. */
function barrelExports(text) {
  const names = new Set()
  const listRes = /^export\s+(?:type\s+)?\{([^}]*)\}/gm
  const declRes =
    /^export\s+(?:declare\s+)?(?:async\s+)?(?:function|const|let|var|class|type|interface|enum)\s+([A-Za-z0-9_$]+)/gm
  let match
  while ((match = listRes.exec(text)) !== null) {
    // Strip line comments from the whole brace body BEFORE splitting: a comment
    // inside the braces can contain a comma (token-routes documents why
    // `useRoutes` is shared), which would otherwise split mid-sentence and
    // swallow the symbol that follows it.
    const body = match[1].replace(/\/\/[^\n]*/g, '')
    for (const part of body.split(',')) {
      const named =
        /^\s*(?:type\s+)?([A-Za-z0-9_$]+)(?:\s+as\s+([A-Za-z0-9_$]+))?\s*$/.exec(
          part
        )
      if (named) names.add(named[2] ?? named[1])
    }
  }
  while ((match = declRes.exec(text)) !== null) names.add(match[1])
  return names
}

/**
 * The barrel's header prose, as one string. Comment lines are concatenated so
 * an example import split across several `//` lines still parses; oxfmt does
 * not reformat comment bodies, so this cannot rely on the one-line `from`
 * clause assumption the edge scan above uses.
 */
function commentProse(text) {
  return text
    .split('\n')
    .map((line) => {
      const trimmed = line.trimStart()
      if (trimmed.startsWith('//')) return trimmed.slice(2)
      if (trimmed.startsWith('*')) return trimmed.slice(1)
      return ''
    })
    .join('\n')
}

const DOC_EXAMPLE_RES =
  /import\s+(?:type\s+)?\{([^}]*)\}\s*from\s*['"]([^'"]+)['"]/g

const barrelFiles = files
  .map((file) => file.slice(SRC.length + 1).replaceAll('\\', '/'))
  .filter((rel) => BARREL_FILE_RES.test(rel))
const barrelSources = new Map(
  barrelFiles.map((rel) => [rel, readFileSync(join(SRC, rel), 'utf8')])
)
const barrelExportSets = new Map(
  [...barrelSources].map(([rel, text]) => [rel, barrelExports(text)])
)
const docExampleViolations = []
let docExamples = 0

for (const [rel, text] of barrelSources) {
  const prose = commentProse(text)
  const fileDirRel = dirname(rel)
  DOC_EXAMPLE_RES.lastIndex = 0
  let match
  while ((match = DOC_EXAMPLE_RES.exec(prose)) !== null) {
    const target = resolveSpecifier(match[2], fileDirRel)
    const targetBarrel = target === null ? null : `${target}/index.ts`
    if (target === null || !barrelExportSets.has(targetBarrel)) continue
    const exported = barrelExportSets.get(targetBarrel)
    for (const part of match[1].split(',')) {
      const named =
        /^\s*(?:type\s+)?([A-Za-z0-9_$]+)(?:\s+as\s+([A-Za-z0-9_$]+))?\s*$/.exec(
          part
        )
      if (!named) continue
      const symbol = named[1]
      docExamples += 1
      if (!exported.has(symbol)) {
        docExampleViolations.push({ location: rel, symbol, targetBarrel })
      }
    }
  }
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

for (const violation of docExampleViolations) {
  failed = true
  console.error(
    `✗ barrel doc example does not resolve: ${violation.location}\n` +
      `    documents '${violation.symbol}', which src/${violation.targetBarrel} does not export.\n` +
      `    A comment is the only place this can rot silently: fix the example, or\n` +
      `    export the symbol if a consumer genuinely needs it.`
  )
}

if (docExamples === 0) {
  failed = true
  console.error(
    `✗ barrel doc-example check would pass vacuously: no documented example\n` +
      `    import was found in any src/{features,components}/*/index.ts header.\n` +
      `    Either the examples were removed or DOC_EXAMPLE_RES stopped matching\n` +
      `    (comment prose is concatenated before matching, so a multi-line\n` +
      `    example must still parse).`
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

// feature 检查面不依赖子系统登记项，不能放进 BARRELS 循环。
if (featureBarrelEdges === 0) {
  failed = true
  console.error(
    `✗ feature barrel gate would pass vacuously: no feature imports another\n` +
      `    feature's barrel. Either the features stopped talking to each other\n` +
      `    or specifier resolution stopped matching '@/features/<name>'.`
  )
}

// 子系统入口必须存在且有包外消费者，否则门禁没有实际检查面。
for (const barrel of BARRELS) {
  const barrelFile = join(SRC, `${barrel.package}/index.ts`)
  if (!existsSync(barrelFile)) {
    failed = true
    console.error(
      `✗ barrel gate would pass vacuously: src/${barrel.package}/index.ts is\n` +
        `    missing. Was the package renamed? Update BARRELS in this script.`
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
    `${EXCEPTIONS.length} registered exception(s) all matched, ` +
    `${docExamples} barrel doc-example symbol(s) resolved`
)
