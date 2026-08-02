// verify-dist.mjs — dist asset integrity check (CLI + importable).
//
// Catches exactly the class of failure observed at v0.8.51 deploy:
// index.html (or a lazy-importing bundle) referencing an asset that does not
// exist in the built dist — the browser then 404s on route navigation and
// renders the error page.
//
// Usage:
//   node scripts/verify-dist.mjs            # checks web/dist relative to this file
//   node scripts/verify-dist.mjs <distDir>  # checks an explicit dist directory
//
// Runs inside the Dockerfile right after `npm run build:web`, so a
// self-inconsistent build never becomes an image.

import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, resolve } from 'node:path';

/**
 * @param {string} distRoot absolute path to a built dist directory
 * @returns {{ missing: string[], referenced: number }}
 */
export function collectMissing(distRoot) {
  const assetsDir = join(distRoot, 'assets');
  const missing = [];
  if (!existsSync(assetsDir)) {
    return { missing: ['assets/ (dist not built?)'], referenced: 0 };
  }

  const referenced = new Set();

  // 1) Entry assets referenced by index.html (script src / link href).
  const htmlPath = join(distRoot, 'index.html');
  if (existsSync(htmlPath)) {
    const html = readFileSync(htmlPath, 'utf8');
    for (const m of html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g)) {
      referenced.add(m[1].replace(/^\//, ''));
    }
  }

  // 2) Lazy chunks referenced by any bundle via dynamic import (rolldown
  //    emits backtick form; keep the double-quote form for older rollup).
  for (const f of readdirSync(assetsDir).filter((f) => f.endsWith('.js'))) {
    const src = readFileSync(join(assetsDir, f), 'utf8');
    for (const m of src.matchAll(/import\(`\.\/([^`]+)`\)/g)) referenced.add(`assets/${m[1]}`);
    for (const m of src.matchAll(/import\("\.\/([^"]+)"\)/g)) referenced.add(`assets/${m[1]}`);
  }

  for (const rel of referenced) {
    if (!existsSync(join(distRoot, rel))) missing.push(rel);
  }
  return { missing, referenced: referenced.size };
}

// CLI entry — only when executed directly (not when imported by tests).
if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
  const distRoot = process.argv[2]
    ? resolve(process.argv[2])
    : resolve(fileURLToPath(import.meta.url), '..', '..', 'dist');
  const { missing, referenced } = collectMissing(distRoot);
  if (missing.length > 0) {
    console.error(
      `verify-dist FAIL: ${missing.length}/${referenced} referenced assets missing:\n` +
        missing.join('\n'),
    );
    process.exit(1);
  }
  console.log(`verify-dist OK: ${referenced} referenced assets all present in ${distRoot}`);
}
