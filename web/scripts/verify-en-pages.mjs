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
 * `--with-data` additionally seeds a fake site via the admin API and chart
 * fixtures directly into the SQLite database before walking, so table rows,
 * row actions, dialogs, and Dashboard data-state charts are exercised. Every
 * route then also probes the first Add/New/Create button and asserts the
 * opened dialog is clean.
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
const zhMode = process.argv.includes('--zh');
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

/** Seed deterministic chart data via direct SQLite writes (--with-data mode).
 * The account API verifies upstream tokens, which is intentionally unavailable
 * in this hermetic UI job. Direct fixtures keep the test offline while still
 * exercising the real backend queries and rendered Dashboard data states.
 */
async function seedChartData(siteId) {
  const dbPath = (process.env.METAPI_UI_DB_URL || process.env.DB_URL || '').trim();
  if (!dbPath) {
    throw new Error('--with-data requires METAPI_UI_DB_URL or DB_URL for the SQLite fixture database');
  }
  if (dbPath.includes('://')) {
    throw new Error(`--with-data chart fixtures require a SQLite path, got: ${dbPath}`);
  }
  let db;
  try {
    const { DatabaseSync } = await import('node:sqlite');
    db = new DatabaseSync(dbPath, { timeout: 10_000 });
    db.exec('BEGIN');
    const nowMs = Date.now();
    const now = new Date(nowMs).toISOString();
    const fixtureKey = `enverify-${nowMs}-${process.pid}`;

    const accRes = db.prepare(
      `INSERT INTO accounts (site_id, username, access_token, api_token, balance, balance_used, quota, status, created_at, updated_at)
       VALUES (?, ?, 'sk-demo-token', 'sk-demo-token', 100, 25.5, 200, 'healthy', ?, ?)`,
    ).run(siteId, `${fixtureKey}-user`, now, now);
    const accountId = Number(accRes.lastInsertRowid);

    for (let i = 13; i >= 0; i--) {
      const d = new Date(nowMs - i * 86_400_000);
      const day = d.toISOString().slice(0, 10);
      const elapsedDays = 13 - i;
      const bal = 100 - elapsedDays * 2;
      db.prepare(
        `INSERT INTO balance_history (account_id, balance, balance_used, quota, local_day, captured_at, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
      ).run(accountId, bal, 25.5 + elapsedDays * 2, 200, day, d.toISOString(), d.toISOString());
    }

    for (let i = 6; i >= 0; i--) {
      const d = new Date(nowMs - i * 86_400_000);
      const day = d.toISOString().slice(0, 10);
      db.prepare(
        `INSERT INTO site_day_usage (local_day, site_id, total_calls, success_calls, failed_calls, total_tokens,
           total_summary_spend, total_site_spend, total_latency_ms, latency_count, created_at, updated_at)
         VALUES (?, ?, 3, 3, 0, 4020, 0.042, 0.042, 1380, 3, ?, ?)`,
      ).run(day, siteId, d.toISOString(), d.toISOString());

      for (let k = 0; k < 3; k++) {
        db.prepare(
          `INSERT INTO proxy_logs (route_id, channel_id, account_id, model_requested, model_actual, status, http_status,
             is_stream, latency_ms, prompt_tokens, completion_tokens, total_tokens, estimated_cost, request_id, created_at)
           VALUES (NULL, NULL, ?, 'gpt-4o-mini', 'gpt-4o-mini', 'success', 200, 0, ?, ?, ?, ?, ?, ?, ?)`,
        ).run(accountId, 400 + k * 60, 800 + k * 100, 400 + k * 40, 1200 + k * 140,
          0.01 + k * 0.004, `${fixtureKey}-${i}-${k}`, d.toISOString());
      }
    }

    const accountCount = Number(db.prepare('SELECT COUNT(*) AS count FROM accounts WHERE id = ?').get(accountId).count);
    const balanceCount = Number(db.prepare('SELECT COUNT(*) AS count FROM balance_history WHERE account_id = ?').get(accountId).count);
    const siteUsageCount = Number(db.prepare('SELECT COUNT(*) AS count FROM site_day_usage WHERE site_id = ?').get(siteId).count);
    const proxyLogCount = Number(db.prepare('SELECT COUNT(*) AS count FROM proxy_logs WHERE request_id LIKE ?').get(`${fixtureKey}-%`).count);
    if (accountCount !== 1 || balanceCount !== 14 || siteUsageCount !== 7 || proxyLogCount !== 21) {
      throw new Error(
        `fixture count mismatch: account=${accountCount}, balance=${balanceCount}, siteUsage=${siteUsageCount}, proxyLogs=${proxyLogCount}`,
      );
    }

    db.exec('COMMIT');
    console.log(`chart fixture seeded: account=${accountId}, balance=14, siteUsage=7, proxyLogs=21 (${dbPath})`);
  } catch (err) {
    try { db?.exec('ROLLBACK'); } catch { /* already closed */ }
    throw new Error(`chart fixture failed: ${err?.message ?? err}`, { cause: err });
  } finally {
    try { db?.close(); } catch { /* ignore */ }
  }
}

/** Seed a fake site and its offline data fixtures (--with-data mode). */
async function seedData() {
  const headers = { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` };
  const siteFixtureKey = `en-verify-${Date.now()}-${process.pid}`;
  const siteRes = await fetch(`${base}/api/sites`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      name: siteFixtureKey,
      url: `https://${siteFixtureKey}.invalid`,
      platform: 'new-api',
      status: 'disabled',
    }),
  });
  const site = await siteRes.json().catch(() => ({}));
  const siteId = site.site?.id ?? site.id;
  if (!siteRes.ok || !siteId) {
    throw new Error(`site fixture failed (${siteRes.status}): ${JSON.stringify(site).slice(0, 300)}`);
  }
  await seedChartData(siteId);
  console.log(`seeded site id=${siteId ?? 'n/a'} (${siteRes.status})`);
}

/** Assert selected Dashboard charts reached their real data state, not EmptyState. */
async function checkDashboardDataState(page) {
  const titlePrefixes = zhMode
    ? ['余额趋势', '模型成本分布', '延迟直方图', '延迟趋势']
    : ['Balance trend', 'Model cost distribution', 'Latency histogram', 'Latency trend'];
  return page.evaluate((prefixes) => {
    const issues = [];
    const elements = Array.from(document.querySelectorAll('span, button'));
    for (const prefix of prefixes) {
      const title = elements.find((el) => (el.textContent || '').trim().startsWith(prefix));
      if (!title) {
        issues.push(`${prefix}: title missing`);
        continue;
      }
      let card = title.parentElement;
      while (card && !card.querySelector('canvas') && !card.querySelector('.dashboard-chart-empty')) {
        card = card.parentElement;
      }
      if (!card) {
        issues.push(`${prefix}: chart container missing`);
      } else if (card.querySelector('.dashboard-chart-empty')) {
        issues.push(`${prefix}: still rendered EmptyState`);
      } else if (!card.querySelector('canvas')) {
        issues.push(`${prefix}: canvas missing`);
      }
    }
    return issues;
  }, titlePrefixes);
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
await context.addInitScript(({ token, lang }) => {
  try {
    localStorage.setItem('app_language', lang);
    localStorage.setItem('theme_mode', 'light');
    localStorage.setItem('auth_token', token);
    localStorage.setItem('auth_token_expires_at', String(Date.now() + 12 * 60 * 60 * 1000));
  } catch { /* ignore */ }
}, { token: authToken, lang: zhMode ? 'zh' : 'en' });

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
    // Attribute surface: placeholder / title / aria-label values are translated
    // by the observer but invisible to innerText — assert them separately
    // (2026-08-01 attr-surface wave). zh mode: only Untranslated matters
    // (Han/CJK-punct attributes are the normal zh state).
    const attrBad = await page.evaluate(({ zh }) => {
      const bad = [];
      const sel = '[placeholder], [title], [aria-label]';
      for (const el of document.querySelectorAll(sel)) {
        for (const attr of ['placeholder', 'title', 'aria-label']) {
          const v = el.getAttribute(attr);
          if (!v) continue;
          if (v.includes('Untranslated')) bad.push(`${attr}="${v.slice(0, 70)}"`);
          if (!zh && (/[㐀-鿿]/.test(v) || /[：，。（）]/.test(v))) bad.push(`${attr}="${v.slice(0, 70)}"`);
        }
      }
      return bad;
    }, { zh: zhMode }).catch(() => []);
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
    // zh mode: no Untranslated fallback + known zh UI labels must be present
    // (guards against en→zh restore pollution / English residue).
    let zhMissing = null;
    if (zhMode) {
      const known = ['站点', '设置', '仪表盘', '路由', '账号', '通知'];
      const hits = known.filter((k) => text.includes(k));
      if (hits.length === 0) zhMissing = `no known zh labels (of ${known.join('/')})`;
    }
    let status = untranslated === 0 && attrBad.length === 0 && !zhMissing && (zhMode || (!hanRuns && !cjkPunctRuns)) ? 'PASS' : 'FAIL';
    let dialogNote = '';
    if (status === 'PASS') {
      if (withData && route.id === 'dashboard') {
        const dataStateIssues = await checkDashboardDataState(page).catch((err) => [String(err)]);
        if (dataStateIssues.length > 0) {
          status = 'FAIL';
          dialogNote += ` | data-state: ${dataStateIssues.join(' || ')}`;
        }
      }
    }
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
    console.log(`${status}  ${route.id.padEnd(18)} untranslated=${untranslated}${zhMode ? '' : `${hanRuns ? ` han=${hanRuns.length} [${hanSnippet}]` : ''}${cjkPunctRuns ? ` cjkPunct=${cjkPunctRuns.length} [${cjkPunctSnippet}]` : ''}`}${attrBad.length ? ` attr=${attrBad.length} [${attrBad.slice(0, 3).join(' || ')}]` : ''}${zhMissing ? ` zh=${zhMissing}` : ''}${untranslatedSnippets}${dialogNote}`);
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
