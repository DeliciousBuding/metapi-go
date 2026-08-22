#!/usr/bin/env node
// One-off: after the offline-navigation failure, locate WHERE the lingering
// skeleton elements sit in the DOM (parent chain), plus a screenshot.
import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4100'
const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})
const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
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
await page.goto(`${BASE_URL}/accounts`, {
  waitUntil: 'domcontentloaded',
  timeout: 20000,
})
await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
await context.route('**/api/**', (route) => route.abort('internetdisconnected'))
await page.locator('a[href="/sites"]').first().click()
await page.waitForTimeout(15000)
const skeletonInfo = await page.evaluate(() => {
  const nodes = [
    ...document.querySelectorAll('[data-slot="skeleton"], .animate-pulse'),
  ]
  return nodes.map((node) => {
    const chain = []
    let current = node
    for (let depth = 0; depth < 5 && current; depth += 1) {
      chain.push(
        `${current.tagName.toLowerCase()}.${(current.className ?? '')
          .toString()
          .split(' ')
          .slice(0, 3)
          .join('.')}`
      )
      current = current.parentElement
    }
    const rect = node.getBoundingClientRect()
    return {
      chain: chain.join(' < '),
      visible: rect.width > 0 && rect.height > 0,
    }
  })
})
console.log(JSON.stringify(skeletonInfo, null, 1))
await page.screenshot({ path: '/tmp/probe-ab-offline.png', fullPage: false })
await browser.close()
