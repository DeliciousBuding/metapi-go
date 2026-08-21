// metapi-go/features/about — TanStack Query hook for build/runtime info.
//
// The build provenance (`version` / `commit` / `buildTime` / `goVersion`) comes
// from the Go binary via `GET /api/about`, so the About page shows the version
// of the process actually serving the request rather than the frontend bundle
// version. The repository metadata below is curated in the frontend — it
// describes the project, not the build, and has no backend owner.
//
// Fields the backend leaves empty (a local `go build` injects no commit or
// build time) stay undefined here so the page renders an em-dash instead of a
// fabricated value.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import { aboutKeys, type AboutInfo } from './types'

/**
 * Project metadata that is not build provenance: name, description, links,
 * license and author. `version` is the `METAPI_WEB_VERSION` global injected by
 * `source.define` in rsbuild.config.ts (read from web/package.json) and acts
 * as the fallback when the backend does not report a binary version.
 */
const ABOUT_INFO: AboutInfo = {
  version: METAPI_WEB_VERSION,
  projectName: 'Metapi',
  description:
    'Meta-layer management and unified proxy for AI API aggregation platforms',
  homepage: 'https://github.com/DeliciousBuding/metapi-go#readme',
  repository: 'https://github.com/DeliciousBuding/metapi-go',
  license: 'MIT',
  author: 'DeliciousBuding',
}

export { ABOUT_INFO }

/**
 * Narrow an untrusted backend field to a non-empty trimmed string, or
 * undefined. Absent / blank / non-string values all collapse to undefined so
 * the page's em-dash fallback takes over instead of rendering `"undefined"`.
 */
function optionalText(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : undefined
}

/**
 * Merge the backend build provenance into the curated project metadata. The
 * binary version wins over the bundle version because it describes the process
 * answering the request; the bundle version remains the fallback for a backend
 * that reports no version at all.
 */
async function fetchAboutInfo(): Promise<AboutInfo> {
  const response = await api.getAbout()
  return {
    ...ABOUT_INFO,
    version: optionalText(response?.version) ?? ABOUT_INFO.version,
    commit: optionalText(response?.commit),
    buildTime: optionalText(response?.buildTime),
    goVersion: optionalText(response?.goVersion),
  }
}

/**
 * Fetch about/build info from `GET /api/about`. `staleTime: Infinity` because
 * the build provenance of a running binary cannot change without a restart.
 */
export function useAboutInfo(
  options?: Omit<UseQueryOptions<AboutInfo>, 'queryKey' | 'queryFn'>
) {
  return useQuery<AboutInfo>({
    queryKey: aboutKeys.info(),
    queryFn: fetchAboutInfo,
    staleTime: Infinity,
    ...options,
  })
}
