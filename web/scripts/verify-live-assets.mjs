// verify-live-assets.mjs — post-deploy smoke: every asset the running instance
// references must answer 200 with a non-HTML content type.
//
// Regression net for the v0.8.50/0.8.51 deploy incidents: /logo.png answered
// 200 text/html (SPA fallback swallowed it) and lazy chunks 404'd after
// upgrades because a stale index.html referenced old hashes. This script
// replays the exact browser asset graph against a live server.
//
// Usage:
//   node scripts/verify-live-assets.mjs [baseUrl]   (default http://127.0.0.1:4000)

const base = (process.argv[2] || 'http://127.0.0.1:4000').replace(/\/+$/, '');

const referenced = new Set();
const missing = [];
const bad = [];

async function collectFrom(rel, isHtml) {
  const res = await fetch(base + rel);
  if (!res.ok) {
    missing.push(`${res.status} ${rel}`);
    return;
  }
  const ct = res.headers.get('content-type') || '';
  if (isHtml && !ct.includes('text/html')) bad.push(`index.html content-type=${ct}`);
  if (!isHtml && ct.includes('text/html')) {
    // SPA fallback swallowed a static asset (the /logo.png incident).
    bad.push(`${rel} answered text/html instead of a real asset`);
  }
  if (ct.includes('javascript')) {
    const src = await res.text();
    for (const m of src.matchAll(/import\(`\.\/([^`]+)`\)/g)) {
      referenced.add(`/assets/${m[1]}`);
    }
    for (const m of src.matchAll(/import\("\.\/([^"]+)"\)/g)) {
      referenced.add(`/assets/${m[1]}`);
    }
  }
}

// 1) index.html → entry assets; 2) entry bundles → lazy chunks.
const htmlRes = await fetch(base + '/');
if (!htmlRes.ok) {
  console.error(`verify-live-assets FAIL: GET / -> ${htmlRes.status}`);
  process.exit(1);
}
const html = await htmlRes.text();
for (const m of html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g)) referenced.add(m[1]);

for (const rel of [...referenced].filter((r) => r.endsWith('.js'))) await collectFrom(rel, false);

for (const rel of referenced) {
  if (rel.endsWith('.js')) continue; // already fetched above
  const res = await fetch(base + rel);
  if (!res.ok) missing.push(`${res.status} ${rel}`);
  else if ((res.headers.get('content-type') || '').includes('text/html')) {
    bad.push(`${rel} answered text/html instead of a real asset`);
  }
}

if (missing.length > 0 || bad.length > 0) {
  console.error('verify-live-assets FAIL');
  for (const m of missing) console.error(`  missing: ${m}`);
  for (const b of bad) console.error(`  bad: ${b}`);
  process.exit(1);
}
console.log(`verify-live-assets OK: ${referenced.size} referenced assets all 200 (${base})`);
