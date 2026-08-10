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
  MoreHorizontal as MoreHorizontalIcon,
  RefreshCw as RefreshCwIcon,
  Trash2 as Trash2Icon,
  Unplug as UnplugIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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

function TruncatedText({ text, maxLength = 160 }: { text: string; maxLength?: number }) {
  const display = text.length > maxLength ? `${text.slice(0, maxLength)}…` : text
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

/**
 * Build the OAuth connections columns. Must be called during render (it is
 * a hook because it reads i18n state). The `actions` callbacks are supplied
 * by the page so the columns stay free of mutation/query concerns.
 */
export function useOAuthColumns(actions: OAuthColumnActions): ColumnDef<OAuthClient>[] {
  const { t } = useTranslation()

  const columns: ColumnDef<OAuthClient>[] = [
    {
      id: 'select',
      enableSorting: false,
      enableHiding: false,
      enableResizing: false,
      size: 36,
      meta: { mobileHidden: true },
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllRowsSelected()}
          indeterminate={table.getIsSomeRowsSelected() && !table.getIsAllRowsSelected()}
          onCheckedChange={(value) => table.toggleAllRowsSelected(!!value)}
          aria-label={t('oauth.columns.selectAll')}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          onClick={(event) => event.stopPropagation()}
          aria-label={t('oauth.columns.selectRow')}
        />
      ),
    },
    {
      id: 'provider',
      accessorKey: 'provider',
      size: 160,
      meta: { mobileTitle: true, mobileOrder: 0 },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('oauth.columns.provider')} />
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
        <DataTableColumnHeader column={column} title={t('oauth.columns.username')} />
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
        <DataTableColumnHeader column={column} title={t('oauth.columns.site')} />
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
        <DataTableColumnHeader column={column} title={t('oauth.columns.status')} />
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
        <DataTableColumnHeader column={column} title={t('oauth.columns.modelCount')} />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>{row.original.modelCount ?? 0}</span>
      ),
    },
    {
      id: 'lastModelSyncAt',
      accessorKey: 'lastModelSyncAt',
      size: 180,
      meta: { mobileHidden: true, mobileOrder: 30 },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('oauth.columns.lastSync')} />
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
        <span className='text-muted-foreground text-xs'>{t('oauth.columns.actions')}</span>
      ),
      cell: ({ row }) => {
        const client = row.original
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
                <MoreHorizontalIcon className='size-4' />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end' className='w-48'>
                <DropdownMenuItem onClick={() => actions.onRefreshQuota(client)}>
                  <RefreshCwIcon className='text-muted-foreground/70 size-3.5' />
                  {t('oauth.actions.refreshQuota')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => actions.onRebind(client)}>
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
      },
    },
  ]

  return columns
}

export const OAUTH_STATUS_FILTER_OPTIONS = [
  { label: 'oauth.status.healthy', value: 'healthy' },
  { label: 'oauth.status.abnormal', value: 'abnormal' },
]
