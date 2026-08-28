// metapi-go — golden screenshot regression for 4 key authenticated pages.
//
// Runs against a real server serving the built SPA (BASE_URL), the same way
// the a11y/route-smoke gates do: fresh sqlite DATA_DIR, AUTH_TOKEN exchanged
// for the HttpOnly session cookie via POST /api/auth/login, zh-CN + light
// theme, desktop 1440x900 at DPR 1.
//
// Only light/desktop is golden (the parent regression contract); dark and
// mobile coverage lives in the evidence pipeline (scripts/screenshot-scan.mjs
// + the ui-screenshots CI job).
//
// Usage:
//   bun run visual:regression                       # compare (CI default)
//   UPDATE_SNAPSHOTS=all bun run visual:regression  # write new baselines
import { expect, test } from 'playwright/test'

import { loginSession } from './session-auth.mjs'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'

// Every golden page must be layout-stable and date-independent on a fresh
// (empty) dev DB so baselines stay valid across days and CI runs.
const PAGES = [
  // Highest-traffic admin pages (original four): empty-state charts/tables,
  // no wall-clock text rendered.
  ['dashboard', '/'],
  ['token-routes', '/token-routes'],
  ['accounts', '/accounts'],
  ['sites', '/sites'],
  // Wave 11 expansion — same contract, audited on a fresh DB:
  // models/channels/oauth render empty tables with no timestamp columns when
  // the DB is empty (date fields only appear on data rows).
  ['models', '/models'],
  ['channels', '/channels'],
  ['oauth', '/oauth'],
  // settings-overview is a static card grid of section links; no live data.
  ['settings-overview', '/settings'],
  // settings-basic-site is a static configuration form (defaults only).
  ['settings-basic-site', '/settings/basic/site'],
  // model-tester is a static request form; responses only render after a
  // manual run, so the rest state carries no timestamps.
  ['model-tester', '/model-tester'],
]

test.beforeEach(async ({ context }) => {
  // Same auth seed as a11y-scan/route-smoke: real session login (HttpOnly
  // metapi_session cookie) + zh-CN, light theme via the vite-ui-theme cookie.
  await loginSession(context, { baseUrl: BASE_URL, token: AUTH_TOKEN })
  await context.addInitScript(() => {
    localStorage.setItem('i18nextLng', 'zh-CN')
    document.cookie = 'vite-ui-theme=light; path=/'
  })
})

for (const [name, route] of PAGES) {
  test(`${name} golden (light desktop)`, async ({ page }) => {
    await page.goto(BASE_URL + route, {
      waitUntil: 'domcontentloaded',
      timeout: 30_000,
    })
    // Let data hooks settle and charts finish their intro animation before
    // capturing — makes the baseline a stable rest state, not a spinner.
    await page
      .waitForLoadState('networkidle', { timeout: 10_000 })
      .catch(() => {})
    await page.waitForTimeout(1500)
    await expect(page).toHaveScreenshot(`${name}.png`, {
      fullPage: true,
    })
  })
}
