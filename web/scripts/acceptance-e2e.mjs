#!/usr/bin/env node
// metapi-go — frontend ACCEPTANCE e2e (real user journeys, real upstream).
//
// Unlike route-smoke.mjs (a crash/console gate over a fake site), this drives
// the built SPA in a real Chromium through genuine user flows against a REAL
// backend that is itself pointed at a REAL upstream platform (new-api/one-api).
// It is an operator-gated acceptance gate, NOT part of the blocking PR CI: it
// needs live credentials and a running upstream, so it runs on demand via
// `bun run acceptance:e2e` (see docs/internal/analysis/e2e-acceptance-platform.md).
//
// Journey 1 — Site onboarding: open the sites page, add a site by URL, pick the
//   platform, submit, and assert it lands in the table (fires the real detect
//   round-trip).
// Journey 2 — Account login (opt-in via ACCEPT_LOGIN=1): add an account with the
//   password credential mode against the site from journey 1 and submit; the
//   backend performs a REAL login against the live upstream and the account must
//   appear in the table.
// Journey 3 — Check-in (opt-in, needs journey 2's account): enable check-in on
//   the account, run all check-ins from the check-in page, and assert a check-in
//   log row is recorded.
//
// Configuration (environment variables):
//   BASE_URL          metapi admin origin      (default http://127.0.0.1:4000)
//   AUTH_TOKEN        admin Bearer token       (default e2e-admin-token)
//   UPSTREAM_URL      real upstream platform   (default http://127.0.0.1:3000)
//   UPSTREAM_USERNAME upstream login user      (default metapi-e2e)
//   UPSTREAM_PASSWORD upstream login password  (required for journey 2)
//   PLATFORM          platform to select       (default new-api)
//   ACCEPT_SITE_NAME  site name to create      (default acceptance-real-site)
//   ACCEPT_LOGIN      set to "1" to also run journeys 2 and 3
//
// Requires a Chromium install (`bunx playwright install chromium`).

import { chromium } from 'playwright'

import { loginSession } from './session-auth.mjs'

const BASE_URL = (process.env.BASE_URL ?? 'http://127.0.0.1:4000').replace(
  /\/$/,
  ''
)
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'e2e-admin-token'
const UPSTREAM_URL = process.env.UPSTREAM_URL ?? 'http://127.0.0.1:3000'
const UPSTREAM_USERNAME = process.env.UPSTREAM_USERNAME ?? 'metapi-e2e'
const UPSTREAM_PASSWORD = process.env.UPSTREAM_PASSWORD ?? ''
const PLATFORM = process.env.PLATFORM ?? 'new-api'
const ACCEPT_SITE_NAME = process.env.ACCEPT_SITE_NAME ?? 'acceptance-real-site'

const failures = []
const passes = []
const fail = (message) => failures.push(message)
const pass = (message) => {
  passes.push(message)
  console.log(`[PASS] ${message}`)
}

function collectPageFailures(page, label) {
  page.on('pageerror', (error) =>
    fail(`${label}: pageerror — ${error.message}`)
  )
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    // A 4xx "Failed to load resource" is a handled HTTP outcome (e.g. the
    // idempotent site-create race returns 409), not an application defect.
    if (/Failed to load resource.*\b4\d\d\b/.test(text)) return
    fail(`${label}: console.error — ${text}`)
  })
  page.on('response', (response) => {
    if (response.status() >= 500) {
      fail(
        `${label}: HTTP ${response.status()} — ${response.request().method()} ${response.url()}`
      )
    }
  })
}

async function seedAuth(context) {
  await loginSession(context, { baseUrl: BASE_URL, token: AUTH_TOKEN })
  await context.addInitScript(() => {
    localStorage.setItem('i18nextLng', 'en')
    document.cookie = 'vite-ui-theme=light; path=/'
  })
}

// Wipe all sites and accounts for a deterministic start. This acceptance gate
// runs against a dedicated throwaway metapi instance, so clearing prior state
// (including any site already bound to the upstream URL, which would otherwise
// trigger a duplicate-URL create conflict) is safe and keeps runs idempotent.
async function cleanupState(request) {
  const headers = { Authorization: `Bearer ${AUTH_TOKEN}` }

  const accounts = await request.get(`${BASE_URL}/api/accounts`, { headers })
  if (accounts.status() === 200) {
    const data = await accounts.json()
    const list = Array.isArray(data) ? data : (data.accounts ?? [])
    for (const account of list) {
      await request.delete(`${BASE_URL}/api/accounts/${account.id}`, {
        headers,
      })
    }
  }

  const sites = await request.get(`${BASE_URL}/api/sites`, { headers })
  if (sites.status() === 200) {
    const data = await sites.json()
    const list = Array.isArray(data) ? data : (data.sites ?? [])
    for (const site of list) {
      await request.delete(`${BASE_URL}/api/sites/${site.id}`, { headers })
    }
  }
}

async function journeySiteOnboarding(context) {
  const label = 'journey: site onboarding'
  const page = await context.newPage()
  collectPageFailures(page, label)
  try {
    await page.goto(`${BASE_URL}/sites`, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })

    const addSite = page
      .getByRole('button', { name: /Add site|添加站点|Add/i })
      .first()
    await addSite.waitFor({ state: 'visible', timeout: 10_000 })
    await addSite.click()

    const nameField = page.getByLabel('Name', { exact: false }).first()
    await nameField.waitFor({ state: 'visible', timeout: 10_000 })
    await nameField.fill(ACCEPT_SITE_NAME)

    const urlField = page.getByLabel('URL', { exact: false }).first()
    await urlField.fill(UPSTREAM_URL)

    // Explicitly choose the platform so the journey does not depend on the
    // timing of the background auto-detect request.
    const platformCombobox = page
      .getByRole('combobox', { name: /Platform/i })
      .first()
    await platformCombobox.click()
    const platformOption = page
      .getByRole('option', { name: PLATFORM, exact: true })
      .first()
    await platformOption.waitFor({ state: 'visible', timeout: 10_000 })
    await platformOption.click()

    await page
      .getByRole('button', { name: /Create|创建/i })
      .first()
      .click()

    // The created site must appear in the sites table.
    await page
      .getByText(ACCEPT_SITE_NAME, { exact: false })
      .first()
      .waitFor({ state: 'visible', timeout: 15_000 })

    pass(
      `${label}: created "${ACCEPT_SITE_NAME}" -> ${UPSTREAM_URL} (${PLATFORM}) via UI`
    )
  } catch (error) {
    fail(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await page.close().catch(() => {})
  }
}

async function journeyAccountLogin(context) {
  const label = 'journey: account login via UI'
  if (!UPSTREAM_PASSWORD) {
    fail(`${label}: UPSTREAM_PASSWORD not set — skipping real login journey`)
    return
  }
  const page = await context.newPage()
  collectPageFailures(page, label)
  const act = async (name, fn) => {
    try {
      await fn()
    } catch (error) {
      throw new Error(
        `step "${name}": ${String(error?.message ?? error).split('\n')[0]}`
      )
    }
  }
  // Poll the accounts snapshot until it includes the freshly created site,
  // BEFORE navigating. The "Add account" button is disabled while the snapshot
  // reports zero sites, and the page pins the first prefetched snapshot; waiting
  // here ensures the first render already sees the site so the button enables.
  const waitForSiteInSnapshot = async (timeoutMs = 45_000) => {
    const deadline = Date.now() + timeoutMs
    const headers = { Authorization: `Bearer ${AUTH_TOKEN}` }
    for (;;) {
      const resp = await context.request.get(`${BASE_URL}/api/accounts`, {
        headers,
      })
      if (resp.status() === 200) {
        const data = await resp.json().catch(() => null)
        const sites = data?.sites ?? []
        if (sites.some((site) => site?.name === ACCEPT_SITE_NAME)) return
      }
      if (Date.now() > deadline)
        throw new Error('site never appeared in the accounts snapshot')
      await page.waitForTimeout(500)
    }
  }
  try {
    await act('site in accounts snapshot', () => waitForSiteInSnapshot())
    await act('goto /accounts', () =>
      page.goto(`${BASE_URL}/accounts`, {
        waitUntil: 'networkidle',
        timeout: 30_000,
      })
    )
    await page.waitForTimeout(1000)

    const addAccount = page
      .getByRole('button', { name: /Add account/i })
      .first()
    await act('Add account visible', () =>
      addAccount.waitFor({ state: 'visible', timeout: 10_000 })
    )
    await act('click Add account', () => addAccount.click({ timeout: 10_000 }))
    await page.waitForTimeout(500)

    // Switch to the password credential mode. Tabs: Session / API Key / Password.
    await act('click Password tab', () =>
      page
        .getByRole('tab', { name: /Password|密码/i })
        .first()
        .click({ timeout: 10_000 })
    )
    await page.waitForTimeout(400)

    // Choose the site created in journey 1. The site field is a Base UI select
    // whose trigger renders the "Select a site" placeholder text.
    const siteTrigger = page
      .getByRole('combobox')
      .filter({ hasText: /Select a site|选择站点/ })
      .first()
    await act('site trigger visible', () =>
      siteTrigger.waitFor({ state: 'visible', timeout: 10_000 })
    )
    await act('click site trigger', () =>
      siteTrigger.click({ timeout: 10_000 })
    )
    await page.waitForTimeout(600)
    // The option label is "<site name> · <platform>", so match by substring.
    const siteOption = page
      .getByRole('option', { name: new RegExp(ACCEPT_SITE_NAME) })
      .first()
    await act('site option visible', () =>
      siteOption.waitFor({ state: 'visible', timeout: 10_000 })
    )
    await act('click site option', () => siteOption.click({ timeout: 10_000 }))
    await page.waitForTimeout(400)

    await act('fill Username', () =>
      page.getByLabel('Username').first().fill(UPSTREAM_USERNAME)
    )
    await act('fill Password', () =>
      page
        .getByLabel('Password', { exact: true })
        .first()
        .fill(UPSTREAM_PASSWORD)
    )

    // The dialog's submit button is the only type=submit inside the dialog.
    const submit = page.locator('[role="dialog"] button[type="submit"]').first()
    await act('submit visible', () =>
      submit.waitFor({ state: 'visible', timeout: 10_000 })
    )
    await act('click submit', () => submit.click({ timeout: 10_000 }))

    // The real upstream login happens on submit; the account row must appear.
    await act('account row appears', () =>
      page
        .getByText(UPSTREAM_USERNAME, { exact: false })
        .first()
        .waitFor({ state: 'visible', timeout: 25_000 })
    )

    pass(
      `${label}: logged in "${UPSTREAM_USERNAME}" against real upstream via UI`
    )
  } catch (error) {
    fail(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await page.close().catch(() => {})
  }
}

// Journey 3 — Check-in via UI: enable check-in on the account from journey 2,
//   open the check-in page, run all check-ins, and assert a check-in log row
//   appears. Against a real upstream that does not enable check-in the result is
//   an honest skipped/failed log — the point is the full UI + backend round-trip
//   runs and records something, not that the upstream grants reward.
async function journeyCheckin(context) {
  const label = 'journey: check-in via UI'
  const headers = {
    Authorization: `Bearer ${AUTH_TOKEN}`,
    'Content-Type': 'application/json',
  }
  const page = await context.newPage()
  collectPageFailures(page, label)
  const act = async (name, fn) => {
    try {
      await fn()
    } catch (error) {
      throw new Error(
        `step "${name}": ${String(error?.message ?? error).split('\n')[0]}`
      )
    }
  }
  try {
    // Find the account created in journey 2 and make sure check-in is enabled
    // so "run all" actually targets it.
    let accountId = null
    await act('locate account', async () => {
      const resp = await context.request.get(`${BASE_URL}/api/accounts`, {
        headers,
      })
      const data = await resp.json().catch(() => null)
      const accounts = data?.accounts ?? []
      const match = accounts.find((a) => a?.username === UPSTREAM_USERNAME)
      if (!match) throw new Error(`account "${UPSTREAM_USERNAME}" not found`)
      accountId = match.id
    })
    await act('enable check-in', async () => {
      const resp = await context.request.put(
        `${BASE_URL}/api/accounts/${accountId}`,
        { headers, data: { checkinEnabled: true } }
      )
      if (resp.status() !== 200) {
        throw new Error(`enable check-in HTTP ${resp.status()}`)
      }
    })

    await act('goto /checkin', () =>
      page.goto(`${BASE_URL}/checkin`, {
        waitUntil: 'networkidle',
        timeout: 30_000,
      })
    )
    await page.waitForTimeout(800)

    await act('click Run all check-ins', () =>
      page
        .getByRole('button', { name: /Run all check-ins|运行所有签到|签到/i })
        .first()
        .click({ timeout: 10_000 })
    )

    // The run records a check-in log row (success / skipped / failed). Wait for
    // any row to appear in the check-in records table.
    await act('check-in log appears', () =>
      page
        .locator('table tbody tr')
        .first()
        .waitFor({ state: 'visible', timeout: 25_000 })
    )

    pass(`${label}: ran all check-ins and a check-in log row was recorded`)
  } catch (error) {
    fail(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await page.close().catch(() => {})
  }
}

const browser = await chromium.launch({ headless: true })
try {
  const onboardingContext = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: 'en',
  })
  await seedAuth(onboardingContext)
  await cleanupState(onboardingContext.request)
  await journeySiteOnboarding(onboardingContext)
  await onboardingContext.close()
} finally {
  await browser.close()
}

// Journey 2 (login) + Journey 3 (check-in) are opt-in. Give the freshly created
// site a brief moment to commit, then run the login journey, then the check-in
// journey (which needs the account journey 2 created), each in its own browser.
if (process.env.ACCEPT_LOGIN === '1') {
  await new Promise((resolve) => setTimeout(resolve, 3_000))

  const loginBrowser = await chromium.launch({ headless: true })
  try {
    const loginContext = await loginBrowser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'en',
    })
    await seedAuth(loginContext)
    await journeyAccountLogin(loginContext)
    await loginContext.close()

    const checkinContext = await loginBrowser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'en',
    })
    await seedAuth(checkinContext)
    await journeyCheckin(checkinContext)
    await checkinContext.close()
  } finally {
    await loginBrowser.close()
  }
}

console.log(
  `== acceptance:e2e summary — ${passes.length} passed, ${failures.length} failed ==`
)
if (failures.length > 0) {
  for (const failure of [...new Set(failures)])
    console.error(`  [FAIL] ${failure}`)
  process.exit(1)
}
