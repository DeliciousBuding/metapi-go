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

const ROUTES = [
  ['dashboard', '/'],
  ['models', '/models'],
  ['sites', '/sites'],
  ['accounts', '/accounts'],
  ['checkin', '/checkin'],
  ['token-routes', '/token-routes'],
  ['proxy-logs', '/proxy-logs'],
  ['oauth', '/oauth'],
  ['about', '/about'],
  ['settings-overview', '/settings'],
  ['settings-basic', '/settings/basic/site'],
  ['settings-proxy-models', '/settings/proxy-models/proxy-transport'],
  ['settings-downstream', '/settings/downstream/keys'],
  ['settings-content', '/settings/content/notifications'],
  ['settings-operations', '/settings/operations/program-logs'],
]

const browser = await chromium.launch({ headless: true })
const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
})
await context.addInitScript(
  ({ token }) => {
    localStorage.setItem('auth_token', token)
    localStorage.setItem(
      'auth_token_expires_at',
      String(Date.now() + 12 * 3600 * 1000)
    )
    localStorage.setItem('i18nextLng', 'en')
    document.cookie = 'vite-ui-theme=light; path=/'
  },
  { token: AUTH_TOKEN }
)

const failures = []
for (const [name, path] of ROUTES) {
  const page = await context.newPage()
  await page.goto(BASE_URL + path, { waitUntil: 'networkidle' }).catch(() => {})
  await page.waitForTimeout(500)
  await page.addScriptTag({ path: AXE_SOURCE })
  const violations = await page.evaluate(async () => {
    const result = await window.axe.run(document, {
      resultTypes: ['violations'],
    })
    return result.violations
      .filter((v) => v.impact === 'serious' || v.impact === 'critical')
      .map((v) => ({ id: v.id, nodes: v.nodes.length, help: v.help }))
  })
  for (const v of violations) {
    failures.push(`${name}: ${v.id} (${v.nodes} node(s)) — ${v.help}`)
  }
  await page.close()
}

await browser.close()

if (failures.length > 0) {
  console.error(`[a11y] ${failures.length} serious/critical violation(s):`)
  for (const f of failures) console.error('  ' + f)
  process.exit(1)
}
console.log(
  `[a11y] clean — ${ROUTES.length} routes scanned, 0 serious/critical violations.`
)
