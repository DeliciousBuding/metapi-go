import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import {
  Eye as EyeIcon,
  MoreHorizontal as MoreHorizontalIcon,
  RefreshCw as RefreshCwIcon,
  Trash2 as Trash2Icon,
  Unplug as UnplugIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'

import type { OAuthClient } from '../types'

export type OAuthColumnActions = {
  onViewDetails: (client: OAuthClient) => void
  onRefreshQuota: (client: OAuthClient) => void
  onRebind: (client: OAuthClient) => void
  onDelete: (client: OAuthClient) => void
}

/**
 * Per-row dropdown trigger + view-details/refresh/rebind/delete actions.
 *
 * The pending state is per-row (mirrors the accounts page's
 * `pendingStatusId` pattern): when `pendingAccountId === client.accountId`,
 * the trigger swaps `MoreHorizontal` for a `Spinner` and the
 * refresh/rebind menu items are disabled so the user cannot spam-click the
 * same row into duplicate requests. The view-details item stays enabled
 * (opening a read-only panel cannot conflict with an in-flight request) and
 * so does delete, so the connection can still be removed while a
 * refresh/rebind is in flight. Every other row stays fully clickable —
 * there is no global lock.
 */
export function OAuthRowActions({
  client,
  actions,
  pendingAccountId = null,
}: {
  client: OAuthClient
  actions: OAuthColumnActions
  pendingAccountId?: number | null
}) {
  const { t } = useTranslation()
  const isThisRowPending = pendingAccountId === client.accountId
  return (
    <div className={cn('flex justify-end')}>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              className='data-popup-open:bg-accent'
              aria-label={t('oauth.columns.rowActions')}
            />
          }
        >
          {isThisRowPending ? (
            <Spinner />
          ) : (
            <MoreHorizontalIcon className='size-4' />
          )}
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-48'>
          <DropdownMenuItem onClick={() => actions.onViewDetails(client)}>
            <EyeIcon className='text-muted-foreground/70 size-3.5' />
            {t('oauth.actions.viewDetails')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() => actions.onRefreshQuota(client)}
            disabled={isThisRowPending}
          >
            <RefreshCwIcon className='text-muted-foreground/70 size-3.5' />
            {t('oauth.actions.refreshQuota')}
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => actions.onRebind(client)}
            disabled={isThisRowPending}
          >
            <UnplugIcon className='text-muted-foreground/70 size-3.5' />
            {t('oauth.actions.rebind')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant='destructive'
            onClick={() => actions.onDelete(client)}
          >
            <Trash2Icon className='size-3.5' />
            {t('oauth.actions.delete')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

export function oauthActionColumn(
  t: TFunction,
  actions: OAuthColumnActions,
  pendingAccountId: number | null
): ColumnDef<OAuthClient> {
  return {
    id: 'actions',
    size: 56,
    enableSorting: false,
    enableHiding: false,
    enableResizing: false,
    meta: { mobileHidden: false, mobileOrder: 5 },
    header: () => (
      <span className='text-muted-foreground text-xs'>
        {t('oauth.columns.actions')}
      </span>
    ),
    cell: ({ row }) => (
      <OAuthRowActions
        client={row.original}
        actions={actions}
        pendingAccountId={pendingAccountId}
      />
    ),
  }
}
