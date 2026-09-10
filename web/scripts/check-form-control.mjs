#!/usr/bin/env node
// metapi-go/web — FormControl prop-forwarding gate.
//
// `<FormControl>` clones its single child with `id`, `aria-describedby` and
// `aria-invalid` (see src/components/ui/form.tsx), and `<FormLabel htmlFor>`
// points at that id. A child component that does not forward those props onto a
// real DOM node silently loses the field's accessible name and its
// describedby/invalid wiring — with no type error, no runtime error, and no
// failure from the CI `a11y` job (it scans authenticated routes, not the inside
// of these dialogs).
//
// Empirically confirmed instance: `EndpointsEditor` — `queryByLabelText('API
// endpoints')` returns null while every sibling field in the same form resolves.
// The class recurred across five components / ten call sites, so per AGENTS.md
// ("a violation class seen a third time gets a deterministic gate") it is now
// mechanically enforced. Tracked in issue #1300.
//
// Rule: a `<FormControl>` child must reach a DOM node carrying the injected
// props. Satisfied when the child is (a) a native element, (b) a
// `components/ui/**` primitive (all verified to spread rest props), or (c) a
// component whose own source takes a rest parameter and spreads it into JSX.
// Anything else must be registered in EXCEPTIONS with a reason; a stale entry
// fails the gate, so the registry cannot rot.
//
// Accountability: every raw `<FormControl>` occurrence must end up classified or
// recognised as prose inside a comment. An unhandled shape fails the gate rather
// than being skipped — a gate that silently scans less than it claims is worse
// than no gate (AGENTS.md, #1175: test-sqlite-shard selected zero packages and
// reported success for 30 tags).
//
// Run: bun run check:form-control (chained into `bun run lint`).

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const WEB_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const SRC = join(WEB_ROOT, 'src')
const UI_DIR = join(SRC, 'components', 'ui')
const ISSUE = 'https://github.com/DeliciousBuding/metapi-go/issues/1300'
const REFERENCE_IMPL = 'src/features/sites/components/custom-headers-field.tsx'

// --- exceptions registry -----------------------------------------------------
// Composites that currently swallow FormControl's injected props. Each needs a
// design decision rather than a mechanical fix: a group of controls has no
// single labelable element, so the correct remedy is role="group" +
// aria-labelledby, not forwarding one id onto an arbitrary inner input. Delete
// the entry as soon as the component is fixed — a stale entry fails the gate.
const EXCEPTIONS = [
  {
    component: 'EndpointsEditor',
    reason:
      'composite endpoint-row editor; label association empirically broken (queryByLabelText -> null). Fix pattern in #1300.',
  },

  {
    component: 'ScheduleEditor',
    reason:
      'composite schedule editor, 4 call sites (scheduling-section x3, import-export-section); same mechanism, not individually probed. #1300.',
  },
  {
    component: 'ModelPolicyEditor',
    reason: 'composite policy editor in key-sheet-form; same mechanism. #1300.',
  },
  {
    component: 'SiteScopePicker',
    reason: 'composite scope picker in key-sheet-form; same mechanism. #1300.',
  },
  {
    component: 'CredentialRefPicker',
    reason:
      'composite credential picker, 2 call sites in key-sheet-form; same mechanism. #1300.',
  },
]

// `<FormControl>` and its child may be separated by whitespace and JSX comments.
const RAW_RES = /<FormControl>/g
const CHILD_RES = /^\s*(?:\{\/\*[\s\S]*?\*\/\}\s*)*<([A-Za-z][\w.]*)/
const IMPORT_RES = /import\s+(?:type\s+)?\{([^}]*)\}\s+from\s+['"]([^'"]+)['"]/g

function walk(dir, out) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (entry === '__tests__' || entry === 'node_modules') continue
      walk(full, out)
      continue
    }
    if (entry.endsWith('.tsx')) out.push(full)
  }
  return out
}

function resolveSpecifier(fromFile, specifier) {
  let base
  if (specifier.startsWith('@/')) base = join(SRC, specifier.slice(2))
  else if (specifier.startsWith('.'))
    base = resolve(dirname(fromFile), specifier)
  else return null
  for (const candidate of [
    `${base}.tsx`,
    `${base}.ts`,
    join(base, 'index.tsx'),
    join(base, 'index.ts'),
  ]) {
    if (existsSync(candidate) && statSync(candidate).isFile()) return candidate
  }
  return null
}

/** Does this component take a rest parameter AND spread it into JSX? */
function forwardsInjectedProps(file, component) {
  const source = readFileSync(file, 'utf8')
  const decl = source.match(
    new RegExp(`(?:function|const)\\s+${component}\\b[\\s\\S]{0,900}?\\)`)
  )
  if (!decl) return false
  if (!/\.\.\.(props|rest|restProps)\b/.test(decl[0])) return false
  return /\{\.\.\.(props|rest|restProps)\}/.test(source)
}

const files = walk(SRC, [])
const violations = []
const unclassified = []
const matchedExceptions = new Set()
const tally = {
  native: 0,
  uiPrimitive: 0,
  forwarding: 0,
  exception: 0,
  prose: 0,
}
let checked = 0

for (const file of files) {
  const source = readFileSync(file, 'utf8')
  if (!source.includes('<FormControl>')) continue

  const imports = new Map()
  for (const match of source.matchAll(IMPORT_RES)) {
    const resolved = resolveSpecifier(file, match[2])
    if (!resolved) continue
    for (const raw of match[1].split(',')) {
      const name = raw
        .trim()
        .split(/\s+as\s+/)
        .pop()
        ?.trim()
      if (name) imports.set(name, resolved)
    }
  }

  for (const match of source.matchAll(RAW_RES)) {
    const line = source.slice(0, match.index).split('\n').length
    const location = `${relative(WEB_ROOT, file)}:${line}`
    const lineStart = source.lastIndexOf('\n', match.index) + 1
    const before = source.slice(lineStart, match.index)

    // Prose: the occurrence sits in a line comment or a JSDoc/block comment.
    if (/^\s*(\/\/|\*|\/\*)/.test(before)) {
      tally.prose++
      continue
    }

    const child = CHILD_RES.exec(
      source.slice(match.index + '<FormControl>'.length)
    )?.[1]
    if (!child) {
      unclassified.push(location)
      continue
    }
    checked++

    if (child[0] === child[0].toLowerCase()) {
      tally.native++ // native element: the clone lands on a real node
      continue
    }

    const target =
      imports.get(child) ??
      (new RegExp(`(?:function|const)\\s+${child}\\b`).test(source)
        ? file
        : null)
    if (!target) {
      violations.push({
        location,
        child,
        why: 'could not resolve the component source',
      })
      continue
    }
    if (target.startsWith(UI_DIR)) {
      tally.uiPrimitive++ // every components/ui primitive spreads rest props
      continue
    }
    if (forwardsInjectedProps(target, child)) {
      tally.forwarding++
      continue
    }
    const index = EXCEPTIONS.findIndex((entry) => entry.component === child)
    if (index >= 0) {
      matchedExceptions.add(index)
      tally.exception++
      continue
    }
    violations.push({
      location,
      child,
      why: 'does not forward the id / aria-describedby / aria-invalid that FormControl injects',
    })
  }
}

let failed = false

for (const violation of violations) {
  failed = true
  console.error(
    `✗ FormControl child loses its accessible name: ${violation.location}\n` +
      `    <${violation.child}> ${violation.why}\n` +
      `    <FormLabel htmlFor> then points at a nonexistent id: the control has no accessible\n` +
      `    name and receives no aria-describedby / aria-invalid.\n` +
      `    Fix: take a rest parameter and spread it onto the DOM node that should carry the id\n` +
      `    (reference: ${REFERENCE_IMPL}), or give a composite control role="group" +\n` +
      `    aria-labelledby. Knowingly deferred -> register in web/scripts/check-form-control.mjs\n` +
      `    EXCEPTIONS with a reason and an issue link.`
  )
}

for (const location of unclassified) {
  failed = true
  console.error(
    `✗ unclassified <FormControl> at ${location}\n` +
      `    the gate could not identify its child element, so it did not check this site.\n` +
      `    Extend CHILD_RES in web/scripts/check-form-control.mjs to cover this shape —\n` +
      `    do not let the gate report clean over a site it never looked at.`
  )
}

EXCEPTIONS.forEach((entry, index) => {
  if (!matchedExceptions.has(index)) {
    failed = true
    console.error(
      `✗ stale FormControl exception: ${entry.component}\n` +
        `    no unforwarded <FormControl> child by that name exists anymore — delete the entry.\n` +
        `    (reason was: ${entry.reason})`
    )
  }
})

if (failed) process.exit(1)

console.log(
  `✓ FormControl forwarding clean: ${checked} site(s) classified in ${files.length} files — ` +
    `${tally.native} native, ${tally.uiPrimitive} ui primitive, ${tally.forwarding} forwarding component, ` +
    `${tally.exception} registered exception call site(s); ${tally.prose} prose occurrence(s) skipped; ` +
    `0 unclassified. Exceptions: ${EXCEPTIONS.length}/${EXCEPTIONS.length} matched (${ISSUE})`
)
