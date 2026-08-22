#!/usr/bin/env node
// R2-2 lane empirical probe (Domain A: browser runtime behavior).
//
// Modes:
//   pages   — visit each list page, count per-URL /api requests (silent
//             retry detection), collect console errors, report table rows
//             and lingering skeleton markers.
//   auth    — drive 401 / 403 scenarios with a WRONG token and report the
//             resulting location + visible toast text.
//   ws      — watch the realtime ops reconnect loop after the server-side
//             WebSocket endpoint goes quiet (regression of the Round-2
//             MAX_FAILS "give up forever" fix).
//
// Usage:
//   BASE_URL=http://127.0.0.1:4100 AUTH_TOKEN=probe-token \
//     node scripts/probe-ab-runtime.mjs pages
import { chromium } from 'playwright'

const mode = process.argv[2] ?? 'pages'
const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})

async function newPageWithAuth(token) {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: 'zh-CN',
  })
  await context.addInitScript(
    ({ authToken }) => {
      // Seed the (wrong) token ONCE per tab; sessionStorage survives
      // navigations but not tab closes, so the redirect to /sign-in is not
      // sabotaged by re-injection after clearAuthentication() runs.
      if (!sessionStorage.getItem('ab_probe_seeded')) {
        sessionStorage.setItem('ab_probe_seeded', '1')
        localStorage.setItem('auth_token', authToken)
        localStorage.setItem(
          'auth_token_expires_at',
          String(Date.now() + 12 * 3600 * 1000)
        )
      }
      localStorage.setItem('i18nextLng', 'zh-CN')
    },
    { authToken: token }
  )
  const page = await context.newPage()
  const consoleErrors = []
  const pageErrors = []
  page.on('console', (message) => {
    if (message.type() === 'error') consoleErrors.push(message.text())
  })
  page.on('pageerror', (error) => pageErrors.push(String(error)))
  return { context, page, consoleErrors, pageErrors }
}

function summarizeRequests(requestLog) {
  const counts = new Map()
  for (const entry of requestLog) {
    const parsed = new URL(entry.url)
    const key = `${parsed.pathname}${parsed.search}`
    if (!counts.has(key)) counts.set(key, { count: 0, atMs: [] })
    const bucket = counts.get(key)
    bucket.count += 1
    bucket.atMs.push(entry.atMs)
  }
  return [...counts.entries()]
    .map(([key, bucket]) => [key, bucket.count, bucket.atMs])
    .sort((a, b) => b[1] - a[1])
}

if (mode === 'pages') {
  const routes = (process.env.ROUTES ?? '/accounts,/sites,/channels,/proxy-logs,/checkin').split(',')
  for (const route of routes) {
    const { context, page, consoleErrors, pageErrors } =
      await newPageWithAuth(AUTH_TOKEN)
    if (process.env.STACKS === '1') {
      await page.addInitScript(() => {
        const originalOpen = XMLHttpRequest.prototype.open
        XMLHttpRequest.prototype.open = function (method, url, ...rest) {
          if (String(url).includes('/api/accounts')) {
            console.log(`[xstack] ${method} ${url}\n${new Error().stack}`)
          }
          return originalOpen.call(this, method, url, ...rest)
        }
      })
      page.on('console', (message) => {
        const text = message.text()
        if (text.startsWith('[xstack]')) console.log(text.slice(0, 900))
      })
    }
    const requestLog = []
    const navStartAt = Date.now()
    page.on('request', (request) => {
      if (request.url().includes('/api/'))
        requestLog.push({ url: request.url(), atMs: Date.now() - navStartAt })
    })
    const gotoError = await page
      .goto(`${BASE_URL}${route}`, {
        waitUntil: 'domcontentloaded',
        timeout: 20000,
      })
      .then(() => null)
      .catch((error) => String(error))
    if (gotoError) {
      console.log('=== route:', route)
      console.log('goto failed (server unreachable?):', gotoError.slice(0, 160))
      await context.close()
      continue
    }
    await page
      .waitForLoadState('networkidle', { timeout: 8000 })
      .catch(() => {})
    // Give react-query retries (if any) room to fire: prod retry backoff is
    // exponential (1s/2s/4s), so 9s covers 3 retries with margin.
    await page.waitForTimeout(9000)
    const info = await page.evaluate(() => {
      const rows = document.querySelectorAll('tbody tr').length
      const skeletons = document.querySelectorAll(
        '[data-slot="skeleton"], .animate-pulse'
      ).length
      const bodyText = document.body.innerText.slice(0, 400)
      return { rows, skeletons, bodyText }
    })
    console.log('=== route:', route)
    console.log('api request counts:', JSON.stringify(summarizeRequests(requestLog)))
    console.log('rows:', info.rows, '| skeletons still visible:', info.skeletons)
    console.log(
      'console errors:',
      consoleErrors.length,
      consoleErrors.slice(0, 3)
    )
    console.log('page errors:', pageErrors.length, pageErrors.slice(0, 2))
    console.log('body:', info.bodyText.replaceAll('\n', ' | ').slice(0, 300))
    await context.close()
  }
} else if (mode === 'auth') {
  // Wrong token → backend answers 401. Expect: toast + redirect to /sign-in,
  // no infinite retry.
  const { context, page, consoleErrors, pageErrors } =
    await newPageWithAuth('definitely-wrong-token')
  if (process.env.STACKS === '1') {
    await page.addInitScript(() => {
      const originalOpen = XMLHttpRequest.prototype.open
      XMLHttpRequest.prototype.open = function (method, url, ...rest) {
        if (String(url).includes('/api/')) {
          console.log(`[xstack] ${method} ${url}\n${new Error().stack}`)
        }
        return originalOpen.call(this, method, url, ...rest)
      }
    })
    page.on('console', (message) => {
      const text = message.text()
      if (text.startsWith('[xstack]')) console.log(text.slice(0, 1400))
    })
  }
  const requestLog = []
  const authNavStartAt = Date.now()
  page.on('request', (request) => {
    if (request.url().includes('/api/'))
      requestLog.push({
        url: request.url(),
        atMs: Date.now() - authNavStartAt,
      })
  })
  await page.goto(`${BASE_URL}/accounts`, {
    waitUntil: 'domcontentloaded',
    timeout: 20000,
  })
  await page.waitForTimeout(6000)
  const authBodyText = await page
    .evaluate(() => document.body.innerText.slice(0, 500))
    .catch(() => '<evaluate failed>')
  await page
    .screenshot({ path: '/tmp/probe-ab-auth.png' })
    .catch(() => {})
  const toastText = await page
    .evaluate(() => {
      const toasts = [...document.querySelectorAll('[data-sonner-toast]')]
      return toasts.map((toast) => toast.textContent).join(' || ')
    })
    .catch(() => '<evaluate failed>')
  console.log('=== auth 401 scenario')
  console.log('final location:', page.url())
  console.log('toast text:', toastText)
  console.log('body text:', authBodyText.replaceAll('\n', ' | ').slice(0, 400))
  console.log('api request counts:', JSON.stringify(summarizeRequests(requestLog)))
  console.log('console errors:', consoleErrors.length, consoleErrors.slice(0, 3))
  console.log('page errors:', pageErrors.length, pageErrors.slice(0, 2))
  await context.close()
} else if (mode === 'ws') {
  // Observe the dashboard realtime-ops socket across a server restart gap:
  // the hook must keep retrying (slow cadence after MAX_FAILS), never stop.
  const { context, page } = await newPageWithAuth(AUTH_TOKEN)
  const wsEvents = []
  page.on('websocket', (websocket) => {
    wsEvents.push(`open ${websocket.url()} @ ${new Date().toISOString()}`)
    websocket.on('close', () =>
      wsEvents.push(`close @ ${new Date().toISOString()}`)
    )
  })
  await page.goto(`${BASE_URL}/dashboard/overview`, {
    waitUntil: 'domcontentloaded',
    timeout: 20000,
  })
  const watchSeconds = Number(process.env.WS_WATCH_SECONDS ?? '40')
  await page.waitForTimeout(watchSeconds * 1000)
  console.log('=== ws events (', watchSeconds, 's window)')
  for (const event of wsEvents) console.log(event)
  await context.close()
} else if (mode === 'offline') {
  // Load normally, then abort every /api request (simulated network loss)
  // and SPA-navigate to another list page: the loader/page query must fail
  // visibly (error banner) after bounded retries — never an endless skeleton.
  const { context, page, consoleErrors, pageErrors } =
    await newPageWithAuth(AUTH_TOKEN)
  const requestLog = []
  const offlineNavStartAt = Date.now()
  page.on('request', (request) => {
    if (request.url().includes('/api/'))
      requestLog.push({
        url: request.url(),
        atMs: Date.now() - offlineNavStartAt,
      })
  })
  await page.goto(`${BASE_URL}/accounts`, {
    waitUntil: 'domcontentloaded',
    timeout: 20000,
  })
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
  await context.route('**/api/**', (route) => route.abort('internetdisconnected'))
  requestLog.length = 0
  const sitesNavLink = page.locator('a[href="/sites"]').first()
  await sitesNavLink.click()
  await page.waitForTimeout(15000)
  const info = await page.evaluate(() => {
    const skeletons = document.querySelectorAll(
      '[data-slot="skeleton"], .animate-pulse'
    ).length
    return { skeletons, bodyText: document.body.innerText.slice(0, 500) }
  })
  console.log('=== offline navigation scenario')
  console.log('api request counts:', JSON.stringify(summarizeRequests(requestLog)))
  console.log('skeletons still visible:', info.skeletons)
  console.log('body:', info.bodyText.replaceAll('\n', ' | ').slice(0, 350))
  console.log('console errors:', consoleErrors.length, consoleErrors.slice(0, 3))
  console.log('page errors:', pageErrors.length, pageErrors.slice(0, 2))
  await context.close()
} else if (mode === 'resp') {
  // Capture the exact response bodies the page sees for /api/accounts —
  // distinguishes "server sent null accounts" from "client mishandled it".
  const route = process.argv[3] ?? '/accounts'
  const { context, page } = await newPageWithAuth(AUTH_TOKEN)
  page.on('response', async (response) => {
    if (response.url().includes('/api/accounts')) {
      const body = await response.text().catch(() => '<unreadable>')
      console.log(
        `[resp] ${response.status()} ${response.url()} :: ${body.slice(0, 220)}`
      )
    }
  })
  await page.goto(`${BASE_URL}${route}`, {
    waitUntil: 'domcontentloaded',
    timeout: 20000,
  })
  await page.waitForTimeout(11000)
  await context.close()
} else {
  console.error('unknown mode:', mode)
  process.exit(2)
}

await browser.close()
