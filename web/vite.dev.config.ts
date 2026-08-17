// Throwaway Vite dev config for the sign-in redesign session.
// The canonical dev/build tool is Rsbuild (rsbuild.config.ts); this file only
// exists so we can run a native Vite dev server with HMR while iterating on
// the login page visuals. NOT used for production builds.

import path from 'node:path'
import { fileURLToPath } from 'node:url'

import react from '@vitejs/plugin-react'
import { TanStackRouterVite } from '@tanstack/router-vite-plugin'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig, loadEnv } from 'vite'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

// VITE_ prefix kept for parity with rsbuild.config.ts env handling
// (DEV_PROXY_TARGET / VITE_DEV_PROXY_TARGET / PORT / VITE_BACKEND_PORT).
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, __dirname, 'VITE_')
  const backendPort =
    process.env.VITE_BACKEND_PORT || env.VITE_BACKEND_PORT
  const serverUrl =
    process.env.VITE_DEV_PROXY_TARGET ||
    env.VITE_DEV_PROXY_TARGET ||
    `http://localhost:${backendPort || '4000'}`

  // metapi-go backend serves admin REST under /api and OpenAI-compatible
  // proxy routes under /v1. Proxying is optional for visual work (the sign-in
  // page renders without a backend), but kept so a live login submit works.
  const devProxy = Object.fromEntries(
    (['/api', '/v1'] as const).map((key) => [
      key,
      { target: serverUrl, changeOrigin: true },
    ])
  )

  return {
    plugins: [
      TanStackRouterVite({ target: 'react', autoCodeSplitting: false }),
      tailwindcss(),
      react(),
    ],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      host: '0.0.0.0',
      port: 5173,
      strictPort: false,
      proxy: devProxy,
    },
  }
})
