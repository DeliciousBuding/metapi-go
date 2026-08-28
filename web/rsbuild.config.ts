import { defineConfig, loadEnv } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'
import { tanstackRouter } from '@tanstack/router-plugin/rspack'

// devProxy, version define and '@' alias live in one shared module so the
// dev/build and vitest paths cannot drift (issue #1035 S1).
import {
  createDevProxy,
  srcAlias,
  versionDefines,
} from './config/build-shared.ts'

export default defineConfig(({ envMode }) => {
  // VITE_ prefix kept for parity with legacy env var names
  // (DEV_PROXY_TARGET / VITE_DEV_PROXY_TARGET / PORT / VITE_BACKEND_PORT).
  const env = loadEnv({ mode: envMode, prefixes: ['VITE_'] })
  const isProd = envMode === 'production'

  return {
    // optimize stays false: rsbuild's default minifier already runs
    // lightningcss, and enabling pluginTailwindcss `optimize` re-runs it on
    // the Tailwind output 鈥?measured +3.1 kB raw / +0.8 kB gzip on the CSS
    // bundle (179,325 鈫?182,456 B raw), so it buys nothing.
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
        // Issue #1035 S9: the audit's "vendor-i18n" blob was the unnamed
        // chunk 6432 — a grab-bag of i18next, icon libraries and small
        // utilities. Split the coherent families into named chunks so the
        // i18n runtime is cache-stable on its own.
        'vendor-i18n': {
          test: /node_modules[\\/](i18next|react-i18next|i18next-browser-languagedetector|html-parse-stringify)[\\/]/,
          name: 'vendor-i18n',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-icons': {
          test: /node_modules[\\/](@hugeicons|lucide-react)[\\/]/,
          name: 'vendor-icons',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        // Remainder of the old chunk 6432 (measured via source-map probe):
        // small always-on utilities. Naming it keeps the synchronous graph
        // free of unnamed grab-bag chunks; future deps that do not match
        // fall back to the default split and stay visible in size reports.
        // @floating-ui intentionally excluded: matching it pulls modules
        // from async route chunks into the sync graph (+7 kB gz initial).
        'vendor-core': {
          test: /node_modules[\\/](zod|axios|tailwind-merge|sonner|zustand|clsx|class-variance-authority|use-sync-external-store)[\\/]/,
          name: 'vendor-core',
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
      define: { ...versionDefines },
    },
    resolve: {
      alias: { ...srcAlias },
    },
    html: {
      template: './index.html',
    },
    server: {
      host: '0.0.0.0',
      strictPort: false,
      // metapi-go backend serves admin REST under /api and OpenAI-compatible
      // proxy routes under /v1; createDevProxy resolves the frozen
      // VITE_DEV_PROXY_TARGET / VITE_BACKEND_PORT overrides.
      proxy: createDevProxy(env.rawPublicVars),
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
      // data: URIs 鈥?the production CSP img-src does not allow data:, so
      // inlined brand icons would be blocked just like the old CDN ones.
      dataUriLimit: 0,
      // Rely on Rsbuild default legalComments ("linked" 鈫?per-chunk *.LICENSE.txt) in all modes.
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
            // eager for fast HMR. Route files declare components directly 鈥?no
            // manual `lazyRouteComponent`. Loading state is handled by
            // `defaultPendingComponent` in main.tsx.
            autoCodeSplitting: isProd,
          }),
        ],
      },
    },
  }
})
