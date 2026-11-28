import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import type { Site } from '../types'
import { actionColumns, type SitesColumnActions } from './sites-action-columns'
import { identityColumns } from './sites-identity-columns'
import { operationalColumns } from './sites-operational-columns'
import { statusColumns } from './sites-status-columns'
export type { SitesColumnActions } from './sites-action-columns'

/**
 * Build the site list columns. Must be called during render (it is a hook
 * because it reads i18n state). The `actions` callbacks are supplied by the
 * page so the columns stay free of mutation/query concerns. `pendingSiteId`
 * threads per-row pending state: the row whose status/pin update is in
 * flight disables those toggles and shows a spinner (no global lock).
 */
export function useSitesColumns(
  actions: SitesColumnActions,
  pendingSiteId: number | null = null
): ColumnDef<Site>[] {
  const { t, i18n } = useTranslation()
  return [
    ...identityColumns(t),
    ...statusColumns(t),
    ...operationalColumns(t, i18n),
    ...actionColumns(t, actions, pendingSiteId),
  ]
}

export const SITES_STATUS_FILTER_OPTIONS = [
  { label: 'sites.status.active', value: 'active' },
  { label: 'sites.status.disabled', value: 'disabled' },
]
