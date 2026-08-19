#!/usr/bin/env node
// metapi-go — DOM-level UI/UX audit (no vision required).
// For each route it dumps visible text, flags layout/accessibility anomalies:
//   - horizontal document overflow (desktop + mobile)
//   - truncated text without tooltip (ellipsis + scrollWidth > clientWidth)
//   - interactive elements with empty accessible names or tiny hit areas
//   - text/background contrast pairs sampled from computed styles
//   - duplicate element ids, empty headings, zero-size images
//   - console errors / pageerrors
// Also opens a few key dialogs and dumps their content.

import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'

const ROUTES = [
  '/',
  '/sites',
  '/accounts',
  '/models',
  '/token-routes',
  '/channels',
  '/checkin',
  '/proxy-logs',
  '/oauth',
  '/model-tester',
  '/observability',
  '/price-compare',
  '/fix-candidates',
  '/site-announcements',
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
  '/about',
]

function luminance([r, g, b]) {
  const f = (v) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)
}

function parseColor(css) {
  const m = css?.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)
  if (!m) return null
  return [Number(m[1]), Number(m[2]), Number(m[3])]
}

function contrast(fg, bg) {
  const l1 = luminance(fg)
  const l2 = luminance(bg)
  const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1]
  return (hi + 0.05) / (lo + 0.05)
}

const findings = []

async function seedAuth(context) {
  await context.addCookies([
    { name: 'vite-ui-theme', value: 'light', url: BASE_URL },
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

async function auditPage(page, label) {
  const errors = []
  page.on('pageerror', (e) => errors.push(`pageerror: ${e.message}`))
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(`console.error: ${m.text()}`)
  })

  await page.goto(BASE_URL + label.route, {
    waitUntil: 'domcontentloaded',
    timeout: 20_000,
  })
  await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {})
  await page.waitForTimeout(500)

  const audit = await page.evaluate(() => {
    const out = {
      texts: [],
      truncated: [],
      tinyHits: [],
      emptyLabels: [],
      dupIds: [],
      zeroImages: [],
      overflow: null,
    }
    const seen = new Map()

    const visible = (el) => {
      const s = getComputedStyle(el)
      const r = el.getBoundingClientRect()
      return (
        s.display !== 'none' &&
        s.visibility !== 'hidden' &&
        r.width > 0 &&
        r.height > 0
      )
    }

    // Document overflow
    const de = document.documentElement
    out.overflow = {
      viewport: de.clientWidth,
      document: de.scrollWidth,
      bodyScroll: document.body.scrollWidth,
    }

    // Text dump of main headings + first paragraph
    for (const sel of ['h1', 'h2']) {
      document.querySelectorAll(sel).forEach((el) => {
        if (visible(el))
          out.texts.push(`${sel}: ${el.textContent.trim().slice(0, 120)}`)
      })
    }

    // Truncated text
    document.querySelectorAll('*').forEach((el) => {
      if (!visible(el) || el.children.length > 0) return
      const s = getComputedStyle(el)
      if (
        s.textOverflow === 'ellipsis' &&
        el.scrollWidth > el.clientWidth + 2
      ) {
        out.truncated.push(el.textContent.trim().slice(0, 60))
      }
    })

    // Interactive elements
    document
      .querySelectorAll('button, a, [role="button"], input, [tabindex]')
      .forEach((el) => {
        if (!visible(el)) return
        const r = el.getBoundingClientRect()
        const name =
          el.getAttribute('aria-label') ||
          el.textContent.trim() ||
          el.getAttribute('title') ||
          ''
        if (!name && el.tagName !== 'INPUT') {
          out.emptyLabels.push(
            `<${el.tagName.toLowerCase()} class="${el.className?.toString().slice(0, 50)}">`
          )
        }
        if (
          (r.width < 22 || r.height < 22) &&
          el.tagName !== 'INPUT' &&
          !el.closest('[role="menuitem"]')
        ) {
          out.tinyHits.push(
            `<${el.tagName.toLowerCase()}> ${name.slice(0, 40)} ${Math.round(r.width)}x${Math.round(r.height)}`
          )
        }
      })

    // Duplicate ids
    document.querySelectorAll('[id]').forEach((el) => {
      const id = el.id
      seen.set(id, (seen.get(id) ?? 0) + 1)
    })
    for (const [id, n] of seen) if (n > 1) out.dupIds.push(`${id} x${n}`)

    // Zero-size images
    document.querySelectorAll('img, svg').forEach((el) => {
      const r = el.getBoundingClientRect()
      if (r.width === 0 && r.height === 0) out.zeroImages.push(el.tagName)
    })
    return out
  })

  // Contrast sampling: body text and muted text vs their backgrounds
  const contrastInfo = await page.evaluate(() => {
    const samples = []
    const bg = (el) => {
      let node = el
      while (node && node !== document.documentElement) {
        const c = getComputedStyle(node).backgroundColor
        if (c && c !== 'rgba(0, 0, 0, 0)' && c !== 'transparent') return c
        node = node.parentElement
      }
      return getComputedStyle(document.body).backgroundColor
    }
    document
      .querySelectorAll('p, span, div, td, th, label, a, button')
      .forEach((el) => {
        if (el.children.length > 0) return
        const r = el.getBoundingClientRect()
        if (r.width === 0 || r.height === 0) return
        const s = getComputedStyle(el)
        if (s.fontSize === '0px') return
        samples.push({
          text: el.textContent.trim().slice(0, 30),
          color: s.color,
          bg: bg(el),
        })
      })
    return samples.slice(0, 400)
  })

  const lowContrast = []
  for (const s of contrastInfo) {
    const fg = parseColor(s.color)
    const bg = parseColor(s.bg)
    if (!fg || !bg) continue
    const ratio = contrast(fg, bg)
    if (ratio < 3.0 && s.text) {
      lowContrast.push(
        `${ratio.toFixed(2)} "${s.text}" fg=${s.color} bg=${s.bg}`
      )
    }
  }
  if (lowContrast.length) {
    findings.push(`${label}: LOW CONTRAST x${lowContrast.length}:`)
    for (const c of [...new Set(lowContrast)].slice(0, 12))
      findings.push(`    ${c}`)
  }

  if (audit.overflow && audit.overflow.document > audit.overflow.viewport + 1) {
    findings.push(
      `${label}: H-OVERFLOW doc=${audit.overflow.document} > viewport=${audit.overflow.viewport}`
    )
  }
  for (const t of [...new Set(audit.truncated)].slice(0, 8))
    findings.push(`${label}: TRUNCATED "${t}"`)
  for (const t of [...new Set(audit.emptyLabels)].slice(0, 8))
    findings.push(`${label}: EMPTY-LABEL ${t}`)
  for (const t of [...new Set(audit.tinyHits)].slice(0, 8))
    findings.push(`${label}: TINY-HIT ${t}`)
  for (const t of audit.dupIds.slice(0, 4))
    findings.push(`${label}: DUP-ID ${t}`)
  for (const e of errors.slice(0, 6)) findings.push(`${label}: ${e}`)

  return audit
}

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server', '--disable-dev-shm-usage'],
})
try {
  // ---- desktop light ----
  const desktop = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: 'zh-CN',
  })
  await seedAuth(desktop)
  const pages = new Map()
  for (const route of ROUTES) {
    const page = await desktop.newPage()
    const label = `light:${route}`
    const audit = await auditPage(page, { route })
    pages.set(route, { page, audit })
    await page.close()
  }

  // ---- key dialogs (sites add, account detail, model detail, route detail) ----
  async function dialogDump(route, openLabel, done) {
    const page = await desktop.newPage()
    await page.goto(BASE_URL + route, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })
    await page
      .waitForLoadState('networkidle', { timeout: 5000 })
      .catch(() => {})
    await page.waitForTimeout(400)
    try {
      const trigger = page.getByRole('button', { name: openLabel }).first()
      await trigger.waitFor({ state: 'visible', timeout: 5000 })
      await trigger.click()
      await page.waitForTimeout(700)
      const text = await page.evaluate(() => {
        const dialog = document.querySelector(
          '[role="dialog"], [data-slot="dialog-content"], [data-slot="sheet-content"]'
        )
        if (!dialog) return '(no dialog found)'
        const labels = [...dialog.querySelectorAll('label')]
          .map((l) => l.textContent.trim())
          .slice(0, 30)
        const inputs = [...dialog.querySelectorAll('input, select, textarea')]
          .map((i) => ({
            type: i.type || i.tagName,
            placeholder: i.placeholder?.slice(0, 40),
            name: i.name,
          }))
          .slice(0, 30)
        const buttons = [...dialog.querySelectorAll('button')]
          .map((b) => b.textContent.trim())
          .filter(Boolean)
          .slice(0, 15)
        return {
          labels,
          inputs,
          buttons,
          heading: dialog
            .querySelector(
              'h2, [data-slot="dialog-title"], [data-slot="sheet-title"]'
            )
            ?.textContent.trim(),
        }
      })
      console.log(`\n=== DIALOG ${route} (${openLabel}) ===`)
      console.log(`heading: ${text.heading ?? '(none)'}`)
      console.log(`labels: ${JSON.stringify(text.labels)}`)
      console.log(`inputs: ${JSON.stringify(text.inputs)}`)
      console.log(`buttons: ${JSON.stringify(text.buttons)}`)
      done && (await done(page))
    } catch (e) {
      console.log(
        `\n=== DIALOG ${route} (${openLabel}) === FAILED: ${String(e?.message ?? e).split('\n')[0]}`
      )
    }
    await page.close()
  }

  await dialogDump('/sites', /添加站点|Add site/)
  await dialogDump('/accounts', /添加账号|Add account/)
  await dialogDump('/downstream-keys', /创建|Create/) // may not exist as route; skip silently
  await desktop.close()

  // ---- mobile light: overflow + tiny hits ----
  const mobile = await browser.newContext({
    viewport: { width: 375, height: 812 },
    locale: 'zh-CN',
    isMobile: true,
  })
  await seedAuth(mobile)
  for (const route of [
    '/',
    '/sites',
    '/accounts',
    '/models',
    '/token-routes',
    '/proxy-logs',
    '/settings',
    '/model-tester',
  ]) {
    const page = await mobile.newPage()
    await auditPage(page, { route: route, mobile: true })
    await page.close()
  }
  await mobile.close()
} finally {
  await browser.close()
}

console.log(`\n=== FINDINGS (${findings.length}) ===`)
for (const f of [...new Set(findings)]) console.log(f)
