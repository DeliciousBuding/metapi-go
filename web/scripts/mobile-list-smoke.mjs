#!/usr/bin/env node
// Seeded, real-browser mobile list contract. The admin shell scrolls, while
// cards and pagination must stay reachable without selecting a row by accident.
// Run after seed-demo-data.py against the freshly built embedded SPA.
import assert from 'node:assert/strict'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

import { chromium } from 'playwright'

import { loginSession } from './session-auth.mjs'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const SHOTS_DIR = process.env.MOBILE_LIST_SHOTS_DIR
const VIEWPORT = { width: 375, height: 812 }

async function checkRoute(page, route, minimumRows) {
  await page.goto(BASE_URL + route, { waitUntil: 'domcontentloaded' })
  const rows = page.locator('[data-mobile-card-row]')
  await rows.nth(minimumRows - 1).waitFor()
  assert.ok(
    (await rows.count()) >= minimumRows,
    `${route} needs seeded rows for the mobile scroll check`
  )

  const last = rows.last()
  await last.scrollIntoViewIfNeeded()
  const lastBounds = await last.boundingBox()
  assert.ok(
    lastBounds &&
      lastBounds.y >= 0 &&
      lastBounds.y + lastBounds.height <= VIEWPORT.height + 2,
    `${route}: last card is clipped by the page layout`
  )

  const pager = page.locator('[data-slot=data-table-pagination]')
  await pager.scrollIntoViewIfNeeded()
  const pagerBounds = await pager.boundingBox()
  assert.ok(
    pagerBounds &&
      pagerBounds.y >= 0 &&
      pagerBounds.y + pagerBounds.height <= VIEWPORT.height + 2,
    `${route}: pagination is not reachable below the cards (${JSON.stringify(pagerBounds)})`
  )
  for (const target of await pager.locator('[data-touch-target]').all()) {
    const bounds = await target.boundingBox()
    if (bounds) {
      assert.ok(bounds.height >= 40, `${route}: pager tap target is too short`)
    }
  }
  const scroll = await page.locator('#content').evaluate((element) => ({
    top: element.scrollTop,
    height: element.scrollHeight,
    viewport: element.clientHeight,
  }))
  assert.ok(
    scroll.height > scroll.viewport && scroll.top > 0,
    `${route}: admin content did not scroll (scroll=${JSON.stringify(scroll)})`
  )
  if (SHOTS_DIR) {
    mkdirSync(SHOTS_DIR, { recursive: true })
    await page.screenshot({
      path: join(SHOTS_DIR, `${route.slice(1)}-footer.png`),
    })
  }

  // Selection is explicit. Tapping the card surface, including at the end of
  // a touch scroll, cannot raise the bulk-action bar; the checkbox still can.
  const first = rows.first()
  const firstCardText = await first.textContent()
  await first.evaluate((element) => element.click())
  const checkbox = first.locator('[data-slot=checkbox]')
  assert.equal(await checkbox.getAttribute('aria-checked'), 'false')
  await checkbox.click()
  assert.equal(await checkbox.getAttribute('aria-checked'), 'true')
  // The fixture has a second page. Moving there must replace the cards and
  // leave the pager accessible, not silently keep showing page one.
  await pager.getByRole('button', { name: /2/ }).click()
  await page.waitForFunction((previous) => {
    const card = document.querySelector('[data-mobile-card-row]')
    return card && card.textContent !== previous
  }, firstCardText)
  assert.notEqual(await rows.first().textContent(), firstCardText)
  const nextCheckbox = rows.first().locator('[data-slot=checkbox]')
  const menu = rows.first().locator('[data-slot=dropdown-menu-trigger]')
  await menu.click()
  assert.equal(await nextCheckbox.getAttribute('aria-checked'), 'false')
  const menuItem = page.locator('[data-slot=dropdown-menu-item]').first()
  await page.waitForFunction(() => {
    const item = document.querySelector('[data-slot=dropdown-menu-item]')
    return item && item.getBoundingClientRect().height >= 40
  })
  const itemBounds = await menuItem.boundingBox()
  assert.ok(
    itemBounds && itemBounds.height >= 40,
    `${route}: menu tap target is too short (${JSON.stringify(itemBounds)})`
  )
  console.log(
    `[mobile-list] ${route}: scroll + pager + selection + next page OK`
  )
}

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server', '--disable-dev-shm-usage'],
})
try {
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 1,
    isMobile: true,
    hasTouch: true,
    locale: 'zh-CN',
  })
  await loginSession(context, { baseUrl: BASE_URL, token: AUTH_TOKEN })
  await context.addCookies([
    { name: 'vite-ui-theme', value: 'light', url: BASE_URL },
  ])
  await context.addInitScript(() => localStorage.setItem('i18nextLng', 'zh-CN'))
  const page = await context.newPage()
  await checkRoute(page, '/sites', 20)
  await checkRoute(page, '/accounts', 20)
  await context.close()
} finally {
  await browser.close()
}
