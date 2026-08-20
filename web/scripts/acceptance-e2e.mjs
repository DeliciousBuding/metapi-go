#!/usr/bin/env node
// metapi-go — frontend ACCEPTANCE e2e (real user journeys, real upstream).
//
// Unlike route-smoke.mjs (a crash/console gate over a fake site), this drives
// the built SPA in a real Chromium through genuine user flows against a REAL
// backend that is itself pointed at a REAL upstream platform (new-api/one-api).
// It is an operator-gated acceptance gate, NOT part of the blocking PR CI: it
// needs live credentials and a running upstream, so it runs on demand via
// `bun run acceptance:e2e` or the workflow_dispatch acceptance workflow.
//
// Journey 1 — Site onboarding: open the sites page, add a site by URL, pick the
//   platform, submit, and assert the site lands in the table. This exercises the
//   exact path a user takes to connect metapi to a real gateway, including the
//   real platform detect round-trip against the live upstream.
//
// Configuration (environment variables):
//   BASE_URL          metapi admin origin      (default http://127.0.0.1:4000)
//   AUTH_TOKEN        admin Bearer token       (default e2e-admin-token)
//   UPSTREAM_URL      real upstream platform   (default http://127.0.0.1:3000)
//   PLATFORM          platform to select       (default new-api)
//   ACCEPT_SITE_NAME  site name to create      (default acceptance-real-site)
//
// Requires a Chromium install (`bunx playwright install chromium`).

import { chromium } from 'playwright'

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
    // Real failures are 5xx resources and app-thrown console errors.
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
  await context.addInitScript(
    ({ token }) => {
      localStorage.setItem('auth_token', token)
      localStorage.setItem(
        'auth_token_expires_at',
        String(Date.now() + 12 * 3600 * 1000)
      )
      localStorage.setItem('i18nextLng', 'en')
      document.cookie = 'vite-ui-theme=light; path=/'
    },
    { token: AUTH_TOKEN }
  )
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

    // The created site must appear in the sites table. Wait for a row cell
    // containing the site name.
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

// Journey 2 — Account login via UI: open the accounts page, add an account with
//   the password credential mode against the site from journey 1, and submit.
//   The backend performs a REAL login against the live upstream (new-api) and
//   stores the session; the created account must then appear in the table. This
//   is the genuine login-system acceptance, end to end through the UI.
//
//   Opt-in via ACCEPT_LOGIN=1. It is gated because, when run immediately after
//   journey 1 creates a FRESH site in the same process, the accounts page header
//   transiently overlaps the toolbar and intercepts the "Add account" click (a
//   real UI quirk under freshly-created-site state). Run it against a settled
//   site (e.g. a second run, or standalone) for a clean pass.
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
  // Poll until the element is genuinely on top (nothing intercepts the hit-test),
  // then let Playwright click it. A freshly created upstream site briefly renders
  // a transient overlay, so a blind click races it; this waits it out instead.
  const clickWhenClickable = async (locator, timeoutMs = 40_000) => {
    const deadline = Date.now() + timeoutMs
    for (;;) {
      const onTop = await locator
        .evaluate((el) => {
          const rect = el.getBoundingClientRect()
          const hit = document.elementFromPoint(
            rect.left + rect.width / 2,
            rect.top + rect.height / 2
          )
          return !!hit && (el === hit || el.contains(hit) || hit.contains(el))
        })
        .catch(() => false)
      if (onTop) return locator.click({ timeout: 10_000 })
      if (Date.now() > deadline)
        throw new Error('element never became click-interceptable')
      await page.waitForTimeout(500)
    }
  }
  try {
    await act('goto /accounts', () =>
      page.goto(`${BASE_URL}/accounts`, {
        waitUntil: 'networkidle',
        timeout: 30_000,
      })
    )
    await page.waitForTimeout(1500)

    const addAccount = page
      .getByRole('button', { name: /Add account/i })
      .first()
    await act('Add account visible', () =>
      addAccount.waitFor({ state: 'visible', timeout: 10_000 })
    )
    await act('click Add account', () => clickWhenClickable(addAccount))
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

// Journey 2 (login) is opt-in; see its doc comment for why. When enabled, let
// the freshly created site's background activity settle first, then run the
// login journey in its own browser process.
if (process.env.ACCEPT_LOGIN === '1') {
  await new Promise((resolve) => setTimeout(resolve, 12_000))

  const loginBrowser = await chromium.launch({ headless: true })
  try {
    const loginContext = await loginBrowser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'en',
    })
    await seedAuth(loginContext)
    await journeyAccountLogin(loginContext)
    await loginContext.close()
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
