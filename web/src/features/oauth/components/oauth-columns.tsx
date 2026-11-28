import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { toBcp47 } from '@/i18n/languages'

import type { OAuthClient } from '../types'
import {
  oauthActionColumn,
  type OAuthColumnActions,
} from './oauth-action-column'
import { oauthIdentityColumns } from './oauth-identity-columns'
import { oauthStatusColumns } from './oauth-status-columns'
export { OAuthRowActions, type OAuthColumnActions } from './oauth-action-column'
export { OAuthStatusBadge } from './oauth-status-badge'

/**
 * Build the OAuth connections columns. Must be called during render (it is
 * a hook because it reads i18n state). The `actions` callbacks are supplied
 * by the page so the columns stay free of mutation/query concerns.
 * `pendingAccountId` threads the per-row pending state (derived from the
 * TanStack Query mutation's `variables` on the page) into the actions cell
 * — see {@link OAuthRowActions}.
 */
export function useOAuthColumns(
  actions: OAuthColumnActions,
  pendingAccountId: number | null = null
): ColumnDef<OAuthClient>[] {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')

  return [
    ...oauthIdentityColumns(t),
    ...oauthStatusColumns(t, locale),
    oauthActionColumn(t, actions, pendingAccountId),
  ]
}

export const OAUTH_STATUS_FILTER_OPTIONS = [
  { label: 'oauth.status.healthy', value: 'healthy' },
  { label: 'oauth.status.abnormal', value: 'abnormal' },
]
