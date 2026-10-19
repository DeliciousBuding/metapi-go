// Deterministic real-browser review of the route list's inline channel
// expansion. API responses are intercepted so every state is reproducible
// without mutating a local demo database:
//   multi-channel + cooldown + missing credential, fallback-only pricing,
//   price-query failure, the diagnostics sheet, dark mode, and mobile.
//
// Prereq: serve a fresh embedded build on BASE_URL (see screenshot-scan.mjs).
// Usage:
//   BASE_URL=http://127.0.0.1:4099 node scripts/route-expansion-shots.mjs
//   OUT_DIR=<dir> BASE_URL=... node scripts/route-expansion-shots.mjs
import { mkdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { chromium } from 'playwright'

import { loginSession } from './session-auth.mjs'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4000'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const OUT_DIR =
  process.env.OUT_DIR ?? join(tmpdir(), 'metapi-route-expansion-shots')
const ONLY_SCENARIO = process.env.SCENARIO

const future = new Date(Date.now() + 12 * 60 * 1000).toISOString()
const past = new Date(Date.now() - 18 * 60 * 1000).toISOString()

const routes = [
  {
    id: 901,
    modelPattern: 'gpt-6-sol',
    displayName: 'GPT-6 Sol',
    displayIcon: '',
    routeMode: 'exact',
    modelMapping: '',
    routingStrategy: 'weighted',
    contextLength: 400000,
    enabled: true,
    channelCount: 3,
    enabledChannelCount: 2,
    siteNames: ['NewAPI 主站', 'Sub2API 备用'],
    decisionSnapshot: null,
    decisionRefreshedAt: past,
  },
  {
    id: 902,
    modelPattern: 'claude-opus-5*',
    displayName: 'Claude Opus 5',
    displayIcon: '',
    routeMode: 'pattern',
    modelMapping: '',
    routingStrategy: 'weighted',
    contextLength: null,
    enabled: true,
    channelCount: 2,
    enabledChannelCount: 1,
    siteNames: ['Sub2API 备用'],
    decisionSnapshot: null,
    decisionRefreshedAt: past,
  },
  {
    id: 903,
    modelPattern: 'qwen3.8-max',
    displayName: 'Qwen3.8 Max',
    displayIcon: '',
    routeMode: 'exact',
    modelMapping: '',
    routingStrategy: 'weighted',
    contextLength: 128000,
    enabled: true,
    channelCount: 1,
    enabledChannelCount: 1,
    siteNames: ['NewAPI 主站'],
    decisionSnapshot: null,
    decisionRefreshedAt: past,
  },
]

const sitePrimary = {
  id: 1,
  name: 'NewAPI 主站',
  platform: 'new-api',
}
const siteBackup = {
  id: 2,
  name: 'Sub2API 备用',
  platform: 'sub2api',
}

const channelsByRoute = {
  901: [
    {
      id: 9001,
      routeId: 901,
      accountId: 901,
      tokenId: 91,
      sourceModel: 'gpt-6-sol',
      priority: 0,
      weight: 60,
      enabled: true,
      manualOverride: false,
      successCount: 12840,
      failCount: 7,
      cooldownUntil: '',
      account: {
        username: 'team-production-primary@corp.example',
        accessTokenMasked: 'sess****a1',
      },
      site: sitePrimary,
      token: {
        id: 91,
        name: 'production-key',
        accountId: 901,
        enabled: true,
        isDefault: true,
      },
    },
    {
      id: 9002,
      routeId: 901,
      accountId: 902,
      tokenId: null,
      sourceModel: 'gpt-6-sol',
      priority: 1,
      weight: 30,
      enabled: true,
      manualOverride: false,
      successCount: 3841,
      failCount: 29,
      cooldownUntil: future,
      account: {
        username: 'team-staging-account-scoped@corp.example',
        apiTokenMasked: 'sk-a2****9f',
      },
      site: siteBackup,
      token: null,
    },
    {
      id: 9003,
      routeId: 901,
      accountId: 903,
      tokenId: null,
      sourceModel: 'gpt-6-sol',
      priority: 2,
      weight: 10,
      enabled: false,
      manualOverride: false,
      successCount: 0,
      failCount: 0,
      cooldownUntil: '',
      account: { username: 'credentialless-fallback@corp.example' },
      site: siteBackup,
      token: null,
    },
  ],
  902: [
    {
      id: 9011,
      routeId: 902,
      accountId: 904,
      tokenId: null,
      sourceModel: 'claude-opus-5-5',
      priority: 0,
      weight: 70,
      enabled: true,
      manualOverride: false,
      successCount: 902,
      failCount: 4,
      cooldownUntil: '',
      account: {
        username: 'claude-backup@corp.example',
        accessTokenMasked: 'sess****b1',
      },
      site: siteBackup,
      token: null,
    },
    {
      id: 9012,
      routeId: 902,
      accountId: 905,
      tokenId: 95,
      sourceModel: 'claude-opus-5-5',
      priority: 1,
      weight: 30,
      enabled: false,
      manualOverride: false,
      successCount: 118,
      failCount: 13,
      cooldownUntil: '',
      account: { username: 'claude-secondary@corp.example' },
      site: sitePrimary,
      token: {
        id: 95,
        name: 'secondary-key',
        accountId: 905,
        enabled: true,
        isDefault: false,
      },
    },
  ],
  903: [
    {
      id: 9021,
      routeId: 903,
      accountId: 906,
      tokenId: 96,
      sourceModel: 'qwen3.8-max',
      priority: 0,
      weight: 100,
      enabled: true,
      manualOverride: false,
      successCount: 601,
      failCount: 2,
      cooldownUntil: '',
      account: {
        username: 'qwen-route@corp.example',
        apiTokenMasked: 'sk-qw****21',
      },
      site: sitePrimary,
      token: {
        id: 96,
        name: 'qwen-key',
        accountId: 906,
        enabled: true,
        isDefault: true,
      },
    },
  ],
}

const priceRows = {
  'gpt-6-sol': [
    {
      siteId: 1,
      siteName: 'NewAPI 主站',
      platform: 'new-api',
      model: 'gpt-6-sol',
      accountId: 901,
      username: 'team-production-primary@corp.example',
      inputPerMillion: 1.25,
      outputPerMillion: 10,
      source: 'observed',
      ratesSource: 'configured',
      estimatedCostSample: 0.011,
      observedSamples: 18,
      configuredUnitCost: null,
      missingPrice: false,
      recommended: true,
    },
    {
      siteId: 2,
      siteName: 'Sub2API 备用',
      platform: 'sub2api',
      model: 'gpt-6-sol',
      accountId: 902,
      username: 'team-staging-account-scoped@corp.example',
      inputPerMillion: 0.8,
      outputPerMillion: 6.4,
      source: 'billing_details',
      ratesSource: 'billing_details',
      estimatedCostSample: 0.0072,
      observedSamples: 11,
      configuredUnitCost: 0.008,
      missingPrice: false,
      recommended: false,
    },
  ],
  'claude-opus-5-5': [
    {
      siteId: 2,
      siteName: 'Sub2API 备用',
      platform: 'sub2api',
      model: 'claude-opus-5-5',
      accountId: 904,
      username: 'claude-backup@corp.example',
      inputPerMillion: 3,
      outputPerMillion: 15,
      source: 'observed',
      ratesSource: 'fallback',
      estimatedCostSample: 0.018,
      observedSamples: 2,
      configuredUnitCost: null,
      missingPrice: false,
      recommended: true,
    },
  ],
  'qwen3.8-max': [
    {
      siteId: 1,
      siteName: 'NewAPI 主站',
      platform: 'new-api',
      model: 'qwen3.8-max',
      accountId: 906,
      username: 'qwen-route@corp.example',
      inputPerMillion: 0.42,
      outputPerMillion: 1.68,
      source: 'configured',
      ratesSource: 'configured',
      estimatedCostSample: 0.0021,
      observedSamples: 7,
      configuredUnitCost: 0.0021,
      missingPrice: false,
      recommended: true,
    },
  ],
}

function json(route, body, status = 200) {
  return route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  })
}

async function newPage(browser, { viewport, theme = 'light', mobile = false }) {
  const context = await browser.newContext({
    viewport,
    deviceScaleFactor: 2,
    locale: 'zh-CN',
    isMobile: mobile,
  })
  await loginSession(context, { baseUrl: BASE_URL, token: AUTH_TOKEN })
  await context.addCookies([
    { name: 'vite-ui-theme', value: theme, url: BASE_URL },
  ])
  await context.addInitScript(() => localStorage.setItem('i18nextLng', 'zh-CN'))
  await context.route('**/api/routes/summary*', (route) => json(route, routes))
  await context.route('**/api/routes/*/channels*', (route) => {
    const match = new URL(route.request().url()).pathname.match(
      /^\/api\/routes\/(\d+)\/channels$/
    )
    const routeId = match ? Number(match[1]) : 0
    const channels = channelsByRoute[routeId]
    if (!channels) {
      return json(route, { success: false, message: 'route not found' }, 404)
    }
    return json(route, channels)
  })

  let failPrices = false
  await context.route('**/api/models/price-compare*', (route) => {
    if (failPrices) {
      return json(
        route,
        { success: false, message: 'synthetic price lookup failure' },
        500
      )
    }
    const model = new URL(route.request().url()).searchParams.get('model')
    return json(route, {
      model: model ?? '',
      days: 30,
      limit: 200,
      items: priceRows[model] ?? [],
      pricingAccounts: [],
      currentPrices: [],
      officialPrices: {},
    })
  })

  return {
    context,
    page: await context.newPage(),
    failPrices: () => {
      failPrices = true
    },
  }
}

async function settle(page, ms = 900) {
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
  await page.waitForTimeout(ms)
}

async function openRoutes(page) {
  await page.goto(`${BASE_URL}/token-routes`, { waitUntil: 'domcontentloaded' })
  await settle(page)
}

async function expand(page, title) {
  await page.getByRole('button', { name: title, exact: true }).click()
  const region = page.getByRole('region', { name: `${title} 的通道详情` })
  await region.waitFor({ state: 'visible', timeout: 8000 })
  await settle(page, 1200)
  return region
}

async function capture(page, name, options = {}) {
  await page.screenshot({ path: join(OUT_DIR, `${name}.png`), ...options })
}

async function captureRegion(region, name) {
  await region.screenshot({ path: join(OUT_DIR, `${name}.png`) })
}

async function expectText(locator, ...needles) {
  const text = await locator.innerText()
  for (const needle of needles) {
    if (!text.includes(needle)) {
      throw new Error(`missing expected text ${JSON.stringify(needle)}`)
    }
  }
}

mkdirSync(OUT_DIR, { recursive: true })
const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})
const results = []

async function run(name, options, body) {
  if (ONLY_SCENARIO && !name.includes(ONLY_SCENARIO)) return
  const { context, page, failPrices } = await newPage(browser, options)
  try {
    await body({ page, failPrices })
    results.push(`OK   ${name}`)
  } catch (error) {
    const reason = String(error?.message ?? error).split('\n')[0]
    results.push(`FAIL ${name}: ${reason}`)
    await capture(page, `${name}-FAIL`).catch(() => {})
  } finally {
    await context.close().catch(() => {})
  }
}

const desktop = { viewport: { width: 1440, height: 1000 } }
const mobile = { viewport: { width: 390, height: 844 }, mobile: true }

await run('01-routes-collapsed-light', desktop, async ({ page }) => {
  await openRoutes(page)
  await capture(page, '01-routes-collapsed-light', { fullPage: true })
})

await run('02-route-multi-expanded-light', desktop, async ({ page }) => {
  await openRoutes(page)
  const region = await expand(page, 'GPT-6 Sol')
  await expectText(region, '冷却中', '无可用凭据', '$1.2500', '价格不可用')
  await capture(page, '02-route-multi-expanded-light', { fullPage: true })
  await captureRegion(region, '02-route-multi-detail')
})

await run('03-route-fallback-price', desktop, async ({ page }) => {
  await openRoutes(page)
  const region = await expand(page, 'Claude Opus 5')
  await expectText(region, '价格不可用', '成本来源')
  await capture(page, '03-route-fallback-price', { fullPage: true })
  await captureRegion(region, '03-route-fallback-detail')
})

await run('04-route-price-error', desktop, async ({ page, failPrices }) => {
  failPrices()
  await openRoutes(page)
  const region = await expand(page, 'Qwen3.8 Max')
  if (!(await region.innerText()).includes('正在加载价格')) {
    throw new Error('price loading state was not visible')
  }
  await captureRegion(region, '04-route-price-loading-detail')
  // The production QueryClient retries transient 5xx four times (1+2+4+8s)
  // before surfacing the error; wait through that budget rather than
  // screenshotting a transient loading state as if it were the failure.
  await page.waitForTimeout(18_000)
  await expectText(region, '价格不可用', '重新加载价格')
  await capture(page, '04-route-price-error', { fullPage: true })
  await captureRegion(region, '04-route-price-error-detail')
})

await run('05-route-detail-sheet', desktop, async ({ page }) => {
  await openRoutes(page)
  const row = page.locator('tbody tr').filter({ hasText: 'GPT-6 Sol' })
  await row.locator('button').last().click()
  await page.getByRole('menuitem', { name: '查看详情' }).click()
  await settle(page, 1200)
  await capture(page, '05-route-detail-sheet')
})

await run(
  '06-route-multi-expanded-dark',
  { ...desktop, theme: 'dark' },
  async ({ page }) => {
    await openRoutes(page)
    await expand(page, 'GPT-6 Sol')
    await capture(page, '06-route-multi-expanded-dark', { fullPage: true })
  }
)

await run('07-routes-collapsed-mobile', mobile, async ({ page }) => {
  await openRoutes(page)
  await capture(page, '07-routes-collapsed-mobile', { fullPage: true })
})

await run('08-route-multi-expanded-mobile', mobile, async ({ page }) => {
  await openRoutes(page)
  await expand(page, 'GPT-6 Sol')
  await capture(page, '08-route-multi-expanded-mobile', { fullPage: true })
})

await run('09-route-detail-sheet-mobile', mobile, async ({ page }) => {
  await openRoutes(page)
  const row = page
    .locator('[data-mobile-card-row]')
    .filter({ hasText: 'GPT-6 Sol' })
    .first()
  await row.getByRole('button', { name: '路由操作' }).click()
  await page.getByRole('menuitem', { name: '查看详情' }).click()
  await settle(page, 1200)
  await capture(page, '09-route-detail-sheet-mobile')
  const scroller = page.getByRole('dialog').locator('.overflow-y-auto').first()
  await scroller.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })
  await page.waitForTimeout(300)
  await capture(page, '10-route-detail-sheet-mobile-scrolled')
})

await browser.close()
console.log(results.join('\n'))
console.log(
  `done: ${results.filter((r) => r.startsWith('OK')).length}/${results.length}`
)
console.log(`output: ${OUT_DIR}`)
if (results.some((r) => r.startsWith('FAIL'))) process.exitCode = 1
