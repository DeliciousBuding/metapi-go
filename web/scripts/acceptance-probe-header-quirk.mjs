#!/usr/bin/env node
// metapi-go — probe for a transient accounts-page header overlap: create a
// fresh site, enter the accounts page immediately, and the page header covers
// the toolbar for a moment, intercepting the "Add account" click.
//
// The main acceptance journey (acceptance-e2e.mjs) works around the quirk by
// polling the accounts snapshot until the site appears BEFORE navigating. This
// probe deliberately recreates the ORIGINAL failing conditions — navigate to
// /accounts immediately after creating a fresh site, no snapshot polling — and
// samples, at high frequency, what element sits at the "Add account" button
// center during the first moments after load, plus whether a real (non-force)
// click succeeds, is intercepted, or finds the button disabled.
//
// Verdicts per trial:
//   clicked        — real click landed and the create dialog opened
//   disabled       — button present but disabled (snapshot race, distinct bug)
//   intercepted    — click hit-tested onto another element (the header overlap)
//   missing        — button never appeared in the sampling window
//
// Configuration:
//   BASE_URL     metapi admin origin  (default http://127.0.0.1:4000)
//   AUTH_TOKEN   admin Bearer token   (default e2e-admin-token)
//   UPSTREAM_URL upstream for the fresh site (any URL; reachability irrelevant)
//   PROBE_REPEATS number of trials    (default 5)

import { chromium } from 'playwright'

import { loginSession } from './session-auth.mjs'

const BASE_URL = (process.env.BASE_URL ?? 'http://127.0.0.1:4000').replace(
  /\/$/,
  ''
)
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'e2e-admin-token'
const UPSTREAM_URL = process.env.UPSTREAM_URL ?? 'http://127.0.0.1:39999'
const REPEATS = Number(process.env.PROBE_REPEATS ?? '5')
const SAMPLE_WINDOW_MS = 2500

const headers = { Authorization: `Bearer ${AUTH_TOKEN}` }

async function wipeState(request) {
  // Both endpoints return bare JSON arrays (not {accounts}/{sites} wrappers),
  // but accept either shape defensively like acceptance-e2e.mjs does.
  const accounts = await request.get(`${BASE_URL}/api/accounts`, { headers })
  if (accounts.status() === 200) {
    const data = await accounts.json().catch(() => null)
    const accountList = Array.isArray(data) ? data : (data?.accounts ?? [])
    for (const account of accountList) {
      await request.delete(`${BASE_URL}/api/accounts/${account.id}`, {
        headers,
      })
    }
  }
  const sites = await request.get(`${BASE_URL}/api/sites`, { headers })
  if (sites.status() === 200) {
    const data = await sites.json().catch(() => null)
    const siteList = Array.isArray(data) ? data : (data?.sites ?? [])
    for (const site of siteList) {
      await request.delete(`${BASE_URL}/api/sites/${site.id}`, { headers })
    }
  }
}

async function createFreshSite(request, name) {
  const response = await request.post(`${BASE_URL}/api/sites`, {
    headers: { ...headers, 'Content-Type': 'application/json' },
    data: { name, url: UPSTREAM_URL, platform: 'new-api' },
  })
  if (response.status() !== 200) {
    throw new Error(
      `create site failed: HTTP ${response.status()} ${await response.text()}`
    )
  }
}

// High-frequency DOM sampling from inside the page. Returns per-sample rows:
// button state (absent/disabled/enabled), elementFromPoint at button center,
// and the header-row vs toolbar rects so an overlap is visible in evidence.
// Uses setInterval (not rAF) so sampling keeps firing even if the compositor
// considers the page idle; a hard stop makes the promise always resolve.
// NOTE: everything the in-page function needs arrives via `arg` — closure
// variables do NOT survive page.evaluate serialization.
function samplerInPage({ windowMs, buttonText }) {
  return new Promise((resolve) => {
    const startedAt = performance.now()
    const samples = []
    const findButton = () => {
      const buttons = Array.from(document.querySelectorAll('button'))
      return buttons.find((element) =>
        new RegExp(buttonText, 'i').test(element.textContent ?? '')
      )
    }
    const describeElement = (element) => {
      if (!element) return null
      const className =
        typeof element.className === 'string'
          ? element.className.split(/\s+/).slice(0, 4).join('.')
          : ''
      return {
        tag: element.tagName.toLowerCase(),
        role: element.getAttribute('role'),
        classes: className,
      }
    }
    const timer = window.setInterval(() => {
      const elapsed = Math.round(performance.now() - startedAt)
      const button = findButton()
      const row = { elapsedMs: elapsed, button: 'absent' }
      // Interception samples must be separable by cause: a dialog opened by
      // the probe's own successful click legitimately covers the button
      // afterwards, which is NOT the load-time overlap this probe hunts.
      row.dialogOpen = !!document.querySelector('[role="dialog"]')
      if (button) {
        const rect = button.getBoundingClientRect()
        const centerX = rect.left + rect.width / 2
        const centerY = rect.top + rect.height / 2
        const hitTarget = document.elementFromPoint(centerX, centerY)
        row.button = button.disabled ? 'disabled' : 'enabled'
        row.buttonRect = {
          top: Math.round(rect.top),
          height: Math.round(rect.height),
        }
        row.hitTarget = describeElement(hitTarget)
        row.hitIsButton =
          !!hitTarget && (hitTarget === button || button.contains(hitTarget))
        const heading = document.querySelector('h1')
        if (heading) {
          const headerRow = heading.closest('div')?.getBoundingClientRect()
          if (headerRow) row.headerRowBottom = Math.round(headerRow.bottom)
        }
      }
      samples.push(row)
      if (elapsed >= windowMs) {
        window.clearInterval(timer)
        resolve(samples)
      }
    }, 50)
  })
}

function summarizeTrial(samples, clickOutcome, dialogOpened) {
  // Load-time interception (the §6 quirk) = covered BEFORE any dialog exists.
  // Post-click interception = the sheet opened by our own successful click
  // legitimately covering the button afterwards (expected, not a quirk).
  const loadTimeIntercepted = samples.filter(
    (row) =>
      row.button === 'enabled' && row.hitIsButton === false && !row.dialogOpen
  )
  const postClickIntercepted = samples.filter(
    (row) =>
      row.button === 'enabled' && row.hitIsButton === false && row.dialogOpen
  )
  const disabledSamples = samples.filter((row) => row.button === 'disabled')
  const firstEnabledAt = samples.find((row) => row.button === 'enabled')
  return {
    clickOutcome,
    dialogOpened,
    firstEnabledAtMs: firstEnabledAt?.elapsedMs ?? null,
    disabledSampleCount: disabledSamples.length,
    loadTimeInterceptedCount: loadTimeIntercepted.length,
    postClickInterceptedCount: postClickIntercepted.length,
    loadTimeTargets: [
      ...new Set(
        loadTimeIntercepted.map(
          (row) =>
            `${row.hitTarget?.tag ?? '?'}${row.hitTarget?.classes ? '.' + row.hitTarget.classes : ''}`
        )
      ),
    ],
    sampleCount: samples.length,
  }
}

const browser = await chromium.launch({ headless: true })
const results = []
try {
  for (let trial = 1; trial <= REPEATS; trial += 1) {
    const context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'en',
    })
    await loginSession(context, { baseUrl: BASE_URL, token: AUTH_TOKEN })
    await context.addInitScript(() => {
      localStorage.setItem('i18nextLng', 'en')
    })

    await wipeState(context.request)
    await createFreshSite(context.request, `quirk-probe-${trial}`)

    const page = await context.newPage()
    // Navigate immediately after create — no snapshot polling (the point).
    await page.goto(`${BASE_URL}/accounts`, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })

    const samplingPromise = page.evaluate(samplerInPage, {
      windowMs: SAMPLE_WINDOW_MS,
      buttonText: 'Add account',
    })
    // Never let a stuck in-page sampler hang the whole probe.
    const samplingGuard = new Promise((resolve) =>
      setTimeout(() => resolve([]), SAMPLE_WINDOW_MS + 5000)
    )
    samplingPromise.catch(() => {})

    // Concurrently attempt a real, actionable click (no force): Playwright
    // waits for visible+enabled+stable+receives-events, so its failure reason
    // distinguishes "disabled" from "intercepted by another element".
    const addAccount = page
      .getByRole('button', { name: /Add account/i })
      .first()
    let clickOutcome = 'clicked'
    try {
      await addAccount.click({ timeout: SAMPLE_WINDOW_MS })
    } catch (error) {
      // Playwright's timeout error embeds the full chronological call log; the
      // LAST matching reason is the state that consumed the timeout budget.
      const message = String(error?.message ?? error)
      const interceptAt = message.search(/intercept/i)
      const notEnabledAt = message.search(/not enabled|disabled/i)
      if (interceptAt === -1 && notEnabledAt === -1) {
        clickOutcome = `error: ${message.split('\n')[0]}`
      } else if (interceptAt > notEnabledAt) {
        clickOutcome = 'intercepted'
      } else {
        clickOutcome = 'disabled'
      }
    }

    let dialogOpened = false
    if (clickOutcome === 'clicked') {
      dialogOpened = await page
        .locator('[role="dialog"]')
        .first()
        .isVisible()
        .catch(() => false)
    }

    const samples = await Promise.race([samplingPromise, samplingGuard])
    const summary = summarizeTrial(samples, clickOutcome, dialogOpened)
    results.push({ trial, ...summary })
    console.log(
      `trial ${trial}: click=${summary.clickOutcome} dialog=${summary.dialogOpened} ` +
        `firstEnabledAt=${summary.firstEnabledAtMs}ms ` +
        `disabledSamples=${summary.disabledSampleCount} ` +
        `loadTimeIntercepted=${summary.loadTimeInterceptedCount} ` +
        `postClickIntercepted=${summary.postClickInterceptedCount}` +
        (summary.loadTimeTargets.length
          ? ` loadTimeTargets=[${summary.loadTimeTargets.join(', ')}]`
          : '')
    )
    await page.close().catch(() => {})
    await context.close().catch(() => {})
  }
} finally {
  await browser.close()
}

const loadTimeInterceptionTotal = results.reduce(
  (sum, r) => sum + r.loadTimeInterceptedCount,
  0
)
const failedTrials = results.filter((r) => r.clickOutcome !== 'clicked')
const clickedTrials = results.filter(
  (r) => r.clickOutcome === 'clicked' && r.dialogOpened
)
console.log(
  `== header-overlap probe summary: ${clickedTrials.length}/${results.length} clean clicks, ` +
    `${failedTrials.length} failed (${
      failedTrials.map((r) => r.clickOutcome).join(', ') || '-'
    }), load-time interception samples=${loadTimeInterceptionTotal} ==`
)
if (loadTimeInterceptionTotal > 0) {
  console.log(
    'REPRODUCED: §6 load-time overlap — button covered by: ' +
      [...new Set(results.flatMap((r) => r.loadTimeTargets))].join(', ')
  )
} else {
  console.log(
    'NOT reproduced at load time: every interception sample occurred AFTER the ' +
      "probe's own click opened the create sheet (expected coverage, not the §6 quirk)."
  )
}
