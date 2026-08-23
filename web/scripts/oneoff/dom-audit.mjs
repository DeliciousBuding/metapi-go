#!/usr/bin/env node
// metapi-go — DOM-level UI/UX audit (no vision required).
//
// Modes:
//   default     full audit: route pass + interaction dumps + soft heuristics
//   --hard      CI gate: route pass only; fails on HARD signals exclusively
//               (console errors / pageerrors / HTTP 5xx / horizontal overflow
//                / occlusion / undersized hit targets)
//
// Hard signals stay deterministic against a fresh DB so the same script can
// gate CI (a11y job boots the real server + built SPA). Soft heuristics
// (tiny hit areas, truncation, contrast) are review aids, not gates.
//
// Wave 7: exported `scanOcclusion` (5-point elementFromPoint probe) and
// `scanHitTargets` (24×24 WCAG 2.5.8 minimum) are hard signals, see their
// header comments. They are exported (and the driver is guarded by isMain)
// so the standalone per-route testbench can run the SAME detection code via
// function-source eval, avoiding copy-drifting detector logic.

import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

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

// ---------------------------------------------------------------------------
// Hard signal 1 — occlusion: an interactive element whose paint is covered by
// a different element (clicks land on something else, "button blocked").
// 5-point probe strategy (rect centre + 4 corners, adapted from the rlint /
// render-qa audits): at each point we ask document.elementFromPoint() who
// actually owns the click; a point is blocked evidence when its owner is
// neither the target itself nor one of its descendants. One blocked point
// turns the element into an H-OCCLUDED item (pts=blocked/checked shows the
// severity + the hit point).
//
// Deliberate exemptions (each is "not a defect", rationale in-line):
//   * target has pointer-events:none          — inert by design, no click to
//     lose.
//   * target fully outside the viewport       — invisible means unclickable
//     anyway; H-OVERFLOW owns the horizontal case and off-screen rows are
//     covered by the mobile route pass.
//   * hit is an ANCESTOR of the target        — degenerate paint-out (parent
//     with overflow:hidden clipping a rounded corner). An ancestor can never
//     paint above its own descendant, so this is stacking, not occlusion.
//   * hit has pointer-events:none             — never intercepts input; this
//     check is belt-and-braces (elementFromPoint already skips such layers),
//     and it directly implements the "pointer-events:none overlay is not an
//     occlusion" negative test.
//   * hit is a sibling <label> resolving to the target (for= or wrapping) —
//     the label is the DESIGNED hit target of its control, not a blocker.
//   * hit belongs to a different open overlay layer than the target: modal /
//     sheet popups, pickers, menus and toasts intentionally stack above the
//     page ("recently-opened modal/dialog coverage" exemption — implemented
//     as a layer IDENTITY comparison rather than a raw dialog-rect union so
//     a full-viewport backdrop counts once as one layer regardless of its
//     size, while a genuinely broken stacking INSIDE a layer still gets
//     reported). Layer membership reuses the semantic markers this codebase
//     emits: dialog/alertdialog/menu/listbox roles + the base-ui data-slot
//     dialog/sheet/alert-dialog/dropdown-menu/popover/select/command/tooltip
//     layers + sonner toasts. Layers without size are closed portals (base-ui
//     unmounts closed popups) and don't apply.
//   * target itself is inside a toast region  — transient by design, it
//     disappears on its own; no user interaction is being lost.
//
// Self-contained by design: the driver passes the function into
// page.evaluate(), and the testbench re-evaluates its source — nothing may
// close over module state.
// ---------------------------------------------------------------------------
export function scanOcclusion() {
  const INTERACTIVE =
    'a[href], button, input, select, textarea, [role="button"], [role="link"], ' +
    '[role="tab"], [role="switch"], [role="checkbox"], [role="radio"], ' +
    '[role="option"], [role="menuitem"], [role="menuitemcheckbox"], ' +
    '[role="menuitemradio"], [onclick], [onpointerdown], [onmousedown], ' +
    '[contenteditable="true"]'
  const OVERLAY =
    'dialog, [role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"], ' +
    '[data-slot="dialog-overlay"], [data-slot="dialog-content"], ' +
    '[data-slot="sheet-overlay"], [data-slot="sheet-content"], ' +
    '[data-slot="alert-dialog-overlay"], [data-slot="alert-dialog-content"], ' +
    '[data-slot="dropdown-menu-content"], [data-slot="select-content"], ' +
    '[data-slot="popover-content"], [data-slot="command"], ' +
    '[data-slot="tooltip-content"], [data-slot="toast"], [data-sonner-toast]'
  const vw = document.documentElement.clientWidth
  const vh = document.documentElement.clientHeight
  const describe = (el) => {
    const parts = [el.tagName.toLowerCase()]
    if (el.id) parts.push(`#${el.id}`)
    else if (el.getAttribute('aria-label')) {
      parts.push(`[aria-label="${el.getAttribute('aria-label').slice(0, 24)}"]`)
    } else {
      for (const c of el.classList) {
        if (/^[a-zA-Z][\w-]{0,24}$/.test(c)) parts.push(`.${c}`)
        if (parts.length >= 3) break
      }
    }
    const name =
      el.getAttribute('aria-label') ||
      el.title ||
      el.textContent.trim().replace(/\s+/g, ' ').slice(0, 32)
    return `${parts.join('')}${name ? ` "${name}"` : ''}`
  }
  const findOverlay = (el) => {
    if (!el || el.nodeType !== 1) return null
    const root = el.matches(OVERLAY) ? el : el.closest(OVERLAY)
    if (!root) return null
    const r = root.getBoundingClientRect()
    if (r.width <= 0 || r.height <= 0) return null // closed/unmounted layer
    return root
  }
  const findings = []
  for (const el of document.querySelectorAll(INTERACTIVE)) {
    if (el.closest('[aria-hidden="true"]')) continue
    if (el.disabled) continue
    const cs = getComputedStyle(el)
    if (cs.display === 'none' || cs.visibility === 'hidden') continue
    if (cs.pointerEvents === 'none') continue
    const r = el.getBoundingClientRect()
    if (r.width <= 0 || r.height <= 0) continue
    if (r.right <= 0 || r.left >= vw || r.bottom <= 0 || r.top >= vh) continue
    if (el.closest('[data-sonner-toast], [data-slot="toast"]')) continue
    const targetLayer = findOverlay(el)
    const pts = [
      [r.left + r.width / 2, r.top + r.height / 2],
      [r.left, r.top],
      [r.right - 0.5, r.top],
      [r.left, r.bottom - 0.5],
      [r.right - 0.5, r.bottom - 0.5],
    ]
    let checked = 0
    const blocked = new Map() // blocker el -> first [x, y]
    for (const [x, y] of pts) {
      if (x < 0 || y < 0 || x >= vw || y >= vh) continue
      checked++
      const hit = document.elementFromPoint(x, y)
      if (!hit || hit === el || el.contains(hit)) continue
      if (hit.contains(el)) continue // ancestor paint-out — see header
      if (getComputedStyle(hit).pointerEvents === 'none') continue
      if (hit.tagName === 'LABEL') {
        const ctrl = hit.htmlFor
          ? document.getElementById(hit.htmlFor)
          : hit.querySelector('input, select, button, textarea')
        if (ctrl === el) continue
      }
      const hitLayer = findOverlay(hit)
      if (hitLayer && hitLayer !== targetLayer) continue
      if (!blocked.has(hit)) blocked.set(hit, [x, y])
    }
    if (blocked.size === 0) continue
    const [blocker, [x, y]] = blocked.entries().next().value
    findings.push({
      el: describe(el),
      blocker: describe(blocker),
      pts: `${blocked.size}/${checked}`,
      at: `(${Math.round(x)},${Math.round(y)})`,
    })
  }
  return findings
}

// ---------------------------------------------------------------------------
// Hard signal 2 — hit target size: an interactive element whose bounding box
// is smaller than 24x24 px (WCAG 2.5.8 minimum, the desktop rule). The rect
// is the CONTROL BLOCK rect: a small icon inside a padded <button> passes
// naturally because the button (not the icon) is measured — icon-only Button
// sizes are size-6/7/8 = 24–32px, so table row-action icon buttons are fine
// by design (their padding IS the hit area, per the Wave 5 deep-audit fix).
//
// Exemptions (render-qa style, rationale in-line):
//   * inline text links — a bare-text <a>/[role="link"] with no element
//     children and CSS display:inline is a sentence fragment (running copy),
//     not a standalone control; demanding 24px there kills the copy.
//   * checkable controls — native input[type=checkbox|radio] AND the chosen
//     base-ui widget (role="checkbox"/"radio", 16px box): the component
//     expands the clickable area with a ::after hit-region
//     (-inset-x-3/-inset-y-2 => >=40px) and the row label is the designed
//     target; a rect-based gate would only produce false positives.
//   * option / role="option" / role="menuitem*" — dense picker & menu items;
//     density is intentional (Wave 5 already dropped menuitems from tinyHits
//     as noise for the same reason).
//
// Mobile (viewport <= 640px): 44x44 (WCAG 2.5.5 AAA) is too strict here — the
// app marks coarse-pointer targets with data-hit-area and the component CSS
// expands their tap target to >=40px, so a bare 44 rule would flood the
// report with designed controls. Decision kept from the wave spec: mobile
// hard threshold stays 24 (= AA), and only 24–44 elements WITHOUT a
// data-hit-area expansion are reported as SOFT review aids.
// ---------------------------------------------------------------------------
export function scanHitTargets(isMobile) {
  const INTERACTIVE =
    'a[href], button, input, select, textarea, [role="button"], [role="link"], ' +
    '[role="tab"], [role="switch"], [role="checkbox"], [role="radio"], ' +
    '[role="option"], [role="menuitem"], [role="menuitemcheckbox"], ' +
    '[role="menuitemradio"], [onclick], [onpointerdown], [onmousedown], ' +
    '[contenteditable="true"]'
  const describe = (el) => {
    const parts = [el.tagName.toLowerCase()]
    if (el.id) parts.push(`#${el.id}`)
    else if (el.getAttribute('aria-label')) {
      parts.push(`[aria-label="${el.getAttribute('aria-label').slice(0, 24)}"]`)
    } else {
      for (const c of el.classList) {
        if (/^[a-zA-Z][\w-]{0,24}$/.test(c)) parts.push(`.${c}`)
        if (parts.length >= 3) break
      }
    }
    const name =
      el.getAttribute('aria-label') ||
      el.title ||
      el.textContent.trim().replace(/\s+/g, ' ').slice(0, 32)
    return `${parts.join('')}${name ? ` "${name}"` : ''}`
  }
  const hard = []
  const nearMiss = [] // mobile 24–44 without coarse-pointer expansion
  for (const el of document.querySelectorAll(INTERACTIVE)) {
    if (el.closest('[aria-hidden="true"]')) continue
    if (el.disabled) continue
    const cs = getComputedStyle(el)
    if (cs.display === 'none' || cs.visibility === 'hidden') continue
    if (cs.pointerEvents === 'none') continue
    const r = el.getBoundingClientRect()
    if (r.width <= 0 || r.height <= 0) continue
    const w = Math.round(r.width)
    const h = Math.round(r.height)
    if (w >= 24 && h >= 24) {
      if (
        isMobile &&
        (w < 44 || h < 44) &&
        // data-hit-area: component-level coarse-pointer expansion (>=40px).
        // data-slot=sidebar: nav rail items are intentionally dense and
        // already covered by the <24 hard rule — soft-ordering them again
        // would double-report every settings page.
        !el.matches('[data-hit-area]') &&
        !el.closest('[data-slot="sidebar"]')
      ) {
        nearMiss.push(`${describe(el)} ${w}x${h}`)
      }
      continue
    }
    // Below the 24 hard threshold — check the exemption list:
    if (
      (el.tagName === 'A' || el.matches('[role="link"]')) &&
      !el.firstElementChild &&
      cs.display === 'inline'
    ) {
      continue // inline text link inside running copy
    }
    if (el.matches('input[type="checkbox"], input[type="radio"]')) continue
    if (el.matches('[role="checkbox"], [role="radio"]')) continue
    if (el.matches('option, [role="option"]')) continue
    if (
      el.matches(
        '[role="menuitem"], [role="menuitemcheckbox"], [role="menuitemradio"]'
      )
    ) {
      continue
    }
    hard.push(`${describe(el)} ${w}x${h}`)
  }
  return { hard, nearMiss }
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

  const mode = mobile ? 'mobile:' : 'light:'

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

  for (const e of errors) hardFailures.push(`${mode}${label}: ${e}`)
  if (audit.overflow && audit.overflow.document > audit.overflow.viewport + 1) {
    hardFailures.push(
      `${mode}${label}: H-OVERFLOW doc=${audit.overflow.document} > viewport=${audit.overflow.viewport}`
    )
  }

  // Wave 7 hard signals — occlusion + hit target size (both also gate the
  // --hard mode: they land in hardFailures unconditionally).
  const occluded = await page.evaluate(scanOcclusion)
  const targets = await page.evaluate(scanHitTargets, mobile)
  const MAX_ROUTE = 12
  if (occluded.length) {
    for (const [i, f] of occluded.entries()) {
      if (i === MAX_ROUTE) {
        hardFailures.push(
          `${mode}${label}: H-OCCLUDED +${occluded.length - MAX_ROUTE} more`
        )
        break
      }
      hardFailures.push(
        `${mode}${label}: H-OCCLUDED ${f.el} pts=${f.pts} blocked by ${f.blocker} @${f.at}`
      )
    }
  }
  if (targets.hard.length) {
    for (const [i, t] of targets.hard.entries()) {
      if (i === MAX_ROUTE) {
        hardFailures.push(
          `${mode}${label}: H-HITAREA +${targets.hard.length - MAX_ROUTE} more`
        )
        break
      }
      hardFailures.push(`${mode}${label}: H-HITAREA ${t}`)
    }
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
    for (const t of [...new Set(targets.nearMiss)].slice(0, 10))
      softFindings.push(`${mode}${label}: SMALL-HIT ${t}`)
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

async function main() {
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
}

// Direct execution (`node dom-audit.mjs`) runs the audit; imported (by the
// per-route testbench) it only exposes the detectors with the same source.
const isMain =
  process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)
if (isMain) await main()
