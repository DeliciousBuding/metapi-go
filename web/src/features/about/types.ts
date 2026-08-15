// metapi-go/features/about — domain types for the About feature.
//
// The About page renders build/runtime metadata. There is no backend
// `/api/about` endpoint today, so `useAboutInfo` resolves build-time
// constants; the optional `buildTime` / `commit` / `goVersion` fields are
// reserved for when the endpoint lands — the page shows an em-dash for any
// field the backend does not yet supply. The public GitHub repository link
// is the only external reference (metapi-go is a public repo).

export type AboutInfo = {
  /** Semver version, sourced from package.json at build time. */
  version: string
  /** ISO build timestamp. Undefined until a backend endpoint supplies it. */
  buildTime?: string
  /** Git commit SHA. Undefined until a backend endpoint supplies it. */
  commit?: string
  /** Go runtime version string. Undefined until a backend endpoint supplies it. */
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
 * Curated list of key runtime/framework dependencies shown on the About
 * page. Versions are kept in sync with `web/package.json` by hand; the list
 * is intentionally short — the page shows categories, not the full lock file.
 */
export type AboutDependency = {
  name: string
  version: string
  category: 'framework' | 'build' | 'data' | 'ui' | 'form' | 'style'
}

export const KEY_DEPENDENCIES: AboutDependency[] = [
  { name: 'React', version: '19.2', category: 'framework' },
  { name: 'TanStack Router', version: '1.170', category: 'framework' },
  { name: 'TanStack Query', version: '5.101', category: 'data' },
  { name: 'TanStack Table', version: '8.21', category: 'data' },
  { name: 'Rsbuild', version: '2.1', category: 'build' },
  { name: 'Tailwind CSS', version: '4.3', category: 'style' },
  { name: 'shadcn/ui (Base UI)', version: 'base-nova', category: 'ui' },
  { name: 'React Hook Form', version: '7.80', category: 'form' },
  { name: 'Zod', version: '4.4', category: 'form' },
  { name: 'i18next', version: '26.3', category: 'framework' },
  { name: 'Recharts', version: '3.10', category: 'ui' },
  { name: 'tw-animate-css', version: '1.4', category: 'ui' },
]

/**
 * TanStack Query key factory. Centralised so invalidation is grep-able and
 * the keys stay stable across hooks.
 */
export const aboutKeys = {
  all: ['about'] as const,
  info: () => [...aboutKeys.all, 'info'] as const,
}
