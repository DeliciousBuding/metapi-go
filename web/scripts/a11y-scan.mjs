#!/usr/bin/env node
// metapi-go — axe-core accessibility gate (serious/critical only).
//
// Scans the authenticated admin routes with axe-core and exits non-zero when
// any serious/critical violation is found. Requires the dev server:
//
//   cd web && bun run a11y:scan            # http://localhost:3000
//   BASE_URL=http://localhost:3000 bun run a11y:scan
//
// Auth is seeded from the local dev session (dev-admin-token-123) before each
// load; that token/port are local dev values only. axe-core is injected from
// node_modules (no @axe-core/playwright needed — its dist is pruned).
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { chromium } from 'playwright'

// Single source of truth: the desktop route inventory lives in
// route-smoke.mjs; importing it keeps the two gates from drifting.
import { DESKTOP_ROUTES } from './route-smoke.mjs'

const BASE_URL = process.env.BASE_URL ?? 'http://localhost:3000'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url))
const AXE_SOURCE = join(
  SCRIPT_DIR,
  '..',
  'node_modules',
  'axe-core',
  'axe.min.js'
)

// Route names are derived from the shared inventory so failure output stays
// readable without a second hand-maintained list.
const ROUTES = DESKTOP_ROUTES.map((path) => [
  path === '/' ? 'dashboard' : path.slice(1).replaceAll('/', '-'),
  path,
])

// Both shipped locales are scanned: translated chrome strings can introduce
// a11y failures the English pass never sees (label collisions, i18n key
// fallbacks, copy-level violations).
const LOCALES = ['en', 'zh-CN']

const browser = await chromium.launch({ headless: true })

async function scanLocale(locale) {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
  })
  await context.addInitScript(
    ({ token, locale }) => {
      localStorage.setItem('auth_token', token)
      localStorage.setItem(
        'auth_token_expires_at',
        String(Date.now() + 12 * 3600 * 1000)
      )
      localStorage.setItem('i18nextLng', locale)
      document.cookie = 'vite-ui-theme=light; path=/'
    },
    { token: AUTH_TOKEN, locale }
  )

  const failures = []
  for (const [name, path] of ROUTES) {
    const page = await context.newPage()
    await page
      .goto(BASE_URL + path, { waitUntil: 'networkidle' })
      .catch(() => {})
    await page.waitForTimeout(500)
    // CSP script-src is 'self' (no 'unsafe-inline') since #1033, so an inline
    // <script> injection via addScriptTag is blocked by the browser. Execute
    // the axe bundle through page.evaluate instead — CDP Runtime.evaluate is
    // not subject to the page CSP — which leaves window.axe available
    // regardless.
    await page.evaluate(readFileSync(AXE_SOURCE, 'utf-8'))
    const violations = await page.evaluate(async () => {
      const result = await window.axe.run(document, {
        resultTypes: ['violations'],
      })
      return result.violations
        .filter((v) => v.impact === 'serious' || v.impact === 'critical')
        .map((v) => ({ id: v.id, nodes: v.nodes.length, help: v.help }))
    })
    for (const v of violations) {
      failures.push(
        `${locale}/${name}: ${v.id} (${v.nodes} node(s)) — ${v.help}`
      )
    }
    await page.close()
  }
  await context.close()
  return failures
}

const failures = []
for (const locale of LOCALES) {
  failures.push(...(await scanLocale(locale)))
}

await browser.close()

if (failures.length > 0) {
  console.error(`[a11y] ${failures.length} serious/critical violation(s):`)
  for (const f of failures) console.error('  ' + f)
  process.exit(1)
}
console.log(
  `[a11y] clean — ${ROUTES.length} routes × ${LOCALES.length} locales scanned, 0 serious/critical violations.`
)
