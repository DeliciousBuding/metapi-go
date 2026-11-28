// metapi-go/features/oauth — list page.
//
// Wires the four-layer data-table to the OAuth connections query and the
// URL. Table state (search query / page / page-size / sorting / status
// faceted filter) is mirrored to the URL search string so a deep link
// restores the exact view. The "start authorization" dialog opens an
// OAuth flow in a new tab. Row actions: view details (opens the read-only
// detail sheet), refresh quota, rebind (re-run the OAuth flow), delete.
// Mobile cards are handled by `DataTablePage` via the column `meta` flags.

import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { toast } from '@/lib/toast'

import {
  useOAuthConnections,
  useRebindOAuthConnection,
  useRefreshOAuthQuota,
} from '../api'
import { resolveOAuthConnectionLabel } from '../lib/oauth-connection-label'
import type { OAuthClient } from '../types'
import { OAuthConnectionsTable } from './oauth-connections-table'
import { OAuthPageDialogs, useOAuthDialogs } from './oauth-page-dialogs'
import { OAuthPageHeader } from './oauth-page-header'
import { useOAuthUrlState } from './use-oauth-url-state'

/**
 * Pick the account id of whichever mutation (refresh-quota or rebind) is
 * currently pending. Both mutations take a bare `accountId: number` as
 * variables. Returns `null` when neither is in flight — the actions cell
 * renders all rows as clickable. Kept as a plain function (not a hook) so
 * the page render stays cheap and the derivation is grep-able/testable.
 */
function resolvePendingAccountId(
  refresh: { isPending: boolean; variables?: number },
  rebind: { isPending: boolean; variables?: number }
): number | null {
  if (refresh.isPending) return refresh.variables ?? null
  if (rebind.isPending) return rebind.variables ?? null
  return null
}

export function OAuthPage() {
  const { t } = useTranslation()
  const urlState = useOAuthUrlState()
  const connectionsQuery = useOAuthConnections({
    page: urlState.pagination.pageIndex,
    pageSize: urlState.pagination.pageSize,
  })
  const rawConnections = connectionsQuery.data?.items
  const connections = useMemo(() => rawConnections ?? [], [rawConnections])
  const connectionsTotal = connectionsQuery.data?.total ?? 0
  const refreshQuota = useRefreshOAuthQuota()
  const rebindConnection = useRebindOAuthConnection()

  const dialogs = useOAuthDialogs(connections)

  // Derive the per-row pending account id from whichever mutation is in
  // flight. A single value is sufficient because BOTH row actions (refresh +
  // rebind) should be disabled for the row with an action pending, while
  // every other row stays clickable (no global lock) — mirroring the
  // accounts page's `pendingStatusId` pattern. `useRefreshOAuthQuota` and
  // `useRebindOAuthConnection` both take a bare `accountId: number` as
  // variables, so the pending id is `mutation.variables` when pending.
  const pendingAccountId = resolvePendingAccountId(
    refreshQuota,
    rebindConnection
  )

  // Both entry points (row action menu + detail sheet footer) call these, so
  // there is exactly one mutation instance and one toast vocabulary per
  // operation.
  function handleRefreshQuota(client: OAuthClient) {
    refreshQuota.mutate(client.accountId, {
      onSuccess: () =>
        toast.success(
          t('oauth.toast.quotaRefreshed', { id: client.accountId })
        ),
      onError: () =>
        toast.error(
          t('oauth.toast.refreshFailed', {
            name: resolveOAuthConnectionLabel(client),
          })
        ),
    })
  }

  function handleRebind(client: OAuthClient) {
    rebindConnection.mutate(client.accountId, {
      onSuccess: (result) => {
        if (result.authorizationUrl) {
          window.open(result.authorizationUrl, '_blank', 'noopener,noreferrer')
        }
        toast.success(t('oauth.toast.rebindStarted', { id: client.accountId }))
      },
      onError: () =>
        toast.error(
          t('oauth.toast.rebindFailed', {
            name: resolveOAuthConnectionLabel(client),
          })
        ),
    })
  }

  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <OAuthPageHeader
        connectionsQuery={connectionsQuery}
        connections={connections}
        connectionsTotal={connectionsTotal}
      />
      <OAuthConnectionsTable
        urlState={urlState}
        connectionsQuery={connectionsQuery}
        connections={connections}
        connectionsTotal={connectionsTotal}
        pendingAccountId={pendingAccountId}
        handleStart={dialogs.openStart}
        actions={{
          onViewDetails: dialogs.openDetails,
          onRefreshQuota: handleRefreshQuota,
          onRebind: handleRebind,
          onDelete: dialogs.setDeletingClient,
        }}
      />
      <OAuthPageDialogs
        dialogs={dialogs}
        handleRefreshQuota={handleRefreshQuota}
        handleRebind={handleRebind}
        isRefreshingQuota={
          refreshQuota.isPending &&
          refreshQuota.variables === dialogs.detailConnection?.accountId
        }
        isRebinding={
          rebindConnection.isPending &&
          rebindConnection.variables === dialogs.detailConnection?.accountId
        }
      />
    </div>
  )
}
