/**
 * EN-mode main-surface verification (2026-08-01 e2e wave).
 *
 * Connects to a live single-process backend (embedded SPA, same origin —
 * same pattern as capture-ui-shots.mjs SHOT-1) and walks the admin routes
 * in EN mode, asserting:
 *   - no `Untranslated` fallback text anywhere in the body
 *   - no Han residue in UI containers (page body; user-data cells are
 *     data-i18n-skip exempt by design and not asserted here)
 *   - no CJK punctuation residue (`：` `，` etc. — translateText normalizes
 *     it, so any occurrence is a real regression; quality-audit wave 2026-08-01)
 *
 * `--with-data` additionally seeds a fake site + account via the admin API
 * before walking, so table rows, row actions and dialogs are exercised
 * (empty-DB surfaces only cover EmptyStates). Every route then also probes
 * the first Add/New/Create button and asserts the opened dialog is clean.
 *
 * Usage (from web/):
 *   METAPI_UI_AUTH_TOKEN=<bearer> METAPI_UI_SHOT_BASE=http://127.0.0.1:4000 \
 *     node scripts/verify-en-pages.mjs [--with-data]
 *
 * Exit code 0 = all routes clean; 1 = at least one route had residue.
 */
import { chromium } from '@playwright/test';

const HAN_RE = /[㐀-鿿]/;
// CJK punctuation must never leak into EN output (quality-audit wave:
// translateText normalizes it, so any residue here is a real regression).
const CJK_PUNCT_RE = /[：，。；！？（）【】“”‘’、…]/;
const base = (process.env.METAPI_UI_SHOT_BASE || '').trim().replace(/\/$/, '');
const authToken = (process.env.METAPI_UI_AUTH_TOKEN || '').trim();
const withData = process.argv.includes('--with-data');
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
  // Deep surfaces added 2026-08-01 (quality-audit coverage):
  { id: 'oauth', path: '/oauth' },
  { id: 'playground', path: '/playground' },
  { id: 'tokens', path: '/tokens' },
  { id: 'site-announcements', path: '/site-announcements' },
  { id: 'about', path: '/about' },
  { id: 'settings-notify', path: '/settings/notify' },
  { id: 'settings-import-export', path: '/settings/import-export' },
];

const failures = [];
const details = [];

/** Seed a fake site + account via the admin API (--with-data mode). */
async function seedData() {
  const headers = { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` };
  const siteRes = await fetch(`${base}/api/sites`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ name: 'en-verify-demo', url: 'https://example.com', platform: 'new-api', status: 'disabled' }),
  });
  const site = await siteRes.json().catch(() => ({}));
  const siteId = site.site?.id ?? site.id;
  if (siteId) {
    await fetch(`${base}/api/accounts`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ siteId, username: 'demo-user', apiToken: 'sk-demo-token', skipModelFetch: true }),
    }).catch(() => {});
  }
  console.log(`seeded site id=${siteId ?? 'n/a'} (${siteRes.status})`);
}

/** Probe the first Add/New/Create button and assert its dialog is clean. */
async function checkDialogs(page) {
  const btn = page.locator('button:has-text("Add"), button:has-text("New"), button:has-text("Create")').first();
  if (!(await btn.isVisible().catch(() => false))) return null;
  await btn.click();
  await page.waitForTimeout(800);
  const text = await page.locator('body').innerText().catch(() => '');
  const hasUntranslated = text.includes('Untranslated');
  await page.keyboard.press('Escape');
  await page.waitForTimeout(400);
  return hasUntranslated ? 'dialog contains Untranslated' : null;
}

if (withData) {
  await seedData();
}

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
    const cjkPunctRuns = text.match(CJK_PUNCT_RE);
    const cjkPunctSnippet = cjkPunctRuns ? text.slice(Math.max(0, text.search(CJK_PUNCT_RE) - 40), text.search(CJK_PUNCT_RE) + 40).replace(/\s+/g, ' ') : '';
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
    let status = untranslated === 0 && !hanRuns && !cjkPunctRuns ? 'PASS' : 'FAIL';
    let dialogNote = '';
    if (status === 'PASS') {
      const dialogIssue = await checkDialogs(page).catch(() => null);
      if (dialogIssue) {
        status = 'FAIL';
        dialogNote = ` | dialog: ${dialogIssue}`;
      }
    }
    if (status === 'FAIL') {
      failures.push(route.id);
    }
    console.log(`${status}  ${route.id.padEnd(18)} untranslated=${untranslated}${hanRuns ? ` han=${hanRuns.length} [${hanSnippet}]` : ''}${cjkPunctRuns ? ` cjkPunct=${cjkPunctRuns.length} [${cjkPunctSnippet}]` : ''}${untranslatedSnippets}${dialogNote}`);
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
