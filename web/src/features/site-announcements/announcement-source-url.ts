import type { Site } from '@/features/sites/types'

import type { SiteAnnouncement } from './types'

function httpURL(value: string, base?: string): URL | null {
  try {
    const url = base ? new URL(value, base) : new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url : null
  } catch {
    return null
  }
}

/**
 * Resolve an untrusted upstream announcement URL against the trusted local
 * Site URL. Only same-origin HTTP(S) detail URLs are accepted; hostile,
 * protocol-relative, cross-origin or malformed values fall back to the Site
 * home page. No valid Site URL means no external navigation affordance.
 */
export function resolveAnnouncementSourceURL(
  item: SiteAnnouncement,
  site: Site | undefined
): string | null {
  const siteURL = site?.url ? httpURL(site.url) : null
  if (!siteURL) return null

  const rawSource = item.sourceUrl?.trim()
  if (!rawSource) return siteURL.href

  const candidate = httpURL(rawSource, siteURL.href)
  if (!candidate || candidate.origin !== siteURL.origin) return siteURL.href
  return candidate.href
}
