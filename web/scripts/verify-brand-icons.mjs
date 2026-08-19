#!/usr/bin/env node
// Verify brand icons render from local assets (no CSP blocks, naturalWidth > 0).
import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})
const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
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
  { token: AUTH_TOKEN }
)
const page = await context.newPage()
const consoleErrors = []
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text())
})

await page.goto(`${BASE_URL}/models`, {
  waitUntil: 'domcontentloaded',
  timeout: 20000,
})
await page.waitForLoadState('networkidle', { timeout: 6000 }).catch(() => {})
await page.waitForTimeout(1200)

const imgs = await page.evaluate(() =>
  [...document.querySelectorAll('img')].map((img) => ({
    src: img.src.slice(0, 90),
    naturalWidth: img.naturalWidth,
    alt: img.alt?.slice(0, 30),
  }))
)
console.log('img elements:', imgs.length)
for (const img of imgs.slice(0, 15))
  console.log(`  w=${img.naturalWidth} src=${img.src}`)
const localIcons = imgs.filter((i) => i.src.includes('/static/image/'))
const loadedIcons = localIcons.filter((i) => i.naturalWidth > 0)
console.log(
  `local icon imgs: ${localIcons.length}, loaded: ${loadedIcons.length}`
)
console.log(`console errors: ${consoleErrors.length}`)
for (const e of consoleErrors.slice(0, 5)) console.log(`  ${e.slice(0, 120)}`)

await browser.close()
