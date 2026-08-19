#!/usr/bin/env node
// Probe the sites dialog buttons (tri-state selects display value check).
import { chromium } from 'playwright'

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
await page.goto(`${BASE_URL}/sites`, {
  waitUntil: 'domcontentloaded',
  timeout: 20000,
})
await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {})
await page
  .getByRole('button', { name: /添加站点/ })
  .first()
  .click()
await page.waitForTimeout(800)
const buttons = await page.evaluate(() =>
  [
    ...document.querySelectorAll(
      '[data-slot="dialog-content"] button, [role="dialog"] button'
    ),
  ].map((b) => ({
    text: b.textContent.trim().slice(0, 30),
    role: b.getAttribute('role'),
  }))
)
for (const b of buttons) console.log(JSON.stringify(b))
await browser.close()
