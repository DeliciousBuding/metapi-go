#!/usr/bin/env node
// metapi-go — real-browser screenshot harness for manual UI/UX review.
//
// Boots against a running server (BASE_URL), navigates every authenticated
// route plus the sign-in page, and saves PNG screenshots for light + dark
// themes across desktop and mobile viewports. Mirrors route-smoke.mjs auth
// seeding so the exact shipped bundle is exercised.
//
// Usage:
//   BASE_URL=http://127.0.0.1:4099 AUTH_TOKEN=dev-admin-token-123 \
//     OUT_DIR=/root/metapi-shots node scripts/screenshot-scan.mjs

import { mkdirSync } from 'node:fs'

import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const OUT_DIR = process.env.OUT_DIR ?? '/root/metapi-shots'

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
  '/fix-candidates',
  '/about',
  '/settings',
  '/settings/general/site',
  '/settings/general/authentication',
  '/settings/general/scheduling',
  '/settings/general/proxy-transport',
  '/settings/general/routing',
  '/settings/downstream/keys',
  '/settings/downstream/proxy-token',
  '/settings/models/redirects',
  '/settings/models/rates',
  '/settings/models/allowlist',
  '/settings/content/import-export',
  '/settings/content/notifications',
  '/settings/content/announcements',
  '/settings/system-info/program-logs',
  '/settings/system-info/audit-logs',
  '/settings/system-info/update-center',
  '/settings/system-info/database',
  '/settings/system-info/maintenance',
  '/settings/system-info/danger-zone',
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

function slug(route) {
  return route === '/' ? 'root' : route.replace(/^\//, '').replace(/\//g, '_')
}

async function seedAuth(context, theme) {
  await context.addCookies([
    { name: 'vite-ui-theme', value: theme, url: BASE_URL },
  ])
  await context.addInitScript(
    ({ token }) => {
      localStorage.setItem('auth_token', token)
      localStorage.setItem(
        'auth_token_expires_at',
        String(Date.now() + 12 * 3600 * 1000)
      )
      localStorage.setItem('i18nextLng', 'zh-CN')
    },
    { token: AUTH_TOKEN }
  )
}

async function settle(page) {
  await page.waitForLoadState('networkidle', { timeout: 6000 }).catch(() => {})
  await page.waitForTimeout(400)
}

async function capture(context, route, outFile) {
  const page = await context.newPage()
  try {
    await page.goto(BASE_URL + route, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })
    await settle(page)
    await page.screenshot({ path: outFile, fullPage: true })
  } catch (error) {
    console.error(
      `[screenshot] FAILED ${route} -> ${outFile}: ${String(error?.message ?? error).split('\n')[0]}`
    )
  } finally {
    await page.close().catch(() => {})
  }
}

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server', '--disable-dev-shm-usage'],
})

const themes = THEMES.map((t) => t.trim()).filter(Boolean)
let count = 0
try {
  for (const theme of themes) {
    // Sign-in page (no auth token) — the first thing an operator sees.
    const authless = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'zh-CN',
    })
    await authless.addCookies([
      { name: 'vite-ui-theme', value: theme, url: BASE_URL },
    ])
    mkdirSync(`${OUT_DIR}/${theme}/desktop`, { recursive: true })
    await capture(
      authless,
      '/sign-in',
      `${OUT_DIR}/${theme}/desktop/sign-in.png`
    )
    await authless.close()
    count++

    // Desktop authenticated routes.
    const desktop = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'zh-CN',
    })
    await seedAuth(desktop, theme)
    for (const route of DESKTOP_ROUTES) {
      await capture(
        desktop,
        route,
        `${OUT_DIR}/${theme}/desktop/${slug(route)}.png`
      )
      count++
    }
    await desktop.close()

    // Mobile authenticated routes.
    const mobile = await browser.newContext({
      viewport: { width: 375, height: 812 },
      locale: 'zh-CN',
      isMobile: true,
    })
    await seedAuth(mobile, theme)
    mkdirSync(`${OUT_DIR}/${theme}/mobile`, { recursive: true })
    for (const route of MOBILE_ROUTES) {
      await capture(
        mobile,
        route,
        `${OUT_DIR}/${theme}/mobile/${slug(route)}.png`
      )
      count++
    }
    await mobile.close()
  }
} finally {
  await browser.close()
}

console.log(
  `[screenshot] captured ${count} screenshots into ${OUT_DIR} (themes: ${themes.join(', ')})`
)
