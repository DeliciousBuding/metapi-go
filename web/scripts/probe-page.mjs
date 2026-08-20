#!/usr/bin/env node
// Quick probe: dump one table page's row count, visible buttons, body text
// and console errors against a running server (real-e2e audit helper).
// Usage: bun run ui:probe-page <route>   e.g. bun run ui:probe-page /accounts
import { chromium } from 'playwright'

const route = process.argv[2] ?? '/accounts'
const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})
const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  locale: 'zh-CN',
})
await context.addInitScript(
  ({ token }) => {
    localStorage.setItem('auth_token', token)
    localStorage.setItem(
      'auth_token_expires_at',
      String(Date.now() + 12 * 3600 * 1000)
    )
    localStorage.setItem('i18nextLng', 'zh-CN')
  },
  { token: 'dev-admin-token-123' }
)
const page = await context.newPage()
const errors = []
page.on('console', (m) => {
  if (m.type() === 'error') errors.push(m.text())
})
await page.goto(`${BASE_URL}${route}`, {
  waitUntil: 'domcontentloaded',
  timeout: 20000,
})
await page.waitForLoadState('networkidle', { timeout: 6000 }).catch(() => {})
await page.waitForTimeout(1200)

const info = await page.evaluate(() => {
  const rows = document.querySelectorAll('tbody tr').length
  const buttons = [...document.querySelectorAll('button')]
    .map((b) => ({
      label: b.getAttribute('aria-label') || b.textContent.trim().slice(0, 30),
    }))
    .filter((b) => b.label)
    .slice(0, 30)
  const bodyText = document.body.innerText.slice(0, 600)
  return { rows, buttons, bodyText }
})
console.log('route:', route)
console.log('rows:', info.rows)
console.log('buttons:', JSON.stringify(info.buttons))
console.log('body text:', info.bodyText.replaceAll('\n', ' | '))
console.log('console errors:', errors.length, errors.slice(0, 3))
await browser.close()
