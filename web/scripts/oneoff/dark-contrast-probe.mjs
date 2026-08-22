#!/usr/bin/env node
// Dark-mode contrast probe: samples text/background pairs on key pages and
// reports WCAG AA failures (ratio < 4.5 for normal text, < 3 for large).
import { chromium } from 'playwright'

const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const AUTH_TOKEN = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const ROUTES = [
  '/',
  '/sites',
  '/accounts',
  '/models',
  '/token-routes',
  '/proxy-logs',
  '/settings',
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

const browser = await chromium.launch({
  headless: true,
  args: ['--no-proxy-server'],
})
const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  locale: 'zh-CN',
})
await context.addCookies([
  { name: 'vite-ui-theme', value: 'dark', url: BASE_URL },
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

for (const route of ROUTES) {
  const page = await context.newPage()
  await page.goto(BASE_URL + route, {
    waitUntil: 'domcontentloaded',
    timeout: 20000,
  })
  await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {})
  await page.waitForTimeout(500)
  // List pages sync list state into the URL (e.g. /sites -> ?page=0&pageSize=20)
  // right after mount, which destroys the evaluation context mid-run. Retry
  // sampling once the navigation settles instead of crashing.
  let samples = []
  let sampleError = null
  for (let attempt = 0; attempt < 3 && samples.length === 0; attempt += 1) {
    if (attempt > 0) await page.waitForTimeout(600)
    try {
      samples = await page.evaluate(() => {
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
      sampleError = null
    } catch (error) {
      // Navigation raced the evaluate; retry after the route settles.
      sampleError = error
    }
  }
  if (samples.length === 0) {
    // Never emit a false "ok": an un-sampled route is an audit failure.
    const reason = sampleError
      ? String(sampleError.message ?? sampleError).split('\n')[0]
      : 'no text samples collected'
    console.log(`[${route}] SAMPLE-FAILED (${reason})`)
    process.exitCode = 1
    await page.close()
    continue
  }
  const fails = []
  for (const s of samples) {
    const fg = parseColor(s.color)
    const bgc = parseColor(s.bg)
    if (!fg || !bgc) continue
    const ratio = contrast(fg, bgc)
    const threshold = s.fontSize >= 18 ? 3 : 4.5
    if (ratio < threshold)
      fails.push(`${ratio.toFixed(2)} "${s.text}" fg=${s.color} bg=${s.bg}`)
  }
  const unique = [...new Set(fails)]
  if (unique.length) {
    console.log(`\n[${route}] dark-mode low contrast x${unique.length}:`)
    for (const f of unique.slice(0, 10)) console.log(`  ${f}`)
  } else {
    console.log(`[${route}] ok`)
  }
  await page.close()
}
await browser.close()
