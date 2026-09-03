#!/usr/bin/env node
// metapi-go — frontend ACCEPTANCE e2e (real user journeys, real upstream).
//
// Unlike route-smoke.mjs (a crash/console gate over a fake site), this drives
// the built SPA in a real Chromium through genuine user flows against a REAL
// backend that is itself pointed at a REAL upstream platform (new-api/one-api).
// It is an operator-gated acceptance gate, NOT part of the blocking PR CI: it
// needs live credentials and a running upstream, so it runs on demand via
// `bun run acceptance:e2e`.
//
// Point it at a THROWAWAY instance. cleanupState deletes every site, account,
// route and downstream key on whatever BASE_URL answers, and the login journey
// performs a real upstream login (which rotates that upstream user's dashboard
// credential). Set EXPECT_SERVER_COMMIT to the commit you built so the run
// cannot silently verify a stale binary holding the same port.
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
// Journeys 4-7 are the tail of the core chain, and the reason this gate exists.
// Journeys 1-3 only prove configuration succeeded, which is precisely the shape
// of the "I deployed it and it basically does not work" complaint: every step
// green, chain dead. So the tail is asserted where the operator stands (the UI)
// and then from outside the browser with the key the UI just issued.
// Journey 4 — Models (opt-in): the bound account shows a NON-EMPTY model list in
//   its detail sheet, cross-checked against the API so a UI rendering "0 models"
//   over a populated backend (or the reverse) fails.
// Journey 5 — Route + usable channel (opt-in): "Auto-rebuild" turns those models
//   into routes, and one route must have a channel whose relay credential is
//   BOUND. The unbound state is asserted absent in the wire payload (tokenId)
//   AND in the sheet, because a channel with no token cannot serve anything.
// Journey 6 — Downstream key (opt-in): issue a key through the UI, authorize the
//   model journey 5 verified (an empty model policy is deny-all by design), and
//   capture the key value.
// Journey 7 — Real relay (opt-in): from a standalone HTTP client with no admin
//   session and no browser, call /v1/models and /v1/chat/completions with that
//   key. A true 2xx with non-empty content is required: HTTP 200 over an empty
//   model list, or a structured error body, is a FAIL and never a PASS.
//
// Configuration (environment variables):
//   BASE_URL          metapi admin origin      (default http://127.0.0.1:4000)
//   AUTH_TOKEN        admin Bearer token       (default e2e-admin-token)
//   UPSTREAM_URL      real upstream platform   (default http://127.0.0.1:3000)
//   UPSTREAM_USERNAME upstream login user      (default metapi-e2e)
//   UPSTREAM_PASSWORD upstream login password  (required for journey 2)
//   PLATFORM          platform to select       (default new-api)
//   ACCEPT_SITE_NAME  site name to create      (default acceptance-real-site)
//   ACCEPT_LOGIN      set to "1" to also run journeys 2-7
//   ACCEPT_KEY_NAME   downstream key name to issue  (default acceptance-e2e-key)
//   ACCEPT_KEY_VALUE  downstream key value to issue (default sk-acceptance-e2e-key)
//   ACCEPT_EXPECT_RELAY  require a real relay in journey 7 (default 1; set 0
//                     only for an upstream with no relay capability, which then
//                     reports an honest SKIP instead of a fake PASS)
//   ACCEPT_RELAY_MODEL   model to request (default: the model whose route
//                     journey 5 verified)
//   EXPECTED_COMPLETION_CONTENT  substring the completion content must contain
//   UPSTREAM_REQUEST_LOG  jsonl request log of a deterministic upstream; when
//                     set, journey 7 proves the expected model actually reached
//                     the upstream instead of trusting the response body
//   EXPECT_SERVER_COMMIT  git commit the answering build must report via
//                     /api/about. Set it whenever the instance under test is
//                     built locally: a gate that runs against a stale process
//                     holding the same port verifies nothing while still going
//                     green, which is exactly how a live run of this script once
//                     exercised an older embedded SPA.
//
// Requires a Chromium install (`bunx playwright install chromium`).

import { chromium, request } from 'playwright'

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
const ACCEPT_KEY_NAME = process.env.ACCEPT_KEY_NAME ?? 'acceptance-e2e-key'
const ACCEPT_KEY_VALUE = process.env.ACCEPT_KEY_VALUE ?? 'sk-acceptance-e2e-key'
const ACCEPT_EXPECT_RELAY = process.env.ACCEPT_EXPECT_RELAY ?? '1'
const ACCEPT_RELAY_MODEL = process.env.ACCEPT_RELAY_MODEL ?? ''
const EXPECTED_COMPLETION_CONTENT =
  process.env.EXPECTED_COMPLETION_CONTENT ?? ''
const UPSTREAM_REQUEST_LOG = process.env.UPSTREAM_REQUEST_LOG ?? ''
const EXPECTED_UPSTREAM_MODEL = process.env.EXPECTED_UPSTREAM_MODEL ?? ''
const EXPECT_SERVER_COMMIT = process.env.EXPECT_SERVER_COMMIT ?? ''
const skips = []

const failures = []
const passes = []
const fail = (message) => failures.push(message)
const pass = (message) => {
  passes.push(message)
  console.log(`[PASS] ${message}`)
}
const skip = (message) => {
  skips.push(message)
  console.log(`[SKIP] ${message}`)
}

// Wrap a step so a failure names the step instead of dumping a Playwright stack.
async function act(name, fn) {
  try {
    await fn()
  } catch (error) {
    throw new Error(
      `step "${name}": ${String(error?.message ?? error).split('\n')[0]}`
    )
  }
}

// The tail is ONE chain, not four independent guesses: the model whose route
// journey 5 verifies is the model journey 6 authorizes and journey 7 requests.
const journeyState = {}

/** A form control by its visible label (the label text is locale-pinned to en). */
const sheetLabel = (scope, name) =>
  scope.getByLabel(name, { exact: true }).first()

/** Show a key without printing the whole secret into a log that gets archived. */
const maskKey = (value) =>
  value.length <= 10 ? `${value.slice(0, 3)}...` : `${value.slice(0, 10)}...`

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
async function cleanupState(api) {
  const headers = { Authorization: `Bearer ${AUTH_TOKEN}` }

  const accounts = await api.get(`${BASE_URL}/api/accounts`, { headers })
  if (accounts.status() === 200) {
    const data = await accounts.json()
    const list = Array.isArray(data) ? data : (data.accounts ?? [])
    for (const account of list) {
      await api.delete(`${BASE_URL}/api/accounts/${account.id}`, {
        headers,
      })
    }
  }

  // Routes and downstream keys are wiped as well: a stale route left over from
  // a previous run would show up as a zero-channel row (its channels went away
  // with the accounts) and a stale key value would make journey 6's create a
  // 409. Both would be blamed on the current run.
  const routes = await api.get(`${BASE_URL}/api/routes/summary`, { headers })
  if (routes.status() === 200) {
    const list = (await routes.json().catch(() => null)) ?? []
    for (const route of list) {
      await api.delete(`${BASE_URL}/api/routes/${route.id}`, { headers })
    }
  }

  const keys = await api.get(`${BASE_URL}/api/downstream-keys`, { headers })
  if (keys.status() === 200) {
    const data = await keys.json().catch(() => null)
    for (const key of data?.items ?? []) {
      await api.delete(`${BASE_URL}/api/downstream-keys/${key.id}`, {
        headers,
      })
    }
  }

  const sites = await api.get(`${BASE_URL}/api/sites`, { headers })
  if (sites.status() === 200) {
    const data = await sites.json()
    const list = Array.isArray(data) ? data : (data.sites ?? [])
    for (const site of list) {
      await api.delete(`${BASE_URL}/api/sites/${site.id}`, { headers })
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

// Journey 4 — the account the operator just bound must already carry upstream
// models. This is the step that used to be deferred to a 04:00 cron, so a fresh
// install showed a successful login, an empty model list and no way to build a
// route. Asserted in the UI and cross-checked against the API.
async function journeyAccountModels(context) {
  const label = 'journey: account models visible in UI'
  const headers = { Authorization: `Bearer ${AUTH_TOKEN}` }
  const page = await context.newPage()
  collectPageFailures(page, label)
  try {
    let accountId = null
    let apiModels = []
    await act('locate account', async () => {
      const resp = await context.request.get(`${BASE_URL}/api/accounts`, {
        headers,
      })
      const data = await resp.json().catch(() => null)
      const match = (data?.accounts ?? []).find(
        (account) => account?.username === UPSTREAM_USERNAME
      )
      if (!match) throw new Error(`account "${UPSTREAM_USERNAME}" not found`)
      accountId = match.id
    })

    // The post-login sync is triggered by the login itself but lands
    // asynchronously, so wait for it -- bounded, and the wait is reported in the
    // PASS line. Waiting forever is not the point: an account that never gains
    // models is exactly the defect this journey exists to catch.
    await act('account models over API', async () => {
      const deadline = Date.now() + 60_000
      for (;;) {
        const resp = await context.request.get(
          `${BASE_URL}/api/accounts/${accountId}/models`,
          { headers }
        )
        if (resp.status() !== 200) {
          throw new Error(
            `GET /api/accounts/${accountId}/models HTTP ${resp.status()}`
          )
        }
        const data = await resp.json().catch(() => null)
        apiModels = (data?.models ?? [])
          .map((model) => model?.name)
          .filter(Boolean)
        if (apiModels.length > 0) return
        if (Date.now() > deadline) {
          throw new Error(
            `account ${accountId} still has 0 models 60s after a real login`
          )
        }
        await page.waitForTimeout(1_000)
      }
    })

    await act('goto /accounts', () =>
      page.goto(`${BASE_URL}/accounts`, {
        waitUntil: 'networkidle',
        timeout: 30_000,
      })
    )
    const row = page
      .locator('table tbody tr', { hasText: UPSTREAM_USERNAME })
      .first()
    await act('account row visible', () =>
      row.waitFor({ state: 'visible', timeout: 20_000 })
    )
    await act('open account row actions', () =>
      row
        .getByRole('button', { name: 'Account actions' })
        .first()
        .click({ timeout: 10_000 })
    )
    await act('click View details', () =>
      page
        .getByRole('menuitem', { name: 'View details' })
        .first()
        .click({ timeout: 10_000 })
    )

    const sheet = page.locator('[data-slot="sheet-content"]').first()
    await act('account detail sheet open', () =>
      sheet.waitFor({ state: 'visible', timeout: 15_000 })
    )
    const modelsSection = sheet
      .locator('section')
      .filter({ has: page.locator('h3', { hasText: /^Models/ }) })
      .first()
    await act('models panel visible', () =>
      modelsSection.waitFor({ state: 'visible', timeout: 15_000 })
    )

    let uiCount = 0
    await act('model rows rendered', async () => {
      const items = modelsSection.locator('ul li')
      await items.first().waitFor({ state: 'visible', timeout: 15_000 })
      uiCount = await items.count()
      if (uiCount === 0) throw new Error('models panel rendered no rows')
    })
    await act('models panel does not claim to be empty', async () => {
      if (
        (await modelsSection
          .getByText('No models yet', { exact: false })
          .count()) > 0
      ) {
        throw new Error(
          'models panel shows the empty state while the API reports models'
        )
      }
    })
    await act('UI model count matches the API', async () => {
      if (uiCount !== apiModels.length) {
        throw new Error(
          `UI shows ${uiCount} models, API reports ${apiModels.length}`
        )
      }
    })
    await act('an API model is visible in the UI', () =>
      modelsSection
        .getByText(apiModels[0], { exact: false })
        .first()
        .waitFor({ state: 'visible', timeout: 10_000 })
    )

    journeyState.accountModels = apiModels
    pass(
      `${label}: account ${accountId} shows ${uiCount} models in its detail sheet, API agrees`
    )
  } catch (error) {
    fail(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await page.close().catch(() => {})
  }
}

// Journey 5 — models alone are not a callable product: a route with a BOUND
// relay credential is. "Add route" is what gets a fresh install there (channels
// bind automatically from account model availability). "Auto-rebuild" is NOT:
// it only recomposes the channels of routes that already exist, which a live
// run against a fresh instance proved by completing with 0 routes considered
// while the empty-state copy promised it would generate them.
async function journeyRouteChannel(context) {
  const label = 'journey: route with a usable channel'
  const headers = { Authorization: `Bearer ${AUTH_TOKEN}` }
  const page = await context.newPage()
  collectPageFailures(page, label)
  try {
    const accountModels = journeyState.accountModels ?? []
    if (accountModels.length === 0) {
      throw new Error('the account serves no models (journey 4 did not pass)')
    }
    const model = ACCEPT_RELAY_MODEL || accountModels[0]
    if (!accountModels.includes(model)) {
      throw new Error(
        `model "${model}" is not served by the account (it serves: ${accountModels.join(', ')})`
      )
    }

    await act('goto /token-routes', () =>
      page.goto(`${BASE_URL}/token-routes`, {
        waitUntil: 'networkidle',
        timeout: 30_000,
      })
    )
    await act('click Add route', () =>
      page
        .getByRole('button', { name: 'Add route' })
        .first()
        .click({ timeout: 10_000 })
    )
    const form = page.locator('[data-slot="sheet-content"]').first()
    await act('route form open', () =>
      form.waitFor({ state: 'visible', timeout: 15_000 })
    )
    await act('fill the model match rule', () =>
      sheetLabel(form, 'Model match rule').fill(model)
    )
    await act('submit the route', () =>
      form.locator('button[type="submit"]').first().click({ timeout: 10_000 })
    )

    // Channel population runs inside the create handler, so a route that stays
    // at zero channels is a defect rather than a race. The LIST refetch is
    // asynchronous, so poll the wire and only then look at the row.
    let target = null
    await act('the new route gains a channel', async () => {
      const deadline = Date.now() + 60_000
      let routes = []
      for (;;) {
        const resp = await context.request.get(
          `${BASE_URL}/api/routes/summary`,
          { headers }
        )
        if (resp.status() !== 200) {
          throw new Error(`GET /api/routes/summary HTTP ${resp.status()}`)
        }
        routes = (await resp.json().catch(() => [])) ?? []
        target =
          routes.find((route) => String(route?.modelPattern ?? '') === model) ??
          null
        if (target && Number(target.channelCount ?? 0) > 0) return
        if (Date.now() > deadline) {
          throw new Error(
            target
              ? `route "${model}" was created but has ${target.channelCount} channel(s): nothing can serve it`
              : `route "${model}" never appeared in /api/routes/summary (${routes.length} routes)`
          )
        }
        await page.waitForTimeout(1_000)
      }
    })
    journeyState.routeId = target.id
    journeyState.routeModel = model
    await page.waitForTimeout(1_500) // let the post-create toast settle

    // The same fact now has to be true WHERE THE OPERATOR LOOKS.
    await act('search the new route', async () => {
      await page
        .getByRole('textbox', { name: /Search model/i })
        .first()
        .fill(model)
      await page.waitForTimeout(600) // the toolbar search debounces 300ms
    })
    const row = page.locator('table tbody tr').first()
    await act('route row visible', () =>
      row.waitFor({ state: 'visible', timeout: 15_000 })
    )
    await act('route row does not report zero channels', async () => {
      const text = (await row.innerText()).replace(/\s+/g, ' ')
      if (/0 channels|Not generated|Channels needed/i.test(text)) {
        throw new Error(
          `route row still reports no channels: ${text.slice(0, 160)}`
        )
      }
    })
    await act('open route row actions', () =>
      row
        .getByRole('button', { name: 'Route actions' })
        .first()
        .click({ timeout: 10_000 })
    )
    await act('click View details', () =>
      page
        .getByRole('menuitem', { name: 'View details' })
        .first()
        .click({ timeout: 10_000 })
    )

    const sheet = page.locator('[data-slot="sheet-content"]').first()
    await act('route detail sheet open', () =>
      sheet.waitFor({ state: 'visible', timeout: 15_000 })
    )

    let channels = []
    await act('route channels over API', async () => {
      const resp = await context.request.get(
        `${BASE_URL}/api/routes/${target.id}/channels`,
        { headers }
      )
      if (resp.status() !== 200) {
        throw new Error(
          `GET /api/routes/${target.id}/channels HTTP ${resp.status()}`
        )
      }
      channels = (await resp.json().catch(() => [])) ?? []
      const bound = channels.filter(
        (channel) => channel?.tokenId !== null && channel?.tokenId !== undefined
      )
      if (bound.length === 0) {
        throw new Error(
          `route ${target.id} has ${channels.length} channel(s) but none carries a relay credential`
        )
      }
    })
    await act('channel rows rendered', () =>
      sheet
        .getByText('Token:', { exact: false })
        .first()
        .waitFor({ state: 'visible', timeout: 15_000 })
    )
    // UI-vs-wire truth, per channel. Rebuild creates two flavours for the same
    // account and model: one bound to an account token, one account-scoped that
    // relays with the account's own credential. The sheet has to name each one
    // as the credential it actually is -- and the account-scoped one must never
    // be called "Unbound", which is the label that made a working channel look
    // broken and sent operators hunting for a binding step that does not exist.
    await act(
      'sheet names every channel credential as the wire reports it',
      async () => {
        const text = (await sheet.innerText()).replace(/\s+/g, ' ')
        for (const channel of channels) {
          const hasAccountCredential = Boolean(
            channel?.account?.accessTokenMasked ||
            channel?.account?.apiTokenMasked
          )
          const expected = channel?.tokenId
            ? `Token #${channel.tokenId}`
            : hasAccountCredential
              ? 'Account credential'
              : 'No credential'
          if (!text.includes(expected)) {
            throw new Error(
              `channel ${channel?.id} should read "${expected}" in the sheet`
            )
          }
        }
        if (/Unbound/i.test(text)) {
          throw new Error(
            'route detail still calls an account-scoped channel "Unbound"'
          )
        }
      }
    )

    pass(
      `${label}: route "${model}" created via UI with ${channels.length} channel(s); the sheet names each credential as the wire reports it`
    )
  } catch (error) {
    fail(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await page.close().catch(() => {})
  }
}

// Journey 6 — routes still are not callable; a downstream key is. Issue one the
// way an operator does and authorize the model journey 5 verified: an empty
// model policy is deny-all by design, so a key created without a rule looks
// perfectly healthy in the list and then serves nothing.
async function journeyDownstreamKey(context) {
  const label = 'journey: downstream key issued via UI'
  const page = await context.newPage()
  collectPageFailures(page, label)
  const model = journeyState.routeModel ?? ''
  try {
    if (!model) {
      throw new Error(
        'no verified route model to authorize (journey 5 did not pass)'
      )
    }
    await act('goto /downstream-keys', () =>
      page.goto(`${BASE_URL}/downstream-keys`, {
        waitUntil: 'networkidle',
        timeout: 30_000,
      })
    )
    await act('click Create key', () =>
      page
        .getByRole('button', { name: 'Create key' })
        .first()
        .click({ timeout: 15_000 })
    )
    const sheet = page.locator('[data-slot="sheet-content"]').first()
    await act('key sheet open', () =>
      sheet.waitFor({ state: 'visible', timeout: 15_000 })
    )
    await act('fill key name', () =>
      sheetLabel(sheet, 'Name').fill(ACCEPT_KEY_NAME)
    )
    await act('fill key value', () =>
      sheetLabel(sheet, 'Key').fill(ACCEPT_KEY_VALUE)
    )
    await act('authorize the verified model', async () => {
      await sheet
        .getByPlaceholder('gpt-4o, gpt-*, or re:^claude-')
        .first()
        .fill(model)
      await sheet
        .getByRole('button', { name: 'Add', exact: true })
        .first()
        .click({ timeout: 10_000 })
      const rules = sheet.locator('[data-testid="model-policy-rules"]')
      await rules.waitFor({ state: 'visible', timeout: 10_000 })
      if (!(await rules.innerText()).includes(model)) {
        throw new Error(`model rule "${model}" was not added`)
      }
    })
    await act('submit the key', () =>
      sheet.locator('button[type="submit"]').first().click({ timeout: 10_000 })
    )
    await act('key row appears', () =>
      page
        .getByText(ACCEPT_KEY_NAME, { exact: false })
        .first()
        .waitFor({ state: 'visible', timeout: 20_000 })
    )
    // Creating a key auto-opens the Connect dialog, which is LOCKED until the
    // master token is re-entered (#1034). Close it: the value the operator needs
    // is the one they just typed into the form.
    await act('close the connect dialog', async () => {
      await page.keyboard.press('Escape')
      await page.waitForTimeout(500)
    })

    journeyState.keyValue = ACCEPT_KEY_VALUE
    pass(
      `${label}: issued "${ACCEPT_KEY_NAME}" (${maskKey(ACCEPT_KEY_VALUE)}) authorized for "${model}" via UI`
    )
  } catch (error) {
    fail(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await page.close().catch(() => {})
  }
}

/** Read a deterministic upstream's jsonl request log (missing file => []). */
async function readUpstreamRequestLog() {
  if (!UPSTREAM_REQUEST_LOG) return []
  const { readFile } = await import('node:fs/promises')
  const raw = await readFile(UPSTREAM_REQUEST_LOG, 'utf8').catch(() => '')
  return raw
    .split('\n')
    .filter((line) => line.trim() !== '')
    .map((line) => {
      try {
        return JSON.parse(line)
      } catch {
        return null
      }
    })
    .filter(Boolean)
}

// Journey 7 — the only step that answers "can I actually use it". A standalone
// HTTP client (no admin session, no browser, no SPA) calls /v1 with the key the
// UI issued. Every inference shortcut that ever produced a fake green is closed:
// HTTP 200 is not enough, an empty model list is a failure, a structured error
// body is a failure, missing content is a failure, and when a deterministic
// upstream is configured the request log must show the expected model arriving.
async function journeyRelay() {
  const label = 'journey: real relay with the issued key'
  if (ACCEPT_EXPECT_RELAY !== '1') {
    skip(
      `${label}: ACCEPT_EXPECT_RELAY=${ACCEPT_EXPECT_RELAY} — a chain with no relay capability is a SKIP, never a PASS`
    )
    return
  }
  const keyValue = journeyState.keyValue
  if (!keyValue) {
    fail(`${label}: no downstream key was issued (journey 6 did not pass)`)
    return
  }
  const client = await request.newContext({ baseURL: BASE_URL })
  try {
    const authHeaders = {
      Authorization: `Bearer ${keyValue}`,
      'Content-Type': 'application/json',
    }

    let models = []
    await act('GET /v1/models', async () => {
      const resp = await client.get('/v1/models', { headers: authHeaders })
      const body = await resp.text().catch(() => '')
      if (resp.status() !== 200) {
        throw new Error(`HTTP ${resp.status()} ${body.slice(0, 200)}`)
      }
      const data = body ? JSON.parse(body) : null
      models = (data?.data ?? []).map((model) => model?.id).filter(Boolean)
      if (models.length === 0) {
        throw new Error('/v1/models returned HTTP 200 with an empty model list')
      }
    })

    const model = ACCEPT_RELAY_MODEL || journeyState.routeModel || models[0]
    await act('requested model is served by the key', async () => {
      if (!models.includes(model)) {
        throw new Error(
          `model "${model}" is not in /v1/models (${models.join(', ')})`
        )
      }
    })

    const logBefore = await readUpstreamRequestLog()
    let content = ''
    await act('POST /v1/chat/completions', async () => {
      const resp = await client.post('/v1/chat/completions', {
        headers: authHeaders,
        data: {
          model,
          messages: [
            { role: 'user', content: 'Reply with the model name you serve.' },
          ],
          stream: false,
        },
      })
      const body = await resp.text().catch(() => '')
      if (resp.status() < 200 || resp.status() >= 300) {
        throw new Error(`HTTP ${resp.status()} ${body.slice(0, 300)}`)
      }
      let parsed = null
      try {
        parsed = JSON.parse(body)
      } catch {
        throw new Error(
          `completion response is not JSON: ${body.slice(0, 200)}`
        )
      }
      if (parsed?.error) {
        throw new Error(
          `structured error body: ${JSON.stringify(parsed.error).slice(0, 300)}`
        )
      }
      content = parsed?.choices?.[0]?.message?.content ?? ''
      if (typeof content !== 'string' || content.trim() === '') {
        throw new Error(`completion carried no content: ${body.slice(0, 300)}`)
      }
    })
    await act('completion content carries the expected marker', async () => {
      if (!EXPECTED_COMPLETION_CONTENT) return
      if (!content.includes(EXPECTED_COMPLETION_CONTENT)) {
        throw new Error(
          `content lacks "${EXPECTED_COMPLETION_CONTENT}" (got: ${content.slice(0, 200)})`
        )
      }
    })
    await act('upstream received the expected model', async () => {
      if (!UPSTREAM_REQUEST_LOG) {
        skip(
          `${label}: UPSTREAM_REQUEST_LOG not set — response body is the only relay evidence`
        )
        return
      }
      const expected = EXPECTED_UPSTREAM_MODEL || model
      const after = await readUpstreamRequestLog()
      const fresh = after.slice(logBefore.length)
      const seen = fresh.some(
        (entry) => (entry.model ?? entry.model_received) === expected
      )
      if (!seen) {
        throw new Error(
          `upstream log recorded no "${expected}" request (new lines: ${JSON.stringify(fresh).slice(0, 300)})`
        )
      }
    })

    pass(
      `${label}: ${model} -> real 2xx, content "${content.slice(0, 60)}" with key ${maskKey(keyValue)}`
    )
  } catch (error) {
    fail(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
  } finally {
    await client.dispose().catch(() => {})
  }
}

// Preflight — prove WHICH build is answering before spending a run on it. Every
// journey below is evidence about a specific binary; an orphaned process from an
// earlier build holding the same port turns all of it into evidence about
// nothing. On mismatch the run aborts instead of reporting journeys that
// exercised the wrong SPA.
async function preflightServerIdentity() {
  const label = 'preflight: server identity'
  const client = await request.newContext({ baseURL: BASE_URL })
  try {
    const resp = await client.get('/api/about', {
      headers: { Authorization: `Bearer ${AUTH_TOKEN}` },
    })
    if (resp.status() !== 200) {
      throw new Error(`GET /api/about HTTP ${resp.status()}`)
    }
    const about = await resp.json().catch(() => null)
    const commit = String(about?.commit ?? '')
    const version = String(about?.version ?? '')
    if (EXPECT_SERVER_COMMIT && commit !== EXPECT_SERVER_COMMIT) {
      throw new Error(
        `server reports commit "${commit || '(empty)'}" but EXPECT_SERVER_COMMIT=${EXPECT_SERVER_COMMIT}: a stale or foreign build is answering on ${BASE_URL}`
      )
    }
    pass(
      `${label}: ${BASE_URL} serves version="${version}" commit="${commit || '(empty)'}"`
    )
    return true
  } catch (error) {
    fail(`${label}: ${String(error?.message ?? error).split('\n')[0]}`)
    return false
  } finally {
    await client.dispose().catch(() => {})
  }
}

function summarise() {
  console.log(
    `== acceptance:e2e summary — ${passes.length} passed, ${failures.length} failed, ${skips.length} skipped ==`
  )
  if (failures.length > 0) {
    for (const failure of [...new Set(failures)])
      console.error(`  [FAIL] ${failure}`)
  }
}

if (!(await preflightServerIdentity())) {
  summarise()
  process.exit(1)
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

// Journeys 2-7 are opt-in: they need a real upstream login. Give the freshly
// created site a brief moment to commit, then run login -> check-in -> the tail
// (models, route/channel, key), each stage in its own browser context so a leak
// of one stage's page state cannot flatter the next.
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

    // Journeys 4-6 share one context: they are consecutive steps of the same
    // operator session, and each still opens its own page.
    const tailContext = await loginBrowser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'en',
    })
    await seedAuth(tailContext)
    await journeyAccountModels(tailContext)
    await journeyRouteChannel(tailContext)
    await journeyDownstreamKey(tailContext)
    await tailContext.close()
  } finally {
    await loginBrowser.close()
  }

  // Journey 7 deliberately runs after the browsers are gone: the relay must not
  // depend on a page, a session cookie or an admin token being open.
  await journeyRelay()
}

summarise()
if (failures.length > 0) {
  process.exit(1)
}
