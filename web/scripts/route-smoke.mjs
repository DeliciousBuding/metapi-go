#!/usr/bin/env node
// metapi-go — real-browser UI/route stability gate.
//
// This is intentionally broader than the axe scan. It loads every first-party
// admin surface from the production SPA and fails on browser/runtime errors,
// HTTP 5xx responses, failed requests, an unresponsive renderer, known React
// error-boundary text, or mobile document overflow. It also exercises the
// accounts credential-mode interaction that previously exposed a controlled
// table/router feedback loop.

import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://localhost:3000'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const ROUTE_TIMEOUT_MS = 15_000
const RESPONSIVE_TIMEOUT_MS = 2_500

const SETTINGS_ROUTES = [
  '/settings/general/site',
  '/settings/general/authentication',
  '/settings/general/scheduling',
  '/settings/general/proxy-transport',
  '/settings/general/routing',
  '/settings/downstream/keys',
  '/settings/downstream/proxy-token',
  '/settings/models/redirects',
  '/settings/models/rates',
  '/settings/models/allowlist',
  '/settings/content/import-export',
  '/settings/content/notifications',
  '/settings/content/announcements',
  '/settings/system-info/program-logs',
  '/settings/system-info/audit-logs',
  '/settings/system-info/update-center',
  '/settings/system-info/database',
  '/settings/system-info/maintenance',
  '/settings/system-info/danger-zone',
]

const DESKTOP_ROUTES = [
  '/',
  '/about',
  '/accounts',
  '/channels',
  '/checkin',
  '/fix-candidates',
  '/model-tester',
  '/models',
  '/oauth',
  '/observability',
  '/price-compare',
  '/proxy-logs',
  '/site-announcements',
  '/sites',
  '/token-routes',
  '/dashboard',
  '/dashboard/overview',
  '/dashboard/traffic',
  '/dashboard/models',
  '/dashboard/availability',
  '/settings',
  ...SETTINGS_ROUTES,
]

const MOBILE_ROUTES = [
  '/',
  '/accounts',
  '/channels',
  '/checkin',
  '/fix-candidates',
  '/model-tester',
  '/models',
  '/oauth',
  '/observability',
  '/price-compare',
  '/proxy-logs',
  '/site-announcements',
  '/sites',
  '/token-routes',
]

if (DESKTOP_ROUTES.length !== 40) {
  throw new Error(
    `route manifest drift: expected 40 desktop routes, got ${DESKTOP_ROUTES.length}`
  )
}
if (MOBILE_ROUTES.length !== 14) {
  throw new Error(
    `route manifest drift: expected 14 mobile routes, got ${MOBILE_ROUTES.length}`
  )
}

const KNOWN_FATAL_TEXT = [
  /maximum update depth exceeded/i,
  /too many re-renders/i,
  /unexpected application error/i,
  /something went wrong/i,
  /minified react error/i,
]

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

async function seedAuth(context) {
  await context.addInitScript((token) => {
    localStorage.setItem('auth_token', token)
    localStorage.setItem(
      'auth_token_expires_at',
      String(Date.now() + 12 * 3600 * 1000)
    )
    localStorage.setItem('i18nextLng', 'en')
    document.cookie = 'vite-ui-theme=light; path=/'
  }, AUTH_TOKEN)
}

async function seedInteractionData(request) {
  const response = await request.post(`${BASE_URL}/api/sites`, {
    headers: { Authorization: `Bearer ${AUTH_TOKEN}` },
    data: {
      name: 'Browser Smoke Site',
      url: 'https://example.test',
      platform: 'new-api',
      status: 'active',
    },
  })

  // The CI database is fresh (normally 200/201). 409 is accepted so the
  // script is also repeatable against a developer's persistent local DB.
  if (![200, 201, 409].includes(response.status())) {
    throw new Error(
      `failed to seed browser smoke site: HTTP ${response.status()} ${await response.text()}`
    )
  }
}

function observePage(page, label, failures) {
  page.on('pageerror', (error) => {
    failures.push(`${label}: pageerror — ${error.message}`)
  })
  page.on('console', (message) => {
    if (message.type() === 'error') {
      failures.push(`${label}: console.error — ${message.text()}`)
    }
  })
  page.on('requestfailed', (request) => {
    failures.push(
      `${label}: request failed — ${request.method()} ${request.url()} (${request.failure()?.errorText ?? 'unknown'})`
    )
  })
  page.on('response', (response) => {
    if (response.status() >= 500) {
      failures.push(`${label}: HTTP ${response.status()} — ${response.url()}`)
    }
  })
}

async function assertRendererResponsive(page, label) {
  const probe = page.evaluate(
    () =>
      new Promise((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve('ok')))
      })
  )
  const result = await Promise.race([
    probe,
    delay(RESPONSIVE_TIMEOUT_MS).then(() => 'timeout'),
  ])
  if (result !== 'ok') {
    throw new Error(
      `${label}: renderer did not yield within ${RESPONSIVE_TIMEOUT_MS}ms`
    )
  }
}

async function assertNoFatalUi(page, label) {
  const bodyText = await page.locator('body').innerText({ timeout: 3_000 })
  for (const pattern of KNOWN_FATAL_TEXT) {
    if (pattern.test(bodyText)) {
      throw new Error(`${label}: fatal UI text matched ${pattern}`)
    }
  }
}

async function scanRoute(context, path, mode, failures) {
  const label = `${mode} ${path}`
  const page = await context.newPage()
  observePage(page, label, failures)
  try {
    const response = await page.goto(BASE_URL + path, {
      waitUntil: 'domcontentloaded',
      timeout: ROUTE_TIMEOUT_MS,
    })
    if (!response) throw new Error(`${label}: navigation returned no response`)
    if (response.status() >= 400) {
      throw new Error(`${label}: document returned HTTP ${response.status()}`)
    }

    await page.waitForTimeout(250)
    await assertRendererResponsive(page, label)
    await assertNoFatalUi(page, label)

    if (mode === 'mobile') {
      const overflow = await page.evaluate(
        () =>
          document.documentElement.scrollWidth -
          document.documentElement.clientWidth
      )
      if (overflow > 1) {
        throw new Error(
          `${label}: document overflows horizontally by ${overflow}px`
        )
      }
    }
  } catch (error) {
    failures.push(
      error instanceof Error ? error.message : `${label}: ${String(error)}`
    )
  } finally {
    await page.close().catch(() => {})
  }
}

async function exerciseAccounts(context, failures) {
  const label = 'accounts interaction'
  const page = await context.newPage()
  observePage(page, label, failures)
  try {
    await page.goto(`${BASE_URL}/accounts`, {
      waitUntil: 'domcontentloaded',
      timeout: ROUTE_TIMEOUT_MS,
    })
    await page.waitForTimeout(300)
    await assertRendererResponsive(page, label)

    const addButton = page.getByRole('button', { name: /add account/i })
    await addButton.waitFor({ state: 'visible', timeout: 5_000 })
    if (!(await addButton.isEnabled())) {
      throw new Error(`${label}: Add Account stayed disabled after site seed`)
    }
    await addButton.click({ timeout: 5_000 })
    await assertRendererResponsive(page, `${label} after open`)

    const tabs = page.getByRole('tab')
    if ((await tabs.count()) !== 3) {
      throw new Error(
        `${label}: expected 3 credential tabs, got ${await tabs.count()}`
      )
    }
    await page.getByRole('tab', { name: /password/i }).click({ timeout: 5_000 })
    await page.locator('input[type="password"]').waitFor({
      state: 'visible',
      timeout: 5_000,
    })
    await assertRendererResponsive(page, `${label} password mode`)
    await assertNoFatalUi(page, label)
  } catch (error) {
    failures.push(
      error instanceof Error ? error.message : `${label}: ${String(error)}`
    )
  } finally {
    await page.close().catch(() => {})
  }
}

const browser = await chromium.launch({ headless: true })
const failures = []

try {
  const desktop = await browser.newContext({
    viewport: { width: 1440, height: 900 },
  })
  await seedAuth(desktop)
  await seedInteractionData(desktop.request)
  for (const path of DESKTOP_ROUTES) {
    await scanRoute(desktop, path, 'desktop', failures)
  }
  await exerciseAccounts(desktop, failures)
  await desktop.close()

  const mobile = await browser.newContext({
    viewport: { width: 390, height: 844 },
    isMobile: true,
  })
  await seedAuth(mobile)
  for (const path of MOBILE_ROUTES) {
    await scanRoute(mobile, path, 'mobile', failures)
  }
  await mobile.close()
} finally {
  await browser.close()
}

if (failures.length > 0) {
  console.error(`[ui-smoke] ${failures.length} failure(s):`)
  for (const failure of failures) console.error(`  ${failure}`)
  process.exit(1)
}

console.log(
  `[ui-smoke] clean — ${DESKTOP_ROUTES.length} desktop routes + ${MOBILE_ROUTES.length} mobile routes + accounts interaction.`
)
