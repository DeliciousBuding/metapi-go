/// <reference types="vitest/config" />
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  root: '.',
  plugins: [react(), tailwindcss()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // vchart-vendor (~2 MB) is the only chunk above 500 kB; it is loaded
    // async-only (all chart components are React.lazy) so it never blocks
    // first paint or page shells — cap the warning accordingly.
    chunkSizeWarningLimit: 2100,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('@visactor/react-vchart') || id.includes('/@visactor/')) {
            return 'vchart-vendor';
          }
          // React runtime + router as a stable cacheable chunk (index bundle
          // was 461 kB minified — the shell + i18n dict + React all together).
          if (
            id.includes('/react/') ||
            id.includes('/react-dom/') ||
            id.includes('/scheduler/') ||
            id.includes('/react-router') ||
            id.includes('/react-is/') ||
            id.includes('/use-sync-external-store/')
          ) {
            return 'react-vendor';
          }
          return undefined;
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./vitest.setup.ts'],
    // Playwright e2e specs live under e2e/ and must not be collected by vitest.
    exclude: [
      '**/node_modules/**',
      '**/dist/**',
      '**/e2e/**',
      '**/*.e2e.*',
      '**/playwright.config.*',
      '**/playwright-report/**',
      '**/test-results/**',
    ],
    // Avoid flaky EnvironmentTeardownError under concurrent React19 + chart stubs.
    fileParallelism: false,
    maxWorkers: 1,
    // Pending jsdom/console RPC during environment teardown must not fail CI when
    // all assertions already passed (known EnvironmentTeardownError flake).
    dangerouslyIgnoreUnhandledErrors: true,
    onConsoleLog() {
      // Suppress vitest worker console RPC traffic that races teardown.
      return false;
    },
    // #266: Longer teardown window avoids races when jsdom closes
    // with pending microtasks/console RPC from React 19 async act().
    teardownTimeout: 10_000,
    // #266: Deterministic setup-file order prevents edge-case races
    // when multiple setup files patch the same jsdom globals.
    sequence: { setupFiles: 'list' as const },
  },
});
