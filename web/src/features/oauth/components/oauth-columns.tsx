/* eslint-disable react/only-export-components -- column definitions co-located with cell renderers */
// metapi-go/features/oauth — TanStack Table column definitions.
//
// `useOAuthColumns` is a hook (calls `useTranslation` during render) that
// returns the `ColumnDef<OAuthClient>[]` for the connections list page.
// Column `meta` flags drive the mobile-card layout (`mobileTitle` /
// `mobileBadge` / `mobileOrder` / `mobileHidden`) so the same definition
// serves the desktop table and the phone card list without a parallel
// layout file.

import type { ColumnDef } from '@tanstack/react-table'
import {
  Loader2 as Loader2Icon,
  MoreHorizontal as MoreHorizontalIcon,
  RefreshCw as RefreshCwIcon,
  Trash2 as Trash2Icon,
  Unplug as UnplugIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import type { OAuthClient, OAuthClientStatus } from '../types'

export type OAuthColumnActions = {
  onRefreshQuota: (client: OAuthClient) => void
  onRebind: (client: OAuthClient) => void
  onDelete: (client: OAuthClient) => void
}

const STATUS_LABEL_KEY: Record<OAuthClientStatus, string> = {
  healthy: 'oauth.status.healthy',
  abnormal: 'oauth.status.abnormal',
}

const STATUS_BADGE_VARIANT: Record<
  OAuthClientStatus,
  'default' | 'destructive'
> = {
  healthy: 'default',
  abnormal: 'destructive',
}

function StatusBadge({ status }: { status: OAuthClientStatus | undefined }) {
  const { t } = useTranslation()
  const resolved: OAuthClientStatus =
    status === 'abnormal' ? 'abnormal' : 'healthy'
  return (
    <Badge variant={STATUS_BADGE_VARIANT[resolved]}>
      {t(STATUS_LABEL_KEY[resolved])}
    </Badge>
  )
}

function TruncatedText({
  text,
  maxLength = 160,
}: {
  text: string
  maxLength?: number
}) {
  const display =
    text.length > maxLength ? `${text.slice(0, maxLength)}…` : text
  return (
    <span className='block max-w-[18rem] truncate text-sm' title={text}>
      {display}
    </span>
  )
}

function formatTimestamp(value?: string | null): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

// ---------------------------------------------------------------------------
// Row actions cell
// ---------------------------------------------------------------------------

/**
 * Per-row dropdown trigger + refresh/rebind/delete actions.
 *
 * The pending state is per-row (mirrors the accounts page's
 * `pendingStatusId` pattern): when `pendingAccountId === client.accountId`,
 * the trigger swaps `MoreHorizontal` for a `Loader2` spinner and the
 * refresh/rebind menu items are disabled so the user cannot spam-click the
 * same row into duplicate requests. The delete item stays enabled so the
 * connection can still be removed while a refresh/rebind is in flight.
 * Every other row stays fully clickable — there is no global lock.
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
            <Loader2Icon className='size-4 animate-spin' />
          ) : (
            <MoreHorizontalIcon className='size-4' />
          )}
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-48'>
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

// ---------------------------------------------------------------------------
// Columns hook
// ---------------------------------------------------------------------------

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
  const { t } = useTranslation()

  const columns: ColumnDef<OAuthClient>[] = [
    {
      id: 'provider',
      accessorKey: 'provider',
      size: 160,
      meta: { mobileTitle: true, mobileOrder: 0 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.provider')}
        />
      ),
      cell: ({ row }) => (
        <span className='font-medium'>{row.original.provider}</span>
      ),
    },
    {
      id: 'username',
      accessorFn: (row) => row.username ?? row.email ?? '',
      size: 200,
      meta: { mobileOrder: 1 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.username')}
        />
      ),
      cell: ({ row }) => {
        const client = row.original
        const label = client.username ?? client.email ?? client.accountKey
        return label ? (
          <TruncatedText text={label} />
        ) : (
          <span className='text-muted-foreground text-sm'>—</span>
        )
      },
    },
    {
      id: 'site',
      accessorFn: (row) => row.site?.name ?? '',
      size: 180,
      meta: { mobileHidden: true, mobileOrder: 10 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.site')}
        />
      ),
      cell: ({ row }) => {
        const siteName = row.original.site?.name
        return (
          <span className='text-muted-foreground text-sm'>
            {siteName ?? '—'}
          </span>
        )
      },
    },
    {
      id: 'status',
      accessorKey: 'status',
      size: 120,
      meta: { mobileBadge: true, mobileOrder: 2 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.status')}
        />
      ),
      cell: ({ row }) => <StatusBadge status={row.original.status} />,
      filterFn: (row, columnId, filterValue) => {
        const value = row.getValue(columnId) as OAuthClientStatus | undefined
        const resolved: OAuthClientStatus =
          value === 'abnormal' ? 'abnormal' : 'healthy'
        if (Array.isArray(filterValue)) {
          return filterValue.length === 0 || filterValue.includes(resolved)
        }
        return String(filterValue) === resolved
      },
    },
    {
      id: 'modelCount',
      accessorKey: 'modelCount',
      size: 120,
      meta: { mobileOrder: 20 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.modelCount')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {row.original.modelCount ?? 0}
        </span>
      ),
    },
    {
      id: 'lastModelSyncAt',
      accessorKey: 'lastModelSyncAt',
      size: 180,
      meta: { mobileHidden: true, mobileOrder: 30 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.lastSync')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm tabular-nums'>
          {formatTimestamp(row.original.lastModelSyncAt)}
        </span>
      ),
    },
    {
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
    },
  ]

  return columns
}

export const OAUTH_STATUS_FILTER_OPTIONS = [
  { label: 'oauth.status.healthy', value: 'healthy' },
  { label: 'oauth.status.abnormal', value: 'abnormal' },
]
