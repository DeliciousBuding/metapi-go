/// <reference types="@rsbuild/core/types" />

/**
 * SPA version injected at compile time via `source.define` in
 * rsbuild.config.ts (mirrored in vite.config.ts for vitest), read from the
 * web/package.json `version` field. Available in dev and production builds.
 */
declare const METAPI_WEB_VERSION: string
