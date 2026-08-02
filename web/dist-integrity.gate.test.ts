import { describe, expect, it } from 'vitest';
import { execSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';

// dist-integrity.gate.test.ts — built-frontend self-consistency gate.
//
// The v0.8.51 deploy surfaced a class of failure no unit test covered: the
// built dist referencing assets that do not exist (index.html entries and
// lazy chunks). This gate runs the same verifier the Dockerfile runs after
// `npm run build:web`; any referenced-but-missing asset fails CI.
//
// Skips silently when dist is absent (environments that never built the
// frontend); the Dockerfile side fails hard instead.

const webRoot = join(fileURLToPath(import.meta.url), '..');
const distDir = join(webRoot, 'dist');

describe('dist asset integrity', () => {
  it('every asset referenced by index.html and lazy imports exists in dist', () => {
    if (!existsSync(distDir)) return; // dist not built in this environment

    const out = execSync(`node scripts/verify-dist.mjs`, {
      cwd: webRoot,
      encoding: 'utf8',
    });
    expect(out).toContain('verify-dist OK');
  });
});
