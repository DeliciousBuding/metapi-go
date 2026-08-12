// metapi-go/features/about — TanStack Query hook for build/runtime info.
//
// No backend `/api/about` endpoint exists today. The query resolves
// build-time constants (version + repo metadata from package.json) so the
// page can render immediately. The optional fields (`buildTime` / `commit` /
// `goVersion`) are left undefined; when a backend endpoint lands, swap the
// `queryFn` to call `api.getAbout()` (or equivalent) and the page picks them
// up with no component changes. `staleTime: Infinity` because the data is
// constant for the lifetime of the page load.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { aboutKeys, type AboutInfo } from './types'

const ABOUT_INFO: AboutInfo = {
  version: '0.9.0',
  projectName: 'MetAPI',
  description:
    'Meta-layer management and unified proxy for AI API aggregation platforms',
  homepage: 'https://github.com/DeliciousBuding/metapi-go#readme',
  repository: 'https://github.com/DeliciousBuding/metapi-go',
  license: 'MIT',
  author: 'DeliciousBuding',
}

/**
 * Fetch about/build info. Resolves build-time constants synchronously — no
 * network call. Pass `options.enabled` etc. to override.
 */
export function useAboutInfo(
  options?: Omit<UseQueryOptions<AboutInfo>, 'queryKey' | 'queryFn'>
) {
  return useQuery<AboutInfo>({
    queryKey: aboutKeys.info(),
    queryFn: async () => ABOUT_INFO,
    staleTime: Infinity,
    ...options,
  })
}
