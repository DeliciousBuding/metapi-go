#!/usr/bin/env node
// metapi-go — production-SPA crash / route / interaction smoke gate.
//
// This complements unit tests and axe: it loads the actual built SPA in a real
// Chromium instance and fails on renderer exceptions, console errors, 5xx
// responses, route-level error UI, or document-wide mobile overflow. It also
// exercises the account-creation credential tabs because that flow previously
// admitted a render feedback loop that unit/schema tests did not catch.

import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://localhost:4000'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'

const DESKTOP_ROUTES = [
  '/',
  '/dashboard',
  '/dashboard/overview',
  '/dashboard/traffic',
  '/dashboard/models',
  '/dashboard/availability',
  '/sites',
  '/accounts',
  '/checkin',
  '/models',
  '/model-tester',
  '/oauth',
  '/channels',
  '/token-routes',
  '/proxy-logs',
  '/observability',
  '/price-compare',
  '/site-announcements',
  '/fix-candidates',
  '/about',
  '/settings',
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

// Main product workspaces get a second pass at the 375px mobile contract.
const MOBILE_ROUTES = [
  '/',
  '/sites',
  '/accounts',
  '/checkin',
  '/models',
  '/model-tester',
  '/oauth',
  '/channels',
  '/token-routes',
  '/proxy-logs',
  '/observability',
  '/price-compare',
  '/site-announcements',
  '/settings',
]

const ERROR_TEXT =
  /(服务器错误|服务器内部错误|internal server error|something went wrong|application error|chunkloaderror)/i

async function seedAuth(context) {
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
}

async function seedInteractionData(request) {
  const response = await request.post(`${BASE_URL}/api/sites`, {
    headers: {
      Authorization: `Bearer ${AUTH_TOKEN}`,
      'Content-Type': 'application/json',
    },
    data: {
      name: 'Browser Smoke Site',
      url: 'https://example.invalid',
      platform: 'new-api',
    },
  })
  if (![200, 201, 409].includes(response.status())) {
    throw new Error(
      `failed to seed browser-smoke site: HTTP ${response.status()} ${await response.text()}`
    )
  }
}

function collectPageFailures(page, failures, label) {
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
      failures.push(
        `${label}: HTTP ${response.status()} — ${response.request().method()} ${response.url()}`
      )
    }
  })
}

async function scanRoute(context, route, failures, mobile = false) {
  const label = `${mobile ? 'mobile' : 'desktop'} ${route}`
  const page = await context.newPage()
  collectPageFailures(page, failures, label)
  try {
    const response = await page.goto(BASE_URL + route, {
      waitUntil: 'domcontentloaded',
      timeout: 15_000,
    })
    await page.waitForTimeout(500)

    // Scroll through the page before collecting console errors: lazy-loaded
    // content below the fold (e.g. brand icons) only fires its request when
    // it approaches the viewport, so a CSP-blocked lazy image would stay
    // silent without this pass (regression for the brand-icon CSP fix).
    await page.evaluate(async () => {
      const step = Math.max(1, Math.floor(window.innerHeight * 0.8))
      const total = document.body.scrollHeight
      for (let y = 0; y <= total; y += step) {
        window.scrollTo(0, y)
        await new Promise((resolve) => setTimeout(resolve, 30))
      }
      window.scrollTo(0, 0)
    })
    await page.waitForTimeout(300)

    if (response && response.status() >= 500) {
      failures.push(`${label}: document returned HTTP ${response.status()}`)
    }

    const body = await page.locator('body').innerText({ timeout: 4_000 })
    const errorText = body.match(ERROR_TEXT)?.[0]
    if (errorText) failures.push(`${label}: rendered error UI — ${errorText}`)

    // Renderer liveness check: this times out if a render/effect loop wedges the
    // main thread after initial navigation.
    await page.locator('body').getAttribute('class', { timeout: 3_000 })

    if (mobile) {
      const overflow = await page.evaluate(() => ({
        viewport: document.documentElement.clientWidth,
        document: document.documentElement.scrollWidth,
      }))
      // One CSS pixel of tolerance avoids fractional-rounding false positives.
      if (overflow.document > overflow.viewport + 1) {
        failures.push(
          `${label}: document horizontal overflow ${overflow.document}px > ${overflow.viewport}px`
        )
      }
    }
  } catch (error) {
    failures.push(
      `${label}: smoke exception — ${String(error?.message ?? error).split('\n')[0]}`
    )
  } finally {
    await page.close().catch(() => {})
  }
}

async function exerciseUrlOwnedPage(context, route, failures) {
  const label = `${route} url-state interaction`
  const page = await context.newPage()
  collectPageFailures(page, failures, label)
  try {
    await page.goto(`${BASE_URL}${route}`, {
      waitUntil: 'domcontentloaded',
      timeout: 15_000,
    })
    await page.waitForTimeout(500)

    // The toolbar's global search is the only input carrying a placeholder
    // (date inputs are `datetime-local` with aria-labels, and the account/
    // client selects expose their own unlabeled hidden input), so it is
    // unambiguous here.
    const search = page.locator('input[placeholder]').first()
    await search.waitFor({ state: 'visible', timeout: 5_000 })
    await search.fill('smokeq')

    // The URL-owned hook navigates once after the toolbar debounce; this wait
    // is the regression assertion for the old local-state + effect write-back
    // loop (which either froze the renderer or never settled the URL).
    await page.waitForFunction(
      () => new URLSearchParams(window.location.search).get('q') === 'smokeq',
      undefined,
      { timeout: 5_000 }
    )
    await page.locator('body').getAttribute('class', { timeout: 3_000 })

    if ((await search.inputValue()) !== 'smokeq') {
      failures.push(
        `${label}: search input lost the committed query after URL round-trip`
      )
    }

    // Navigating away must not be hijacked by a stale table callback.
    await page.goto(`${BASE_URL}/dashboard`, {
      waitUntil: 'domcontentloaded',
      timeout: 15_000,
    })
    await page.waitForTimeout(500)
    if (!page.url().startsWith(`${BASE_URL}/dashboard`)) {
      failures.push(`${label}: navigation away was hijacked (${page.url()})`)
    }
    await page.locator('body').getAttribute('class', { timeout: 3_000 })
  } catch (error) {
    failures.push(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await page.close().catch(() => {})
  }
}

async function exerciseAccounts(context, failures) {
  const label = 'accounts interaction'
  const page = await context.newPage()
  collectPageFailures(page, failures, label)
  try {
    await page.goto(`${BASE_URL}/accounts`, {
      waitUntil: 'domcontentloaded',
      timeout: 15_000,
    })
    await page.waitForTimeout(500)

    const add = page
      .getByRole('button', { name: /添加账号|Add account|Add/i })
      .first()
    await add.waitFor({ state: 'visible', timeout: 5_000 })
    if (!(await add.isEnabled())) {
      failures.push(
        `${label}: add-account button unexpectedly disabled after site seed`
      )
      return
    }

    await add.click({ timeout: 5_000 })
    const tabs = page.getByRole('tab')
    if ((await tabs.count()) < 3) {
      failures.push(`${label}: expected 3 credential-mode tabs`)
      return
    }

    const password = page.getByRole('tab', { name: /密码|Password/i }).first()
    await password.click({ timeout: 5_000 })
    await page.waitForTimeout(200)

    // A post-click DOM read is the regression assertion for the historical
    // renderer freeze: it must complete promptly after the mode switch.
    const body = await page.locator('body').innerText({ timeout: 3_000 })
    if (!/密码|Password/i.test(body)) {
      failures.push(`${label}: password credential mode did not render`)
    }
  } catch (error) {
    failures.push(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await page.close().catch(() => {})
  }
}

const browser = await chromium.launch({ headless: true })
const failures = []
try {
  const desktop = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: 'zh-CN',
  })
  await seedAuth(desktop)
  await seedInteractionData(desktop.request)
  for (const route of DESKTOP_ROUTES) {
    await scanRoute(desktop, route, failures)
  }
  await exerciseAccounts(desktop, failures)
  for (const route of ['/checkin', '/proxy-logs', '/token-routes']) {
    await exerciseUrlOwnedPage(desktop, route, failures)
  }
  await desktop.close()

  const mobile = await browser.newContext({
    viewport: { width: 375, height: 812 },
    locale: 'zh-CN',
    isMobile: true,
  })
  await seedAuth(mobile)
  for (const route of MOBILE_ROUTES) {
    await scanRoute(mobile, route, failures, true)
  }
  await mobile.close()
} finally {
  await browser.close()
}

if (failures.length > 0) {
  console.error(`[ui-smoke] ${failures.length} failure(s):`)
  for (const failure of [...new Set(failures)]) console.error(`  ${failure}`)
  process.exit(1)
}

console.log(
  `[ui-smoke] clean — ${DESKTOP_ROUTES.length} desktop routes + ${MOBILE_ROUTES.length} mobile routes + accounts interaction + checkin/proxy-logs/token-routes url-state interactions.`
)
