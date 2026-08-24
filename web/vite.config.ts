import { readFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'

import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// Mirrors the `METAPI_WEB_VERSION` compile-time constant that
// rsbuild.config.ts injects via `source.define`, so modules consuming the
// global (features/about) also resolve it under vitest.
const webPackageJson = JSON.parse(
  readFileSync(new URL('./package.json', import.meta.url), 'utf-8')
) as { version: string }

export default defineConfig({
  plugins: [react()],
  define: {
    METAPI_WEB_VERSION: JSON.stringify(webPackageJson.version),
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
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
