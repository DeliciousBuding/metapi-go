// Interaction sweep: open every reachable dialog/sheet/popover/menu and
// capture the resulting state, desktop (1440x900) + mobile (375x812), light
// theme. One scenario per file; failures are collected, never abort the run.
//
// Complements screenshot-scan.mjs (static per-route first-paint evidence):
// that harness cannot reach overlays, form validation states, or the header
// chrome menus (cmdk / attention bell / theme customizer / user menu).
//
// Prereq: same fresh-dist contract as screenshot-scan (rebuild web/dist and
// restart the Go server first), plus seeded sites/accounts so list pages
// render rows. Selector notes learned the hard way:
//   - the attention bell's aria-label is the dynamic attention.trigger* copy
//     ("待关注告警"), not "notifications";
//   - base-ui Select triggers render role=combobox, and inside a FormControl
//     wrapper their data-slot is merged away — match visible text instead;
//   - mobile list pages render MobileCardList (no <tbody>), so row-menu
//     scenarios are desktop-only.
//
// Usage:
//   BASE_URL=http://127.0.0.1:4000 OUT_DIR=<dir> node scripts/shot-interactions.mjs
import { mkdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { chromium } from 'playwright'

import { loginSession } from './session-auth.mjs'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4000'
const THEME = process.env.THEME ?? 'light'
// Portable default: OS temp dir (same convention as screenshot-scan.mjs).
// Override OUT_DIR to keep screenshots in a stable location.
const OUT_BASE =
  process.env.OUT_DIR ?? join(tmpdir(), 'metapi-shots-interactions')

const results = []

async function newPage(browser, viewport, isMobile) {
  const context = await browser.newContext({
    viewport,
    deviceScaleFactor: 2,
    locale: 'zh-CN',
    isMobile,
  })
  await loginSession(context, {
    baseUrl: BASE_URL,
    token: 'dev-admin-token-123',
  })
  await context.addCookies([
    { name: 'vite-ui-theme', value: THEME, url: BASE_URL },
  ])
  await context.addInitScript(() => localStorage.setItem('i18nextLng', 'zh-CN'))
  return context
}

async function settle(page, ms = 500) {
  await page.waitForLoadState('networkidle', { timeout: 6000 }).catch(() => {})
  await page.waitForTimeout(ms)
}

/**
 * Each scenario: { name, viewport: 'desktop'|'mobile'|'both', run(page) }.
 * run() navigates and opens the target overlay; the harness screenshots.
 */
const SCENARIOS = [
  // ---- sites ----
  {
    name: 'sites-add-dialog',
    run: async (page) => {
      await page.goto(BASE_URL + '/sites', { waitUntil: 'domcontentloaded' })
      await settle(page)
      await page.getByRole('button', { name: '添加站点' }).first().click()
      await page.waitForTimeout(600)
    },
  },
  {
    name: 'sites-row-menu',
    viewport: 'desktop', // mobile renders cards (no tbody); row menu = same popover, covered on desktop
    run: async (page) => {
      await page.goto(BASE_URL + '/sites', { waitUntil: 'domcontentloaded' })
      await settle(page)
      await page.locator('tbody tr').first().hover()
      await page
        .getByRole('button', { name: /操作|更多|actions|more/i })
        .first()
        .click()
        .catch(async () => {
          await page.locator('tbody tr button').last().click()
        })
      await page.waitForTimeout(400)
    },
  },
  {
    name: 'sites-column-settings',
    run: async (page) => {
      await page.goto(BASE_URL + '/sites', { waitUntil: 'domcontentloaded' })
      await settle(page)
      await page.getByRole('button', { name: '列设置' }).click()
      await page.waitForTimeout(400)
    },
  },
  // ---- accounts ----
  {
    name: 'accounts-add-dialog',
    run: async (page) => {
      await page.goto(BASE_URL + '/accounts', { waitUntil: 'domcontentloaded' })
      await settle(page)
      await page
        .getByRole('button', { name: /添加账号|新增账号/ })
        .first()
        .click()
      await page.waitForTimeout(600)
    },
  },
  {
    name: 'accounts-row-menu',
    viewport: 'desktop', // mobile renders cards; same popover covered on desktop
    run: async (page) => {
      await page.goto(BASE_URL + '/accounts', { waitUntil: 'domcontentloaded' })
      await settle(page)
      await page.locator('tbody tr').first().hover()
      await page.locator('tbody tr button').last().click()
      await page.waitForTimeout(400)
    },
  },
  // ---- token routes ----
  {
    name: 'routes-add-dialog',
    run: async (page) => {
      await page.goto(BASE_URL + '/token-routes', {
        waitUntil: 'domcontentloaded',
      })
      await settle(page)
      await page.getByRole('button', { name: '添加路由' }).first().click()
      await page.waitForTimeout(600)
    },
  },
  // ---- model tester ----
  {
    name: 'model-tester-template-select',
    run: async (page) => {
      await page.goto(BASE_URL + '/model-tester', {
        waitUntil: 'domcontentloaded',
      })
      await settle(page, 1200)
      // Select triggers render with role=combobox (ARIA) and, inside
      // FormControl wrappers, lose their data-slot to the merge — match the
      // visible placeholder text instead.
      await page.locator('button:has-text("选择预设模板")').first().click()
      await page.waitForTimeout(500)
    },
  },
  // ---- checkin ----
  {
    name: 'checkin-manual-dialog',
    run: async (page) => {
      await page.goto(BASE_URL + '/checkin', { waitUntil: 'domcontentloaded' })
      await settle(page)
      const btn = page.getByRole('button', { name: /手动签到|签到/ }).first()
      await btn.click().catch(() => {})
      await page.waitForTimeout(500)
    },
  },
  // ---- chrome: cmdk / notifications / theme / language / user ----
  {
    name: 'chrome-cmdk',
    run: async (page) => {
      await page.goto(BASE_URL + '/dashboard', {
        waitUntil: 'domcontentloaded',
      })
      await settle(page)
      await page.keyboard.press('Control+k')
      await page.waitForTimeout(500)
    },
  },
  {
    name: 'chrome-notifications',
    run: async (page) => {
      await page.goto(BASE_URL + '/dashboard', {
        waitUntil: 'domcontentloaded',
      })
      await settle(page)
      await page
        .locator(
          'header button[aria-label*="告警"], header button[aria-label*="ttention" i]'
        )
        .first()
        .click()
      await page.waitForTimeout(500)
    },
  },
  {
    name: 'chrome-theme-customizer',
    run: async (page) => {
      await page.goto(BASE_URL + '/dashboard', {
        waitUntil: 'domcontentloaded',
      })
      await settle(page)
      await page
        .getByRole('button', { name: /外观|Appearance/i })
        .first()
        .click()
      await page.waitForTimeout(500)
    },
  },
  {
    name: 'chrome-user-menu',
    run: async (page) => {
      await page.goto(BASE_URL + '/dashboard', {
        waitUntil: 'domcontentloaded',
      })
      await settle(page)
      await page
        .locator('header button')
        .last()
        .click()
        .catch(() => {})
      await page.waitForTimeout(400)
    },
  },
  // ---- settings ----
  {
    name: 'settings-danger-reset-dialog',
    viewport: 'desktop',
    run: async (page) => {
      await page.goto(BASE_URL + '/settings/operations/danger-zone', {
        waitUntil: 'domcontentloaded',
      })
      await settle(page)
      const btn = page.getByRole('button', { name: /恢复出厂|重置/ }).first()
      await btn.click().catch(() => {})
      await page.waitForTimeout(600)
    },
  },
  // ---- mobile chrome ----
  {
    name: 'mobile-sidebar-drawer',
    viewport: 'mobile',
    run: async (page) => {
      await page.goto(BASE_URL + '/dashboard', {
        waitUntil: 'domcontentloaded',
      })
      await settle(page)
      await page.locator('header button').first().click()
      await page.waitForTimeout(500)
    },
  },
  {
    name: 'mobile-sites-add',
    viewport: 'mobile',
    run: async (page) => {
      await page.goto(BASE_URL + '/sites', { waitUntil: 'domcontentloaded' })
      await settle(page)
      await page.getByRole('button', { name: '添加站点' }).first().click()
      await page.waitForTimeout(600)
    },
  },
  // ---- form validation state ----
  {
    name: 'sites-add-validation',
    viewport: 'desktop',
    run: async (page) => {
      await page.goto(BASE_URL + '/sites', { waitUntil: 'domcontentloaded' })
      await settle(page)
      await page.getByRole('button', { name: '添加站点' }).first().click()
      await page.waitForTimeout(600)
      // Submit empty to trigger validation errors.
      const submit = page
        .locator(
          '[data-slot=dialog-footer] button[type=submit], form button[type=submit]'
        )
        .last()
      await submit.click().catch(() => {})
      await page.waitForTimeout(500)
    },
  },
]

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})

for (const scenario of SCENARIOS) {
  const targets =
    scenario.viewport === 'desktop'
      ? [['desktop', { width: 1440, height: 900 }, false]]
      : scenario.viewport === 'mobile'
        ? [['mobile', { width: 375, height: 812 }, true]]
        : [
            ['desktop', { width: 1440, height: 900 }, false],
            ['mobile', { width: 375, height: 812 }, true],
          ]
  for (const [vpName, viewport, isMobile] of targets) {
    const dir = `${OUT_BASE}/${vpName}`
    mkdirSync(dir, { recursive: true })
    const context = await newPage(browser, viewport, isMobile)
    const page = await context.newPage()
    try {
      await scenario.run(page)
      await page.screenshot({ path: `${dir}/${scenario.name}.png` })
      results.push(`OK   ${vpName}/${scenario.name}`)
    } catch (error) {
      const reason = String(error?.message ?? error).split('\n')[0]
      results.push(`FAIL ${vpName}/${scenario.name}: ${reason}`)
      await page
        .screenshot({ path: `${dir}/${scenario.name}-FAIL.png` })
        .catch(() => {})
    } finally {
      await context.close().catch(() => {})
    }
  }
}

await browser.close()
console.log(results.join('\n'))
console.log(
  `done: ${results.filter((r) => r.startsWith('OK')).length}/${results.length}`
)
