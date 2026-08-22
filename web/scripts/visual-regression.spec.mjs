// metapi-go — golden screenshot regression for 4 key authenticated pages.
//
// Runs against a real server serving the built SPA (BASE_URL), the same way
// the a11y/route-smoke gates do: fresh sqlite DATA_DIR, AUTH_TOKEN dev seed
// via localStorage, zh-CN + light theme, desktop 1440x900 at DPR 1.
//
// Only light/desktop is golden (the parent regression contract); dark and
// mobile coverage lives in the evidence pipeline (scripts/screenshot-scan.mjs
// + the ui-screenshots CI job).
//
// Usage:
//   bun run visual:regression                       # compare (CI default)
//   UPDATE_SNAPSHOTS=all bun run visual:regression  # write new baselines
import { expect, test } from 'playwright/test'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'

// Kept deliberately narrow: these four are the highest-traffic admin pages and
// their layout is date-independent on a fresh (empty) dev DB, so baselines are
// stable across days and CI runs.
const PAGES = [
  ['dashboard', '/'],
  ['token-routes', '/token-routes'],
  ['accounts', '/accounts'],
  ['sites', '/sites'],
]

test.beforeEach(async ({ context }) => {
  // Same auth seed as a11y-scan/route-smoke: localStorage token + zh-CN,
  // light theme via the app's vite-ui-theme cookie.
  await context.addInitScript(
    ({ token }) => {
      localStorage.setItem('auth_token', token)
      localStorage.setItem(
        'auth_token_expires_at',
        String(Date.now() + 12 * 3600 * 1000)
      )
      localStorage.setItem('i18nextLng', 'zh-CN')
      document.cookie = 'vite-ui-theme=light; path=/'
    },
    { token: AUTH_TOKEN }
  )
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
