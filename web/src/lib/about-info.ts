// metapi-go/lib — curated project metadata (S5 boundary inversion).
//
// Shared by the layout shell's user menu (components/layout) and the About
// feature (features/about). Project metadata is a bottom-layer shared
// constant, not feature behavior — hosting it here lets both layers import
// downward (docs/internal/web-package-boundaries.md, rule 5 precedent).
// Build provenance (binary version / commit / build time) has no owner
// here: features/about merges it over this curated base from GET /api/about.
//
// Fields the backend leaves empty (a local `go build` injects no commit or
// build time) stay undefined so the About page renders an em dash instead
// of a fabricated value.

export type AboutInfo = {
  /** Semver version: the binary version from /api/about, else the bundle version. */
  version: string
  /** UTC RFC3339 build timestamp. Undefined when the binary carries none. */
  buildTime?: string
  /** Git commit SHA. Undefined when the binary carries none. */
  commit?: string
  /** Go runtime version string reported by the backend. */
  goVersion?: string
  /** Human-readable product name. */
  projectName: string
  /** One-line description (from package.json). */
  description: string
  /** Public GitHub repository homepage URL. */
  homepage: string
  /** Public GitHub repository URL. */
  repository: string
  /** SPDX license identifier. */
  license: string
  /** Author handle. */
  author: string
}

/**
 * Project metadata that is not build provenance: name, description, links,
 * license and author. `version` is the `METAPI_WEB_VERSION` global injected by
 * `source.define` in rsbuild.config.ts (read from web/package.json) and acts
 * as the fallback when the backend does not report a binary version.
 */
export const ABOUT_INFO: AboutInfo = {
  version: METAPI_WEB_VERSION,
  projectName: 'Metapi',
  description:
    'Meta-layer management and unified proxy for AI API aggregation platforms',
  homepage: 'https://github.com/DeliciousBuding/metapi-go#readme',
  repository: 'https://github.com/DeliciousBuding/metapi-go',
  license: 'MIT',
  author: 'DeliciousBuding',
}
