import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig, loadEnv } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'
import { tanstackRouter } from '@tanstack/router-plugin/rspack'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

const webPackageJson = JSON.parse(
  readFileSync(path.join(__dirname, 'package.json'), 'utf-8')
) as { version: string }

export default defineConfig(({ envMode }) => {
  // VITE_ prefix kept for parity with legacy env var names
  // (DEV_PROXY_TARGET / VITE_DEV_PROXY_TARGET / PORT / VITE_BACKEND_PORT).
  const env = loadEnv({ mode: envMode, prefixes: ['VITE_'] })
  const backendPort =
    process.env.VITE_BACKEND_PORT || env.rawPublicVars.VITE_BACKEND_PORT
  const serverUrl =
    process.env.VITE_DEV_PROXY_TARGET ||
    env.rawPublicVars.VITE_DEV_PROXY_TARGET ||
    `http://localhost:${backendPort || '4000'}`

  const isProd = envMode === 'production'
  // metapi-go backend serves admin REST under /api and OpenAI-compatible proxy
  // routes under /v1 (handler/admin ~144 endpoints + handler/proxy ~30 routes).
  const devProxy = Object.fromEntries(
    (['/api', '/v1'] as const).map((key) => [
      key,
      { target: serverUrl, changeOrigin: true },
    ])
  ) as Record<string, { target: string; changeOrigin: boolean }>

  return {
    // optimize stays false: rsbuild's default minifier already runs
    // lightningcss, and enabling pluginTailwindcss `optimize` re-runs it on
    // the Tailwind output — measured +3.1 kB raw / +0.8 kB gzip on the CSS
    // bundle (179,325 → 182,456 B raw), so it buys nothing.
    plugins: [pluginReact(), pluginTailwindcss({ optimize: false })],
    // Rsbuild 2: replaces deprecated `performance.chunkSplit` (RSPack 2 aligned)
    splitChunks: {
      preset: 'default',
      cacheGroups: {
        'vendor-react': {
          test: /node_modules[\\/](react|react-dom)[\\/]/,
          name: 'vendor-react',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-ui-primitives': {
          test: /node_modules[\\/](@base-ui|@radix-ui)[\\/]/,
          name: 'vendor-ui-primitives',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-tanstack': {
          test: /node_modules[\\/]@tanstack[\\/]/,
          name: 'vendor-tanstack',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-recharts': {
          test: /node_modules[\\/](recharts|d3-.*|victory-vendor)[\\/]/,
          name: 'vendor-recharts',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
      },
    },
    source: {
      entry: {
        index: './src/main.tsx',
      },
      // Compile-time constant consumed by features/about; keeping it in sync
      // with the package.json `version` field prevents the displayed SPA
      // version from drifting at release time. Applied to dev and build.
      define: {
        METAPI_WEB_VERSION: JSON.stringify(webPackageJson.version),
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    html: {
      template: './index.html',
    },
    server: {
      host: '0.0.0.0',
      strictPort: false,
      proxy: devProxy,
    },
    output: {
      // Production optimizations
      minify: isProd,
      target: 'web',
      distPath: {
        // web/embed.go expects assets under web/dist/
        root: 'dist',
      },
      // Force every asset to a hashed file instead of inlining small ones as
      // data: URIs — the production CSP img-src does not allow data:, so
      // inlined brand icons would be blocked just like the old CDN ones.
      dataUriLimit: 0,
      // Rely on Rsbuild default legalComments ("linked" → per-chunk *.LICENSE.txt) in all modes.
      // Do not set "none" in production: that strips minifier-preserved third-party notices and
      // extracted license files, which some distributions require for open-source compliance.
    },
    performance: {
      // Remove console.log in production (keep warn/error for diagnostics)
      removeConsole: isProd ? ['log'] : false,
      // Persistent module cache (node_modules/.cache/rspack). Speeds up
      // incremental rebuilds; output bytes stay identical between a cold and
      // a warm build (verified by hashing dist/ twice).
      buildCache: true,
    },
    tools: {
      rspack: {
        plugins: [
          tanstackRouter({
            target: 'react',
            // Single source of truth for route code-splitting. Prod splits each
            // route's component (plus error/notFound) into async chunks; loaders
            // stay eager (the plugin's default groupings) to avoid a
            // loader-chunk -> data -> component-chunk waterfall. Dev keeps routes
            // eager for fast HMR. Route files declare components directly — no
            // manual `lazyRouteComponent`. Loading state is handled by
            // `defaultPendingComponent` in main.tsx.
            autoCodeSplitting: isProd,
          }),
        ],
      },
    },
  }
})
