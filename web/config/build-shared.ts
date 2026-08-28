// Single source of truth for build-time values shared by every config path:
//   - rsbuild.config.ts  → dev server + production build
//   - vitest.config.ts   → unit tests
//
// Keeping devProxy, the METAPI_WEB_VERSION define and the '@' alias here
// guarantees the three consumers cannot drift apart (issue #1035 S1).

import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

/** Absolute path of the web/ package root (this file lives in web/config/). */
const webRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)))

/**
 * SPA version injected at compile time through `source.define` /
 * vitest `define`; read from the web/package.json `version` field so the
 * displayed SPA version can never drift from the release version.
 */
const webVersion = (
  JSON.parse(readFileSync(path.join(webRoot, 'package.json'), 'utf-8')) as {
    version: string
  }
).version

/** Compile-time constants (`declare const METAPI_WEB_VERSION: string`). */
export const versionDefines = {
  METAPI_WEB_VERSION: JSON.stringify(webVersion),
} as const

/** '@' → web/src alias, identical for bundler and test resolution. */
export const srcAlias = { '@': path.resolve(webRoot, 'src') } as const

export interface DevProxyEntry {
  target: string
  changeOrigin: boolean
}

/**
 * Dev proxy table for `/api` (admin REST) and `/v1` (OpenAI-compatible proxy
 * routes). Env var names are frozen for TS-version parity:
 * DEV_PROXY_TARGET / VITE_DEV_PROXY_TARGET / PORT / VITE_BACKEND_PORT.
 *
 * @param publicEnv values from `.env*` files (rsbuild `loadEnv` public vars);
 * process.env always wins so shell-level overrides behave the same in dev.
 */
export function createDevProxy(
  publicEnv: Record<string, string | undefined> = {}
): Record<string, DevProxyEntry> {
  const backendPort =
    process.env.VITE_BACKEND_PORT || publicEnv.VITE_BACKEND_PORT
  const serverUrl =
    process.env.VITE_DEV_PROXY_TARGET ||
    publicEnv.VITE_DEV_PROXY_TARGET ||
    `http://localhost:${backendPort || '4000'}`

  return Object.fromEntries(
    (['/api', '/v1'] as const).map((key) => [
      key,
      { target: serverUrl, changeOrigin: true },
    ])
  ) as Record<string, DevProxyEntry>
}
