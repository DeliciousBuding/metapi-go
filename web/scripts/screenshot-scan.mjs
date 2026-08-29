#!/usr/bin/env node
// metapi-go — real-browser screenshot harness for manual UI/UX review.
//
// Boots against a running server (BASE_URL), navigates every authenticated
// route plus the sign-in page, and saves PNG screenshots for light + dark
// themes across desktop and mobile viewports. Mirrors route-smoke.mjs auth
// seeding so the exact shipped bundle is exercised.
//
// Prerequisite: the served SPA must be a FRESH build. `web/dist` is not in
// git — the Go server embeds whatever sits on disk when you start it, so a
// stale dist serves a pre-session-model SPA whose auth guard silently
// redirects every authenticated route to /sign-in (the harness ugl says
// "112 screenshots, all login pages, exit 0"). Always rebuild before serving:
//
//   (cd web && bun run build:web)   # or: bun run build:check
//   # restart the Go server afterwards so the new dist is embedded
//
// Usage:
//   BASE_URL=http://127.0.0.1:4099 AUTH_TOKEN=dev-admin-token-123 \
//     OUT_DIR=<output-dir> node scripts/screenshot-scan.mjs
//
// Env knobs:
//   THEMES         light,dark (default) — comma-separated theme list
//   VIEWPORTS      desktop,mobile (default) — trim the sweep to a subset
//   MOBILE_SAMPLE  comma-separated mobile route subset (default: all mobile
//                  routes; only consulted when mobile is in VIEWPORTS)
//   DPR            device pixel ratio (default 2)
//   OUT_DIR        output directory (default: OS temp dir)
//   EXPECTED_DATA_PROFILE  empty|seeded; fail before capture when runtime data does not match
//   PROFILE_CHECK_ONLY     1 exits after the profile preflight (test/debug helper)
//
// MANIFEST.md (route/theme/viewport/dimensions, always written to OUT_DIR) is
// the CI evidence list. Any route that fails to capture makes the script exit
// non-zero — the ui-screenshots CI job gates on full coverage.

import { mkdirSync, statSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, join } from 'node:path'

import { loginSession } from './session-auth.mjs'

let sharp

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const EXPECTED_DATA_PROFILE = (process.env.EXPECTED_DATA_PROFILE ?? '').trim()
const PROFILE_CHECK_ONLY = process.env.PROFILE_CHECK_ONLY === '1'
// Portable default: OS temp dir. Override OUT_DIR to keep screenshots
// somewhere durable (e.g. when collecting README gallery material).
const OUT_DIR = process.env.OUT_DIR ?? join(tmpdir(), 'metapi-shots')
// Retina capture by default so README screenshots stay crisp on HiDPI screens.
const DEVICE_SCALE = Number(process.env.DPR ?? '2')
const VIEWPORTS = (process.env.VIEWPORTS ?? 'desktop,mobile')
  .split(',')
  .map((v) => v.trim())
  .filter(Boolean)
const MOBILE_SAMPLE = (process.env.MOBILE_SAMPLE ?? '')
  .split(',')
  .map((r) => r.trim())
  .filter(Boolean)

const DESKTOP_ROUTES = [
  '/',
  '/dashboard',
  '/dashboard/overview',
  '/dashboard/traffic',
  '/dashboard/models',
  '/dashboard/availability',
  '/sites',
  '/accounts',
  '/checkin',
  '/models',
  '/model-tester',
  '/oauth',
  '/channels',
  '/token-routes',
  '/proxy-logs',
  '/observability',
  '/price-compare',
  '/site-announcements',
  '/about',
  '/settings',
  '/settings/basic/site',
  '/settings/basic/authentication',
  '/settings/proxy-models/proxy-transport',
  '/settings/proxy-models/routing',
  '/settings/proxy-models/redirects',
  '/settings/proxy-models/rates',
  '/settings/proxy-models/allowlist',
  '/settings/proxy-models/catalog-sources',
  '/settings/downstream/keys',
  '/settings/downstream/proxy-token',
  '/settings/content/notifications',
  '/settings/content/announcements',
  '/settings/content/import-export',
  '/settings/operations/scheduling',
  '/settings/operations/database',
  '/settings/operations/data-migration',
  '/settings/operations/maintenance',
  '/settings/operations/program-logs',
  '/settings/operations/audit-logs',
  '/settings/operations/update-center',
  '/settings/operations/danger-zone',
]

const MOBILE_ROUTES = [
  '/',
  '/sites',
  '/accounts',
  '/checkin',
  '/models',
  '/model-tester',
  '/oauth',
  '/channels',
  '/token-routes',
  '/proxy-logs',
  '/observability',
  '/price-compare',
  '/site-announcements',
  '/settings',
]

const THEMES = (process.env.THEMES ?? 'light,dark').split(',')

function collectionCount(payload, label) {
  if (Array.isArray(payload)) return payload.length
  if (typeof payload?.total === 'number') return payload.total
  if (Array.isArray(payload?.items)) return payload.items.length
  throw new Error(`[screenshot] ${label} response has no countable collection`)
}

async function fetchAdminJSON(path) {
  const response = await fetch(BASE_URL + path, {
    headers: { Authorization: `Bearer ${AUTH_TOKEN}` },
  })
  if (!response.ok) {
    throw new Error(
      `[screenshot] profile preflight ${path} returned HTTP ${response.status}`
    )
  }
  return response.json()
}

async function assertDataProfile() {
  if (!EXPECTED_DATA_PROFILE) return
  if (!['empty', 'seeded'].includes(EXPECTED_DATA_PROFILE)) {
    throw new Error(
      `[screenshot] EXPECTED_DATA_PROFILE must be empty or seeded, got ${EXPECTED_DATA_PROFILE}`
    )
  }

  const [sites, accounts] = await Promise.all([
    fetchAdminJSON('/api/sites'),
    fetchAdminJSON('/api/accounts?page=1&pageSize=1'),
  ])
  const siteCount = collectionCount(sites, 'sites')
  const accountCount = collectionCount(accounts, 'accounts')
  const actual = siteCount === 0 && accountCount === 0 ? 'empty' : 'seeded'

  if (actual !== EXPECTED_DATA_PROFILE) {
    throw new Error(
      `[screenshot] data profile mismatch: expected ${EXPECTED_DATA_PROFILE}, ` +
        `observed ${actual} (sites=${siteCount}, accounts=${accountCount})`
    )
  }
  console.log(
    `[screenshot] data profile ${actual} verified (sites=${siteCount}, accounts=${accountCount})`
  )
}

function slug(route) {
  return route === '/' ? 'root' : route.replace(/^\//, '').replace(/\//g, '_')
}

async function seedAuth(context, theme) {
  await loginSession(context, { baseUrl: BASE_URL, token: AUTH_TOKEN })
  await context.addCookies([
    { name: 'vite-ui-theme', value: theme, url: BASE_URL },
  ])
  await context.addInitScript(() => {
    localStorage.setItem('i18nextLng', 'zh-CN')
  })
}

/**
 * Fail-fast guard against the stale-dist trap: navigate one authenticated
 * route and assert the SPA did NOT bounce to /sign-in. When web/dist predates
 * the session model the embedded auth guard never probes /api/auth/session
 * and redirects everything — without this check the sweep "succeeds" with a
 * full set of login-page PNGs (exit 0, evidence worthless). Throws with a
 * diagnosis naming the rebuild/restart fix.
 */
async function assertAuthPageReachable(context) {
  const page = await context.newPage()
  try {
    await page.goto(BASE_URL + '/sites', {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })
    await settle(page)
    const pathname = new URL(page.url()).pathname
    if (pathname.startsWith('/sign-in')) {
      throw new Error(
        '[screenshot] PREFLIGHT FAILED: authenticated route redirected to /sign-in — ' +
          'the served SPA likely embeds a stale web/dist (built before the session model). ' +
          'Rebuild and restart: (cd web && bun run build:web), then restart the Go server ' +
          'so the new dist is embedded. If it persists, verify BASE_URL and AUTH_TOKEN ' +
          `(cookie track for ${BASE_URL}).`
      )
    }
  } finally {
    await page.close().catch(() => {})
  }
}

async function settle(page) {
  await page.waitForLoadState('networkidle', { timeout: 6000 }).catch(() => {})
  await page.waitForTimeout(400)
}

const rows = []
const failures = []

async function capture(context, route, outFile, theme, viewport) {
  const page = await context.newPage()
  try {
    await page.goto(BASE_URL + route, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })
    await settle(page)
    await page.screenshot({ path: outFile, fullPage: true })
    rows.push({ theme, viewport, route, file: outFile })
  } catch (error) {
    const reason = String(error?.message ?? error).split('\n')[0]
    console.error(`[screenshot] FAILED ${route} -> ${outFile}: ${reason}`)
    failures.push(`${route} (${theme}/${viewport}): ${reason}`)
  } finally {
    await page.close().catch(() => {})
  }
}

async function writeManifest() {
  const lines = [
    '# Screenshot manifest',
    '',
    `Generated: ${new Date().toISOString()}`,
    `Source: screenshot-scan.mjs — base URL: ${BASE_URL} · DPR: ${DEVICE_SCALE} · themes: ${THEMES.join(', ')} · viewports: ${VIEWPORTS.join(', ')}`,
    '',
    '| Theme | Viewport | Route | File | Size (KB) | Dimensions |',
    '| --- | --- | --- | --- | ---: | --- |',
  ]
  for (const row of rows) {
    const meta = await sharp(row.file).metadata()
    const sizeKB = Math.round(statSync(row.file).size / 1024)
    lines.push(
      `| ${row.theme} | ${row.viewport} | ${row.route} | \`${basename(row.file)}\` | ${sizeKB} | ${meta.width ?? '?'}x${meta.height ?? '?'} |`
    )
  }
  lines.push(
    '',
    `Total: ${rows.length} screenshots in ${OUT_DIR}${failures.length > 0 ? `; ${failures.length} failed` : ''}.`
  )
  writeFileSync(join(OUT_DIR, 'MANIFEST.md'), lines.join('\n') + '\n')
}

async function runCapture() {
  const [{ chromium }, sharpModule] = await Promise.all([
    import('playwright'),
    import('sharp'),
  ])
  sharp = sharpModule.default

  const browser = await chromium.launch({
    headless: true,
    args: ['--no-proxy-server', '--disable-dev-shm-usage'],
  })

  const themes = THEMES.map((t) => t.trim()).filter(Boolean)
  const wantDesktop = VIEWPORTS.includes('desktop')
  const wantMobile = VIEWPORTS.includes('mobile')
  const assertAuthPageReachableMobileOnly = !wantDesktop && wantMobile

  try {
    for (const theme of themes) {
      if (wantDesktop) {
        const desktopDir = `${OUT_DIR}/${theme}/desktop`
        mkdirSync(desktopDir, { recursive: true })

        // Sign-in page (no auth token) — the first thing an operator sees.
        const authless = await browser.newContext({
          viewport: { width: 1440, height: 900 },
          deviceScaleFactor: DEVICE_SCALE,
          locale: 'zh-CN',
        })
        await authless.addCookies([
          { name: 'vite-ui-theme', value: theme, url: BASE_URL },
        ])
        await capture(
          authless,
          '/sign-in',
          `${desktopDir}/sign-in.png`,
          theme,
          'desktop'
        )
        await authless.close()

        // Desktop authenticated routes.
        const desktop = await browser.newContext({
          viewport: { width: 1440, height: 900 },
          deviceScaleFactor: DEVICE_SCALE,
          locale: 'zh-CN',
        })
        await seedAuth(desktop, theme)
        await assertAuthPageReachable(desktop)
        for (const route of DESKTOP_ROUTES) {
          await capture(
            desktop,
            route,
            `${desktopDir}/${slug(route)}.png`,
            theme,
            'desktop'
          )
        }
        await desktop.close()
      }

      if (wantMobile) {
        const mobileDir = `${OUT_DIR}/${theme}/mobile`
        mkdirSync(mobileDir, { recursive: true })
        const routes =
          MOBILE_SAMPLE.length > 0
            ? MOBILE_ROUTES.filter((r) => MOBILE_SAMPLE.includes(r))
            : MOBILE_ROUTES

        // Mobile authenticated routes.
        const mobile = await browser.newContext({
          viewport: { width: 375, height: 812 },
          deviceScaleFactor: DEVICE_SCALE,
          locale: 'zh-CN',
          isMobile: true,
        })
        await seedAuth(mobile, theme)
        // Desktop already ran the auth preflight when present; mobile-only
        // sweeps (VIEWPORTS=mobile) need it on this context instead.
        if (assertAuthPageReachableMobileOnly) {
          await assertAuthPageReachable(mobile)
        }
        for (const route of routes) {
          await capture(
            mobile,
            route,
            `${mobileDir}/${slug(route)}.png`,
            theme,
            'mobile'
          )
        }
        await mobile.close()
      }
    }
  } finally {
    await browser.close()
  }

  await writeManifest()
  const summary = `[screenshot] captured ${rows.length} screenshots into ${OUT_DIR} (themes: ${themes.join(', ')}, viewports: ${VIEWPORTS.join(', ')})`
  if (failures.length > 0) {
    console.error(`[screenshot] ${failures.length} route(s) failed:`)
    for (const f of failures) console.error('  ' + f)
    console.error(summary)
    process.exitCode = 1
    return
  }
  console.log(summary)
}

try {
  await assertDataProfile()
  if (PROFILE_CHECK_ONLY) {
    console.log('[screenshot] profile check only; capture skipped')
  } else {
    await runCapture()
  }
} catch (error) {
  console.error(String(error?.message ?? error))
  process.exitCode = 1
}
