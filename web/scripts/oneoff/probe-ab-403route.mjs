#!/usr/bin/env node
// One-off variant of the auth probe: valid token in storage, but the network
// layer force-returns 403 {"error":"Invalid token"} for /api/accounts via a
// Playwright route intercept — simulates "token rotated on another device".
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
  if (!sessionStorage.getItem('ab_probe_seeded')) {
    sessionStorage.setItem('ab_probe_seeded', '1')
    localStorage.setItem('auth_token', 'dev-admin-token-123')
    localStorage.setItem(
      'auth_token_expires_at',
      String(Date.now() + 12 * 3600 * 1000)
    )
  }
  localStorage.setItem('i18nextLng', 'zh-CN')
})
const page = await context.newPage()
let intercepted = 0
await context.route('**/api/accounts*', async (route) => {
  intercepted += 1
  await route.fulfill({
    status: 403,
    contentType: 'application/json',
    body: JSON.stringify({ error: 'Invalid token' }),
  })
})
const requestLog = []
const startedAt = Date.now()
page.on('request', (request) => {
  if (request.url().includes('/api/accounts'))
    requestLog.push(Date.now() - startedAt)
})
await page.goto(`${BASE_URL}/accounts`, {
  waitUntil: 'domcontentloaded',
  timeout: 20000,
})
const pageErrors = []
page.on('pageerror', (error) => pageErrors.push(String(error)))
const storageTimeline = []
const pollTimer = setInterval(() => {
  page
    .evaluate(() => ({
      url: location.pathname,
      token: localStorage.getItem('auth_token') ? 'Y' : 'N',
    }))
    .then((snapshot) =>
      storageTimeline.push(
        `t=${Date.now() - startedAt} ${snapshot.url} tok=${snapshot.token}`
      )
    )
    .catch(() => storageTimeline.push(`t=${Date.now() - startedAt} <nav>`))
}, 250)
await page.waitForTimeout(8000)
clearInterval(pollTimer)
console.log('storage timeline:')
console.log(storageTimeline.join('\n'))
const finalUrl = page.url()
const storageToken = await page
  .evaluate(() => localStorage.getItem('auth_token'))
  .catch(() => '<nav>')
const bodyText = await page
  .evaluate(() => document.body.innerText.slice(0, 300))
  .catch(() => '<nav>')
console.log('intercepted responses served:', intercepted)
console.log('request offsets:', JSON.stringify(requestLog))
console.log('final location:', finalUrl)
console.log('storage token after:', storageToken)
console.log('body:', bodyText.replaceAll('\n', ' | '))
console.log('page errors:', pageErrors.length, pageErrors.slice(0, 3))
await browser.close()
