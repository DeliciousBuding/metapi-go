#!/usr/bin/env node
// R2-2 lane Domain-B probe: dump table cells (and console/page errors) for
// each list page against the seeded extreme-data DB. White-screen signal =
// page errors / empty root.
import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4100'
const routes = (
  process.env.ROUTES ?? '/proxy-logs,/accounts,/sites,/checkin'
).split(',')

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})

for (const route of routes) {
  const context = await browser.newContext({
    viewport: { width: 1600, height: 1000 },
    locale: 'zh-CN',
  })
  await context.addInitScript(() => {
    localStorage.setItem('auth_token', 'dev-admin-token-123')
    localStorage.setItem(
      'auth_token_expires_at',
      String(Date.now() + 12 * 3600 * 1000)
    )
    localStorage.setItem('i18nextLng', 'zh-CN')
  })
  const page = await context.newPage()
  const consoleErrors = []
  const pageErrors = []
  page.on('console', (message) => {
    if (message.type() === 'error') consoleErrors.push(message.text())
  })
  page.on('pageerror', (error) => pageErrors.push(String(error)))
  await page
    .goto(`${BASE_URL}${route}`, {
      waitUntil: 'domcontentloaded',
      timeout: 20000,
    })
    .catch((error) => console.log('goto error:', String(error).slice(0, 120)))
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
  await page.waitForTimeout(2500)
  const dump = await page.evaluate(() => {
    const headers = [...document.querySelectorAll('thead th')].map((th) =>
      (th.textContent ?? '').trim().replace(/\s+/g, ' ')
    )
    const rows = [...document.querySelectorAll('tbody tr')]
      .slice(0, 20)
      .map((tr) =>
        [...tr.querySelectorAll('td')].map((td) =>
          (td.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 60)
        )
      )
    return {
      headers,
      rows,
      bodyLen: document.body.innerText.length,
      rootEmpty:
        (document.getElementById('root')?.childElementCount ?? 0) === 0,
    }
  })
  console.log(`=== ${route}`)
  console.log('headers:', JSON.stringify(dump.headers))
  dump.rows.forEach((row, index) =>
    console.log(`row${index}:`, JSON.stringify(row))
  )
  console.log(
    'health:',
    JSON.stringify({
      rootEmpty: dump.rootEmpty,
      bodyLen: dump.bodyLen,
      consoleErrors: consoleErrors.slice(0, 3),
      pageErrors: pageErrors.slice(0, 3),
    })
  )
  await page
    .screenshot({
      path: `/tmp/probe-ab-cells-${route.replaceAll('/', '_')}.png`,
    })
    .catch(() => {})
  await context.close()
}

await browser.close()
