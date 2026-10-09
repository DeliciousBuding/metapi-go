import { analyzePrimarySiteUrl } from '@/features/sites'
import type { SiteInitializationPreset } from '@/lib/api/sites'

import type { Site } from '../types'

/** Keep provider metadata on the server; match the selected connection exactly. */
export function findAccountInitializationPreset(
  site: Pick<Site, 'platform' | 'url'> | null | undefined,
  presets: readonly SiteInitializationPreset[] | undefined
): SiteInitializationPreset | undefined {
  if (!site?.url?.trim() || !site.platform?.trim()) return undefined
  const platform = site.platform.trim().toLowerCase()
  const url = analyzePrimarySiteUrl(site.url).persistedUrl
  return presets?.find(
    (preset) =>
      preset.platform.trim().toLowerCase() === platform &&
      preset.defaultUrl.trim() !== '' &&
      analyzePrimarySiteUrl(preset.defaultUrl).persistedUrl === url
  )
}
