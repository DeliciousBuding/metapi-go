#!/usr/bin/env node
// metapi-go — DOM-level UI/UX audit (no vision required).
//
// Modes:
//   default     full audit: route pass + interaction dumps + soft heuristics
//   --hard      CI gate: route pass only; fails on HARD signals exclusively
//               (console errors / pageerrors / HTTP 5xx / horizontal overflow)
//
// Hard signals stay deterministic against a fresh DB so the same script can
// gate CI (a11y job boots the real server + built SPA). Soft heuristics
// (tiny hit areas, truncation, contrast) are review aids, not gates.

import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const HARD = process.argv.includes('--hard')

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

const MOBILE_ROUTES = [
  '/',
  '/sites',
  '/accounts',
  '/models',
  '/token-routes',
  '/proxy-logs',
  '/settings',
]

const hardFailures = []
const softFindings = []

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

async function auditPage(page, label, mobile = false) {
  const errors = []
  page.on('pageerror', (e) => errors.push(`pageerror: ${e.message}`))
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(`console.error: ${m.text()}`)
  })
  page.on('response', (r) => {
    if (r.status() >= 500) {
      errors.push(`HTTP ${r.status()}: ${r.request().method()} ${r.url()}`)
    }
  })

  await page.goto(BASE_URL + label, {
    waitUntil: 'domcontentloaded',
    timeout: 20_000,
  })
  await page.waitForLoadState('networkidle', { timeout: 6000 }).catch(() => {})
  await page.waitForTimeout(400)

  const audit = await page.evaluate((isMobile) => {
    const out = {
      truncated: [],
      tinyHits: [],
      emptyLabels: [],
      dupIds: [],
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
    const de = document.documentElement
    out.overflow = { viewport: de.clientWidth, document: de.scrollWidth }

    if (!isMobile) {
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
      document.querySelectorAll('button, a, [role="button"]').forEach((el) => {
        if (!visible(el)) return
        const r = el.getBoundingClientRect()
        const name =
          el.getAttribute('aria-label') ||
          el.textContent.trim() ||
          el.getAttribute('title') ||
          ''
        if (!name) out.emptyLabels.push(`<${el.tagName.toLowerCase()}>`)
        if (
          (r.width < 22 || r.height < 22) &&
          !el.closest('[role="menuitem"]')
        ) {
          out.tinyHits.push(
            `<${el.tagName.toLowerCase()}> ${name.slice(0, 40)} ${Math.round(r.width)}x${Math.round(r.height)}`
          )
        }
      })
      document.querySelectorAll('[id]').forEach((el) => {
        seen.set(el.id, (seen.get(el.id) ?? 0) + 1)
      })
      for (const [id, n] of seen) if (n > 1) out.dupIds.push(`${id} x${n}`)
    }
    return out
  }, mobile)

  for (const e of errors)
    hardFailures.push(`${mobile ? 'mobile:' : 'light:'}${label}: ${e}`)
  if (audit.overflow && audit.overflow.document > audit.overflow.viewport + 1) {
    hardFailures.push(
      `${mobile ? 'mobile:' : 'light:'}${label}: H-OVERFLOW doc=${audit.overflow.document} > viewport=${audit.overflow.viewport}`
    )
  }

  if (!HARD) {
    for (const t of [...new Set(audit.truncated)].slice(0, 6))
      softFindings.push(`light:${label}: TRUNCATED "${t}"`)
    for (const t of [...new Set(audit.emptyLabels)].slice(0, 6))
      softFindings.push(`light:${label}: EMPTY-LABEL ${t}`)
    for (const t of [...new Set(audit.tinyHits)].slice(0, 10))
      softFindings.push(`light:${label}: TINY-HIT ${t}`)
    for (const t of audit.dupIds.slice(0, 4))
      softFindings.push(`light:${label}: DUP-ID ${t}`)
  }

  // Contrast pass (desktop only, non-hard).
  if (!HARD && !mobile) {
    const samples = await page.evaluate(() => {
      const bg = (el) => {
        let node = el
        while (node && node !== document.documentElement) {
          const c = getComputedStyle(node).backgroundColor
          if (c && c !== 'rgba(0, 0, 0, 0)' && c !== 'transparent') return c
          node = node.parentElement
        }
        return getComputedStyle(document.body).backgroundColor
      }
      const out = []
      document
        .querySelectorAll('p, span, div, td, th, label, a, button, li')
        .forEach((el) => {
          if (el.children.length > 0) return
          const r = el.getBoundingClientRect()
          if (r.width === 0 || r.height === 0) return
          const s = getComputedStyle(el)
          if (s.fontSize === '0px') return
          const text = el.textContent.trim()
          if (!text || text.length > 40) return
          out.push({
            text: text.slice(0, 30),
            color: s.color,
            bg: bg(el),
            fontSize: parseFloat(s.fontSize),
          })
        })
      return out.slice(0, 300)
    })
    const low = []
    for (const s of samples) {
      const fg = parseColor(s.color)
      const bgc = parseColor(s.bg)
      if (!fg || !bgc) continue
      const ratio = contrast(fg, bgc)
      const threshold = s.fontSize >= 18 ? 3 : 4.5
      if (ratio < threshold)
        low.push(`${ratio.toFixed(2)} "${s.text}" fg=${s.color} bg=${s.bg}`)
    }
    if (low.length) {
      softFindings.push(`light:${label}: LOW CONTRAST x${low.length}:`)
      for (const c of [...new Set(low)].slice(0, 8))
        softFindings.push(`    ${c}`)
    }
  }
}

/** Dump the accessible structure of whatever dialog/sheet is currently open. */
async function dumpDialogSurface(page, label) {
  await page.waitForTimeout(700)
  const text = await page.evaluate(() => {
    const dialog = document.querySelector(
      '[role="dialog"], [data-slot="dialog-content"], [data-slot="sheet-content"]'
    )
    if (!dialog) return null
    const heading =
      dialog
        .querySelector(
          'h2, [data-slot="dialog-title"], [data-slot="sheet-title"]'
        )
        ?.textContent.trim() ?? ''
    const labels = [...dialog.querySelectorAll('label')]
      .map((l) => l.textContent.trim())
      .filter(Boolean)
      .slice(0, 40)
    const inputs = [...dialog.querySelectorAll('input, select, textarea')]
      .map((i) => ({
        type: i.type || i.tagName,
        placeholder: i.placeholder?.slice(0, 40),
        name: i.name,
      }))
      .slice(0, 40)
    const buttons = [...dialog.querySelectorAll('button')]
      .map((b) => b.textContent.trim())
      .filter(Boolean)
      .slice(0, 20)
    return { heading, labels, inputs, buttons }
  })
  console.log(`\n=== ${label} ===`)
  if (!text) {
    console.log('(no dialog surface opened)')
    return
  }
  console.log(`heading: ${text.heading || '(none)'}`)
  console.log(`labels: ${JSON.stringify(text.labels)}`)
  console.log(`inputs: ${JSON.stringify(text.inputs)}`)
  console.log(`buttons: ${JSON.stringify(text.buttons)}`)
}

async function interactionPass(desktop) {
  async function openRowMenuThenDetail(route, menuLabel, detailLabel) {
    const page = await desktop.newPage()
    await page.goto(`${BASE_URL}${route}`, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })
    await page
      .waitForLoadState('networkidle', { timeout: 6000 })
      .catch(() => {})
    await page.waitForTimeout(600)
    try {
      const trigger = page.getByRole('button', { name: menuLabel }).first()
      await trigger.waitFor({ state: 'visible', timeout: 6000 })
      await trigger.click()
      await page.waitForTimeout(400)
      const items = await page.evaluate(() =>
        [...document.querySelectorAll('[role="menuitem"]')]
          .map((m) => m.textContent.trim())
          .filter(Boolean)
          .slice(0, 12)
      )
      console.log(`\n=== ${route} row menu items ===`)
      console.log(JSON.stringify(items))
      const detail = page.getByRole('menuitem', { name: detailLabel }).first()
      if (await detail.count()) {
        await detail.click()
        await dumpDialogSurface(page, `${route} detail sheet`)
      }
    } catch (e) {
      console.log(
        `${route} interaction skipped: ${String(e?.message ?? e).split('\n')[0]}`
      )
    }
    await page.close()
  }

  // 1. Accounts detail sheet.
  await openRowMenuThenDetail('/accounts', /账号操作/, /详情/)
  // 2. Models detail sheet.
  await openRowMenuThenDetail('/models', /行操作/, /查看详情/)
  // 3. Token-routes detail sheet.
  await openRowMenuThenDetail('/token-routes', /路由操作/, /详情/)

  // 4. Sites import wizard (toolbar entry is now always present).
  {
    const page = await desktop.newPage()
    await page.goto(`${BASE_URL}/sites`, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })
    await page
      .waitForLoadState('networkidle', { timeout: 6000 })
      .catch(() => {})
    try {
      const importBtn = page.getByRole('button', { name: /导入站点/ }).first()
      await importBtn.waitFor({ state: 'visible', timeout: 6000 })
      await importBtn.click()
      await dumpDialogSurface(page, 'import wizard (first step)')
    } catch (e) {
      console.log(
        `import wizard skipped: ${String(e?.message ?? e).split('\n')[0]}`
      )
    }
    await page.close()
  }

  // 5. Downstream keys create dialog.
  {
    const page = await desktop.newPage()
    await page.goto(`${BASE_URL}/settings/downstream/keys`, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })
    await page
      .waitForLoadState('networkidle', { timeout: 6000 })
      .catch(() => {})
    try {
      const create = page
        .getByRole('button', { name: /创建|新建|Create/ })
        .first()
      await create.waitFor({ state: 'visible', timeout: 6000 })
      await create.click()
      await dumpDialogSurface(page, 'downstream key create dialog')
    } catch (e) {
      console.log(
        `key create skipped: ${String(e?.message ?? e).split('\n')[0]}`
      )
    }
    await page.close()
  }

  // 6. Global search modal (Ctrl+K).
  {
    const page = await desktop.newPage()
    await page.goto(`${BASE_URL}/dashboard`, {
      waitUntil: 'domcontentloaded',
      timeout: 20_000,
    })
    await page
      .waitForLoadState('networkidle', { timeout: 6000 })
      .catch(() => {})
    await page.keyboard.press('Control+k')
    await page.waitForTimeout(700)
    const search = await page.evaluate(() => {
      const modal = document.querySelector(
        '[role="dialog"], [data-slot="command-menu"], [data-slot="command"]'
      )
      if (!modal) return null
      const items = [...modal.querySelectorAll('[role="option"], [data-value]')]
        .map((el) => el.textContent.trim().slice(0, 60))
        .filter(Boolean)
        .slice(0, 15)
      const input = modal.querySelector('input')
      return {
        placeholder: input?.placeholder?.slice(0, 40) ?? '',
        items,
      }
    })
    console.log('\n=== global search modal ===')
    if (!search) {
      console.log('(no search modal found)')
    } else {
      console.log(`placeholder: ${search.placeholder}`)
      console.log(`items: ${JSON.stringify(search.items)}`)
    }
    await page.close()
  }
}

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server', '--disable-dev-shm-usage'],
})
try {
  const desktop = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: 'zh-CN',
  })
  await seedAuth(desktop)
  for (const route of ROUTES) {
    const page = await desktop.newPage()
    await auditPage(page, route, false)
    await page.close()
  }
  if (!HARD) await interactionPass(desktop)
  await desktop.close()

  const mobile = await browser.newContext({
    viewport: { width: 375, height: 812 },
    locale: 'zh-CN',
    isMobile: true,
  })
  await seedAuth(mobile)
  for (const route of MOBILE_ROUTES) {
    const page = await mobile.newPage()
    await auditPage(page, route, true)
    await page.close()
  }
  await mobile.close()
} finally {
  await browser.close()
}

if (hardFailures.length > 0) {
  console.error(`\n=== HARD FAILURES (${hardFailures.length}) ===`)
  for (const f of [...new Set(hardFailures)]) console.error(`  ${f}`)
  process.exit(1)
}

if (!HARD && softFindings.length > 0) {
  console.log(`\n=== SOFT FINDINGS (${softFindings.length}) ===`)
  for (const f of [...new Set(softFindings)]) console.log(`  ${f}`)
}

console.log(
  `[ui-audit] ${HARD ? 'hard gate clean' : 'audit complete'} — ${ROUTES.length} desktop routes + ${MOBILE_ROUTES.length} mobile routes.`
)
