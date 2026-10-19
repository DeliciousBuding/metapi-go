import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// METAPI_WEB_VERSION define and '@' alias come from the shared build module
// (single source of truth with rsbuild.config.ts) so tests resolve exactly
// the same compile-time constants as dev/build (issue #1035).
import { srcAlias, versionDefines } from './config/build-shared.ts'

export default defineConfig({
  plugins: [react()],
  define: { ...versionDefines },
  resolve: {
    alias: { ...srcAlias },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./vitest.setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['**/node_modules/**', '**/dist/**'],
    maxWorkers: 4,
    // Every async assertion keeps its own waitFor budget (1s by default), so
    // the per-test ceiling only measures environment setup and CPU
    // contention. Three different files hit the 5s default in CI on
    // 2026-10-19 while passing in isolation; jsdom setup alone is ~35% of a
    // run. A genuinely hung test still fails, ten seconds later.
    testTimeout: 15_000,
    teardownTimeout: 10_000,
    sequence: { setupFiles: 'list' as const },
  },
})
