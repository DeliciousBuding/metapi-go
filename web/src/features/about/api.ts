// metapi-go/features/about — TanStack Query hook for build/runtime info.
//
// The build provenance (`version` / `commit` / `buildTime` / `goVersion`) comes
// from the Go binary via `GET /api/about`, so the About page shows the version
// of the process actually serving the request rather than the frontend bundle
// version. The curated repository metadata lives in src/lib/about-info.ts (S5 boundary inversion) — it
// describes the project, not the build, and has no backend owner.
//
// Fields the backend leaves empty (a local `go build` injects no commit or
// build time) stay undefined here so the page renders an em-dash instead of a
// fabricated value.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { ABOUT_INFO, type AboutInfo } from '@/lib/about-info'
import { api } from '@/lib/api'

import { aboutKeys } from './types'

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
