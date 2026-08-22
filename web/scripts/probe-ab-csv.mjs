#!/usr/bin/env node
// R2-2 lane Domain-B CSV probe: click the real export buttons and capture
// the downloaded CSV bytes — adjudicates CSV-injection clue ④ against the
// seeded hostile cells (=1+1 / -cmd / +x / @SUM / quotes / newlines).
import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4100'
const target = process.argv[2] ?? 'proxy-logs'

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})
const context = await browser.newContext({
  viewport: { width: 1600, height: 1000 },
  locale: 'zh-CN',
  acceptDownloads: true,
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

if (target === 'proxy-logs') {
  await page.goto(`${BASE_URL}/proxy-logs`, {
    waitUntil: 'domcontentloaded',
    timeout: 20000,
  })
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
  await page.waitForTimeout(1500)
  const exportButton = page.locator('button', { hasText: '导出 CSV' }).first()
  const [download] = await Promise.all([
    page.waitForEvent('download', { timeout: 15000 }),
    exportButton.click(),
  ])
  const saveTo = '/tmp/probe-ab-export-proxy-logs.csv'
  await download.saveAs(saveTo)
  console.log(
    'saved:',
    saveTo,
    'suggestedFilename:',
    download.suggestedFilename()
  )
} else {
  await page.goto(`${BASE_URL}/settings/system-info/program-logs`, {
    waitUntil: 'domcontentloaded',
    timeout: 20000,
  })
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
  await page.waitForTimeout(1500)
  console.log(
    'page title text:',
    (
      await page.evaluate(() => document.body.innerText.slice(0, 200))
    ).replaceAll('\n', ' | ')
  )
  const exportButton = page
    .locator('button', { hasText: /导出|Export/ })
    .first()
  const [download] = await Promise.all([
    page.waitForEvent('download', { timeout: 15000 }),
    exportButton.click(),
  ])
  const saveTo = '/tmp/probe-ab-export-events.csv'
  await download.saveAs(saveTo)
  console.log(
    'saved:',
    saveTo,
    'suggestedFilename:',
    download.suggestedFilename()
  )
}

await browser.close()
