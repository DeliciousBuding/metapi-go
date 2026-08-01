/**
 * EN-mode main-surface verification (2026-08-01 e2e wave).
 *
 * Connects to a live single-process backend (embedded SPA, same origin —
 * same pattern as capture-ui-shots.mjs SHOT-1) and walks the main admin
 * routes in EN mode, asserting:
 *   - no `Untranslated` fallback text anywhere in the body
 *   - no Han residue in UI containers (page body; user-data cells are
 *     data-i18n-skip exempt by design and not asserted here)
 *
 * Usage (from web/):
 *   METAPI_UI_AUTH_TOKEN=<bearer> METAPI_UI_SHOT_BASE=http://127.0.0.1:4000 \
 *     node scripts/verify-en-pages.mjs
 *
 * Exit code 0 = all routes clean; 1 = at least one route had residue.
 */
import { chromium } from '@playwright/test';

const HAN_RE = /[㐀-鿿]/;
const base = (process.env.METAPI_UI_SHOT_BASE || '').trim().replace(/\/$/, '');
const authToken = (process.env.METAPI_UI_AUTH_TOKEN || '').trim();
if (!base || !authToken) {
  console.error('METAPI_UI_SHOT_BASE and METAPI_UI_AUTH_TOKEN are required');
  process.exit(2);
}

const routes = [
  { id: 'dashboard', path: '/' },
  { id: 'sites', path: '/sites' },
  { id: 'accounts', path: '/accounts' },
  { id: 'models', path: '/models' },
  { id: 'routes', path: '/routes' },
  { id: 'downstream-keys', path: '/downstream-keys' },
  { id: 'checkin', path: '/checkin' },
  { id: 'logs', path: '/logs' },
  { id: 'monitor', path: '/monitor' },
  { id: 'settings', path: '/settings' },
  { id: 'events', path: '/events' },
];

const failures = [];
const details = [];

const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await context.addInitScript(({ token }) => {
  try {
    localStorage.setItem('app_language', 'en');
    localStorage.setItem('theme_mode', 'light');
    localStorage.setItem('auth_token', token);
    localStorage.setItem('auth_token_expires_at', String(Date.now() + 12 * 60 * 60 * 1000));
  } catch { /* ignore */ }
}, { token: authToken });

const page = await context.newPage();
page.on('pageerror', (err) => {
  details.push(`  [pageerror] ${String(err).slice(0, 200)}`);
});

for (const route of routes) {
  try {
    await page.goto(base + route.path, { waitUntil: 'networkidle', timeout: 30_000 }).catch(async () => {
      await page.goto(base + route.path, { waitUntil: 'domcontentloaded', timeout: 30_000 });
    });
    // Let the MutationObserver settle.
    await page.waitForTimeout(1500);
    const body = page.locator('body');
    await body.waitFor({ state: 'visible', timeout: 10_000 }).catch(() => {});
    const text = await body.innerText().catch(() => '');
    const untranslated = (text.match(/Untranslated/g) || []).length;
    const hanRuns = text.match(HAN_RE);
    const hanSnippet = hanRuns ? text.slice(Math.max(0, text.search(HAN_RE) - 40), text.search(HAN_RE) + 40).replace(/\s+/g, ' ') : '';
    // Context around each Untranslated occurrence for triage.
    let untranslatedSnippets = '';
    if (untranslated > 0) {
      const snips = [];
      let idx = 0;
      while ((idx = text.indexOf('Untranslated', idx)) !== -1) {
        snips.push(text.slice(Math.max(0, idx - 50), idx + 40).replace(/\s+/g, ' '));
        idx += 12;
      }
      untranslatedSnippets = ' | ' + [...new Set(snips)].join(' || ');
    }
    const status = untranslated === 0 && !hanRuns ? 'PASS' : 'FAIL';
    if (status === 'FAIL') {
      failures.push(route.id);
    }
    console.log(`${status}  ${route.id.padEnd(18)} untranslated=${untranslated}${hanRuns ? ` han=${hanRuns.length}` : ''}${untranslatedSnippets}`);
  } catch (err) {
    failures.push(route.id);
    console.log(`ERROR ${route.id.padEnd(18)} ${String(err).slice(0, 160)}`);
  }
}

await browser.close();
if (details.length) {
  console.log('\npage errors:');
  for (const d of details) console.log(d);
}
console.log(`\n${routes.length - failures.length}/${routes.length} routes clean`);
if (failures.length) {
  console.log('FAILED:', failures.join(', '));
  process.exit(1);
}
