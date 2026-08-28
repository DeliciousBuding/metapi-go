/// <reference types="@rsbuild/core/types" />

/**
 * SPA version injected at compile time from the web/package.json `version`
 * field. The define value lives in config/build-shared.ts (single source of
 * truth for rsbuild.config.ts and vitest.config.ts). Available in dev and
 * production builds.
 */
declare const METAPI_WEB_VERSION: string
