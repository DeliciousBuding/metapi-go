import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// METAPI_WEB_VERSION define and '@' alias come from the shared build module
// (single source of truth with rsbuild.config.ts) so tests resolve exactly
// the same compile-time constants as dev/build (issue #1035 S1).
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
    fileParallelism: false,
    maxWorkers: 1,
    teardownTimeout: 10_000,
    sequence: { setupFiles: 'list' as const },
  },
})
