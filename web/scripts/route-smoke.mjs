#!/usr/bin/env node
// metapi-go — production-SPA crash / route / interaction smoke gate.
//
// This complements unit tests and axe: it loads the actual built SPA in a real
// Chromium instance and fails on renderer exceptions, console errors, 5xx
// responses, route-level error UI, or document-wide mobile overflow. It also
// exercises the account-creation credential tabs because that flow previously
// admitted a render feedback loop that unit/schema tests did not catch.

import { pathToFileURL } from 'node:url'

import { chromium } from 'playwright'

import { loginSession } from './session-auth.mjs'

const BASE_URL = process.env.BASE_URL ?? 'http://localhost:4000'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const CURRENT_NAV_ROUTES = {
  '/dashboard/overview': '/',
  '/accounts': '/accounts',
  '/proxy-logs': '/proxy-logs',
}

// Single source of truth for the authenticated desktop route inventory.
// a11y-scan.mjs imports this so the two gates can never drift apart.
export const DESKTOP_ROUTES = [
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
  '/users',
  '/proxy-logs',
  '/audit-logs',
  '/audit-logs?category=access',
  '/observability',
  '/price-compare',
  '/site-announcements',
  '/about',
  '/settings',
  '/settings/basic/site',
  '/settings/basic/authentication',
  '/settings/proxy-models/proxy-transport',
  '/settings/proxy-models/routing',
  '/settings/proxy-models/redirects',
  '/settings/proxy-models/rates',
  '/settings/proxy-models/allowlist',
  '/settings/proxy-models/catalog-sources',
  '/settings/downstream/keys',
  '/settings/downstream/proxy-token',
  '/settings/content/notifications',
  '/settings/content/announcements',
  '/settings/content/import-export',
  '/settings/operations/scheduling',
  '/settings/operations/database',
  '/settings/operations/data-migration',
  '/settings/operations/maintenance',
  '/settings/operations/program-logs',
  '/settings/operations/audit-logs',
  '/settings/operations/update-center',
  '/settings/operations/danger-zone',
]

// Pre-regroup settings URLs (wave 9 lane B): every old path must land on its
// new home through the client-side legacy redirect (no 500 / error UI).
const LEGACY_REDIRECT_ROUTES = [
  ['/settings/general', '/settings/basic/site'],
  ['/settings/general/site', '/settings/basic/site'],
  ['/settings/general/authentication', '/settings/basic/authentication'],
  ['/settings/general/scheduling', '/settings/operations/scheduling'],
  [
    '/settings/general/proxy-transport',
    '/settings/proxy-models/proxy-transport',
  ],
  ['/settings/general/routing', '/settings/proxy-models/routing'],
  ['/settings/models', '/settings/proxy-models/proxy-transport'],
  ['/settings/models/redirects', '/settings/proxy-models/redirects'],
  ['/settings/models/rates', '/settings/proxy-models/rates'],
  ['/settings/models/allowlist', '/settings/proxy-models/allowlist'],
  [
    '/settings/models/catalog-sources',
    '/settings/proxy-models/catalog-sources',
  ],
  ['/settings/system-info', '/settings/operations/program-logs'],
  ['/settings/system-info/program-logs', '/settings/operations/program-logs'],
  ['/settings/system-info/audit-logs', '/audit-logs'],
  ['/settings/system-info/update-center', '/settings/operations/update-center'],
  ['/settings/system-info/database', '/settings/operations/database'],
  [
    '/settings/system-info/data-migration',
    '/settings/operations/data-migration',
  ],
  ['/settings/system-info/maintenance', '/settings/operations/maintenance'],
  ['/settings/system-info/danger-zone', '/settings/operations/danger-zone'],
]

// Main product workspaces get a second pass at the 375px mobile contract.
// Settings carries one landing route per subarea so every one of the five
// regrouped groups is reachable and overflow-free at 375px.
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
  '/users',
  '/proxy-logs',
  '/observability',
  '/price-compare',
  '/site-announcements',
  '/settings',
  '/settings/basic/site',
  '/settings/proxy-models/proxy-transport',
  '/settings/downstream/keys',
  '/settings/content/notifications',
  '/settings/operations/program-logs',
]

const ERROR_TEXT =
  /(服务器错误|服务器内部错误|internal server error|something went wrong|application error|chunkloaderror)/i

async function seedAuth(context, locale = 'zh-CN') {
  await loginSession(context, { baseUrl: BASE_URL, token: AUTH_TOKEN })
  await context.addInitScript((language) => {
    localStorage.setItem('i18nextLng', language)
    document.cookie = 'vite-ui-theme=light; path=/'
  }, locale)
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

/**
 * Legacy settings URL gate (wave 9 lane B): each pre-regroup path must
 * client-side redirect to its new home without error UI. The redirect is
 * thrown from the route `beforeLoad` guard on SPA boot, so the new URL is
 * asserted with a wait-for-function on the pathname.
 */
async function scanLegacyRedirects(context, redirects, failures) {
  for (const [fromUrl, toUrl] of redirects) {
    const toPath = new URL(toUrl, BASE_URL).pathname
    const label = `legacy ${fromUrl} -> ${toPath}`
    const page = await context.newPage()
    collectPageFailures(page, failures, label)
    try {
      await page.goto(BASE_URL + fromUrl, {
        waitUntil: 'domcontentloaded',
        timeout: 15_000,
      })
      await page
        .waitForFunction(
          (pathname) => new URL(location.href).pathname === pathname,
          toPath,
          { timeout: 8_000 }
        )
        .catch(() => {
          failures.push(
            `${label}: did not land on ${toPath} (stayed at ${page.url()})`
          )
        })
      await page.waitForTimeout(400)
      const body = await page.locator('body').innerText({ timeout: 4_000 })
      const errorText = body.match(ERROR_TEXT)?.[0]
      if (errorText) failures.push(`${label}: rendered error UI — ${errorText}`)
    } catch (error) {
      failures.push(
        `${label}: smoke exception — ${String(error?.message ?? error).split('\n')[0]}`
      )
    } finally {
      await page.close().catch(() => {})
    }
  }
}

async function scanRoute(context, route, failures, mobileLocale) {
  const label = `${mobileLocale ? `mobile ${mobileLocale}` : 'desktop'} ${route}`
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

    // The SidebarMenuButton primitive owns data-active and aria-current, but
    // the TanStack Link render bridge must forward both to the real anchor.
    // Previously every page looked unselected despite the URL matcher being
    // correct; a primitive-only unit test could not see that bridge defect.
    const expectedNav = !mobileLocale && CURRENT_NAV_ROUTES[route]
    if (expectedNav) {
      const current = await page.locator(
        '[data-sidebar=menu-button][aria-current=page][data-active]'
      )
      const hrefs = await current.evaluateAll((links) =>
        links.map((link) => link.getAttribute('href'))
      )
      if (hrefs.length !== 1 || hrefs[0] !== expectedNav) {
        failures.push(
          `${label}: current sidebar link ${JSON.stringify(hrefs)}, expected ${expectedNav}`
        )
      }
    }

    if (mobileLocale) {
      await page
        .locator(`html[lang="${mobileLocale}"] #content`)
        .getByRole('heading')
        .first()
        .waitFor({ timeout: 5_000 })
      const overflow = await page.evaluate(() => ({
        viewport: document.documentElement.clientWidth,
        document: document.documentElement.scrollWidth,
      }))
      // One CSS pixel of tolerance avoids fractional-rounding false positives.
      if (
        overflow.viewport !== 375 ||
        overflow.document > overflow.viewport + 1
      ) {
        failures.push(
          `${label}: expected 375px layout, document ${overflow.document}px, viewport ${overflow.viewport}px`
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

async function checkLogAlignment(context, failures) {
  const page = await context.newPage()
  collectPageFailures(page, failures, 'log alignment')
  try {
    const model = 'gpt-4o'
    await page.route(/\/api\/stats\/proxy-logs\?/, (route) => {
      if (new URL(route.request().url()).searchParams.get('view') !== 'query')
        return route.continue()
      return route.fulfill({
        json: {
          items: [
            {
              id: 1,
              createdAt: '2026-01-01T00:00:00Z',
              modelRequested: model,
              modelActual: model,
              status: 'success',
              httpStatus: 200,
              latencyMs: 16,
              totalTokens: 2,
              retryCount: 0,
            },
          ],
          total: 1,
          page: 1,
          pageSize: 20,
        },
      })
    })
    await page.goto(`${BASE_URL}/proxy-logs?view=attempts`)
    const row = page
      .getByRole('row')
      .filter({ has: page.getByTitle(model, { exact: true }) })
    const pill = row.getByTitle(model, { exact: true })
    const status = row.getByText('200', { exact: true })
    for (const [name, badge] of [
      ['model', pill],
      ['status', status],
    ]) {
      await badge.waitFor({ timeout: 5_000 })
      const offset = await badge.evaluate(async (element, name) => {
        await document.fonts.ready
        const icon = element.querySelector(
          name === 'model'
            ? ':scope > [aria-hidden] > :first-child'
            : ':scope > [aria-hidden]'
        )
        const label = element.lastChild
        if (!icon || !label?.textContent?.trim())
          throw new Error(`missing ${name} icon or label`)
        const range = document.createRange()
        range.selectNodeContents(label)
        const text =
          label.nodeType === Node.TEXT_NODE
            ? range.getBoundingClientRect()
            : label.getBoundingClientRect()
        const box = icon.getBoundingClientRect()
        if (!box.height || !text.height)
          throw new Error(`empty ${name} icon or label`)
        return Math.abs(box.y + box.height / 2 - (text.y + text.height / 2))
      }, name)
      // Allow font metrics and subpixel rounding while catching a shifted glyph.
      if (offset > 2)
        failures.push(`log ${name}: vertical center offset ${offset}px > 2px`)
    }
  } finally {
    await page.close()
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

// Executable entrypoint. Guarded so importers (a11y-scan.mjs) can pull
// DESKTOP_ROUTES without launching the smoke sweep as a side effect.
async function main() {
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
    await scanLegacyRedirects(desktop, LEGACY_REDIRECT_ROUTES, failures)
    await checkLogAlignment(desktop, failures)
    await exerciseAccounts(desktop, failures)
    for (const route of ['/checkin', '/proxy-logs', '/token-routes']) {
      await exerciseUrlOwnedPage(desktop, route, failures)
    }
    await desktop.close()

    for (const locale of ['zh-CN', 'en']) {
      const mobile = await browser.newContext({
        viewport: { width: 375, height: 812 },
        locale,
        isMobile: true,
      })
      await seedAuth(mobile, locale)
      for (const route of MOBILE_ROUTES) {
        await scanRoute(mobile, route, failures, locale)
      }
      await mobile.close()
    }
  } finally {
    await browser.close()
  }

  if (failures.length > 0) {
    console.error(`[ui-smoke] ${failures.length} failure(s):`)
    for (const failure of new Set(failures)) console.error(`  ${failure}`)
    process.exit(1)
  }

  console.log(
    `[ui-smoke] clean — ${DESKTOP_ROUTES.length} desktop routes + ${MOBILE_ROUTES.length} mobile routes per locale (zh-CN, en) + log icon alignment + accounts interaction + checkin/proxy-logs/token-routes url-state interactions.`
  )
}

const isMainModule =
  process.argv[1] !== undefined &&
  import.meta.url === pathToFileURL(process.argv[1]).href
if (isMainModule) await main()
