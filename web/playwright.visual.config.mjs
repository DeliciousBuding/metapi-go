// metapi-go — Playwright config for the golden screenshot regression suite
// (web/scripts/visual-regression.spec.mjs). Deliberately separate from the
// unit-test tooling: vitest guards logic, this suite guards the shipped SPA's
// pixels against committed baselines in web/visual-baselines/.
//
// Usage:
//   bun run visual:regression                       # compare against baselines
//   UPDATE_SNAPSHOTS=all bun run visual:regression  # rebaseline (intentional UI change)
import { defineConfig } from 'playwright/test'

export default defineConfig({
  testDir: 'scripts',
  // Only .mjs playwright specs under scripts/; vitest owns src/**/*.{test,spec}.{ts,tsx}.
  testMatch: '**/*.spec.mjs',
  // Golden baselines are committed under web/visual-baselines/<page>.png.
  // toHaveScreenshot('foo.png') resolves to {arg}='foo' + {ext}='.png'.
  snapshotPathTemplate: '{testDir}/../visual-baselines/{arg}{ext}',
  // Baselines are committed; CI must never silently create missing ones.
  // Rebaselining is only allowed explicitly via UPDATE_SNAPSHOTS=all (local).
  updateSnapshots: process.env.UPDATE_SNAPSHOTS === 'all' ? 'all' : 'none',
  // Default test-results dir (web/test-results) is already gitignored.
  timeout: 90_000,
  workers: 1,
  fullyParallel: false,
  reporter: [['list']],
  expect: {
    toHaveScreenshot: {
      // Tolerate anti-aliasing jitter only (1% of a 1440x2000 page ≈ 28.8k px);
      // real layout drift exceeds this by orders of magnitude.
      maxDiffPixelRatio: 0.01,
      animations: 'disabled',
      caret: 'hide',
      timeout: 30_000,
    },
  },
  use: {
    browserName: 'chromium',
    headless: true,
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
    locale: 'zh-CN',
    colorScheme: 'light',
    launchOptions: {
      // Keep the sweep deterministic on machines that carry a system proxy.
      args: ['--no-proxy-server', '--disable-dev-shm-usage'],
    },
  },
})
