/**
 * Chart axis/legend contrast verification (2026-08-01 chart contrast pass).
 *
 * Connects to a live single-process backend (same pattern as
 * verify-en-pages.mjs), seeds a fake site + account so Dashboard charts
 * render with data, then checks the actual <canvas> pixels:
 *   - light theme: axis labels must render in --color-text-secondary
 *     (#5f6368) — canvas cannot resolve CSS var(), so labels use the
 *     JS-resolved color from useChartColors; the old fallback would be
 *     VChart default dark #4d4d4d (invisible on dark themes).
 *   - dark theme:  axis labels must render in #9aa0a6.
 *
 * Usage (from web/):
 *   METAPI_UI_AUTH_TOKEN=<bearer> METAPI_UI_SHOT_BASE=http://127.0.0.1:4000 \
 *     node scripts/verify-chart-contrast.mjs
 *
 * Exit code 0 = both themes have axis-label pixels; 1 = missing.
 */
import { chromium } from '@playwright/test';

const base = (process.env.METAPI_UI_SHOT_BASE || '').trim().replace(/\/$/, '');
const authToken = (process.env.METAPI_UI_AUTH_TOKEN || '').trim();
if (!base || !authToken) {
  console.error('METAPI_UI_SHOT_BASE and METAPI_UI_AUTH_TOKEN are required');
  process.exit(2);
}

const TARGET = {
  light: { r: 0x5f, g: 0x63, b: 0x68, tol: 18 },
  dark: { r: 0x9a, g: 0xa0, b: 0xa6, tol: 18 },
};

/** Count pixels within tolerance of the target color across all canvases. */
async function countTargetPixels(page, target) {
  return page.evaluate(({ r, g, b, tol }) => {
    let hits = 0;
    for (const canvas of document.querySelectorAll('canvas')) {
      try {
        const ctx = canvas.getContext('2d');
        if (!ctx) continue;
        const { width, height } = canvas;
        const img = ctx.getImageData(0, 0, width, height).data;
        for (let i = 0; i < img.length; i += 4) {
          const dr = img[i] - r;
          const dg = img[i + 1] - g;
          const db = img[i + 2] - b;
          if (dr * dr + dg * dg + db * db <= tol * tol) hits++;
        }
      } catch { /* tainted/blank canvas — skip */ }
    }
    return hits;
  }, target);
}

/** Seed a fake site + account via the admin API (same as verify-en-pages). */
async function seed() {
  const headers = {
    Authorization: `Bearer ${authToken}`,
    'Content-Type': 'application/json',
  };
  const siteRes = await fetch(base + '/api/sites', {
    method: 'POST',
    headers,
    body: JSON.stringify({
      name: 'Chart Contrast Fixture',
      url: 'https://example.com',
      platform: 'new-api',
      status: 'disabled',
    }),
  });
  const site = await siteRes.json().catch(() => ({}));
  const siteId = site.site?.id ?? site.id;
  if (siteId) {
    await fetch(base + '/api/accounts', {
      method: 'POST',
      headers,
      body: JSON.stringify({ siteId, username: 'fixture-user', apiToken: 'sk-fixture', skipModelFetch: true }),
    }).catch(() => {});
  }
  console.log(`seeded site id=${siteId ?? 'n/a'} (${siteRes.status})`);
}

async function loadTheme(page, theme) {
  // init script runs before every navigation — page.evaluate on about:blank
  // has origin null and localStorage.setItem throws (silently swallowed).
  await page.addInitScript(({ theme: t, token }) => {
    try {
      localStorage.setItem('theme_mode', t);
      localStorage.setItem('app_language', 'zh');
      localStorage.setItem('auth_token', token);
      localStorage.setItem('auth_token_expires_at', String(Date.now() + 12 * 60 * 60 * 1000));
    } catch { /* ignore */ }
  }, { theme, token: authToken });
  await page.goto(base + '/', { waitUntil: 'networkidle', timeout: 30_000 }).catch(async () => {
    await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 30_000 });
  });
  await page.waitForTimeout(2500); // charts render + animations settle
}

const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await context.newPage();

await seed();

let failures = [];
for (const theme of ['light', 'dark']) {
  await loadTheme(page, theme);
  const hits = await countTargetPixels(page, TARGET[theme]);
  const status = hits > 0 ? 'ok' : 'FAIL';
  if (hits === 0) failures.push(theme);
  console.log(`[${theme}] axis-label pixels (#${TARGET[theme].r.toString(16)}...) ≈ ${hits} → ${status}`);
}

await browser.close();
if (failures.length > 0) {
  console.error(`FAIL: ${failures.join(', ')} theme(s) have no axis-label pixels`);
  process.exit(1);
}
console.log('PASS: both themes render readable axis labels');
