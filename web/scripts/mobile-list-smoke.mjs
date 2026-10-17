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
  if (route === '/accounts') {
    const name = first.locator('[data-slot=account-name]')
    assert.ok(
      await name.isVisible(),
      'account name must be a reachable detail action'
    )
    await name.click()
    await page.locator('[data-slot=sheet-content]').waitFor()
    assert.equal(await checkbox.getAttribute('aria-checked'), 'false')
    await page.keyboard.press('Escape')
    await page.locator('[data-slot=sheet-content]').waitFor({ state: 'hidden' })
  }
  await checkbox.click()
  assert.equal(await checkbox.getAttribute('aria-checked'), 'true')
  // The fixture has a second page. Moving there must replace the cards and
  // leave the pager accessible, not silently keep showing page one.
  await pager.getByRole('button', { name: '下一页', exact: true }).click()
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

async function checkManyPages(page) {
  const response = await page.request.get(
    `${BASE_URL}/api/models/marketplace?page=1&pageSize=20`
  )
  assert.ok(response.ok(), 'model fixture request failed')
  const fixture = await response.json()
  assert.ok(fixture.items.length > 0, 'model fixture is empty')
  // Exercise a middle page with two ellipses without seeding thousands of models.
  await page.route('**/api/models/marketplace?*', (route) =>
    route.fulfill({ json: { ...fixture, total: 2000 } })
  )
  for (const width of [320, 375]) {
    await page.setViewportSize({ width, height: VIEWPORT.height })
    await page.goto(`${BASE_URL}/models?page=24`, { waitUntil: 'networkidle' })
    const pager = page.getByRole('navigation', { name: '分页', exact: true })
    await pager.scrollIntoViewIfNeeded()
    const bounds = await pager.boundingBox()
    assert.ok(bounds && bounds.x >= 0 && bounds.x + bounds.width <= width)
    for (const control of await pager.locator('button:visible').all()) {
      const box = await control.boundingBox()
      assert.ok(
        box &&
          box.x >= bounds.x &&
          box.x + box.width <= bounds.x + bounds.width + 1,
        `${width}px: pagination control is clipped`
      )
      assert.ok(box.width >= 40 && box.height >= 40)
      if (!(await control.isDisabled())) await control.click({ trial: true })
    }
    const summary = pager.getByRole('status')
    await summary.waitFor()
    assert.equal(await summary.textContent(), '第 25 / 100 页')
    await pager.getByRole('button', { name: '下一页', exact: true }).click()
    await page.waitForURL((url) => url.searchParams.get('page') === '25')
    assert.equal(await summary.textContent(), '第 26 / 100 页')
    await pager.getByRole('button', { name: '上一页', exact: true }).click()
    await page.waitForURL((url) => url.searchParams.get('page') === '24')
    if (SHOTS_DIR) {
      await page.screenshot({
        path: join(SHOTS_DIR, `models-${width}-pagination.png`),
      })
    }
  }
  await page.unroute('**/api/models/marketplace?*')
  console.log('[mobile-list] 100-page pagination at 320/375px OK')
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
  await checkManyPages(page)
  await context.close()
} finally {
  await browser.close()
}
