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
  Eye as EyeIcon,
  MoreHorizontal as MoreHorizontalIcon,
  RefreshCw as RefreshCwIcon,
  Trash2 as Trash2Icon,
  TriangleAlert as TriangleAlertIcon,
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
import { Spinner } from '@/components/ui/spinner'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime, formatInt } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { OAuthClient, OAuthClientStatus } from '../types'

export type OAuthColumnActions = {
  onViewDetails: (client: OAuthClient) => void
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
  'success' | 'destructive'
> = {
  healthy: 'success',
  abnormal: 'destructive',
}

/**
 * Health badge for one connection. Exported so the detail sheet renders the
 * exact same semantic variant as the list column (a private copy would let
 * the two drift).
 */
export function OAuthStatusBadge({
  status,
}: {
  status: OAuthClientStatus | undefined
}) {
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
    <span className='block max-w-[9.5rem] truncate text-sm' title={text}>
      {display}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Quota cell
// ---------------------------------------------------------------------------

type OAuthQuotaWindow = NonNullable<OAuthClient['quota']>['windows']['fiveHour']

/** Compact `used/limit` pair for one quota window ("12/50"). */
function formatQuotaWindowUsage(window: OAuthQuotaWindow): string {
  if (window.used != null && window.limit != null) {
    return `${window.used}/${window.limit}`
  }
  if (window.used != null) return `${window.used}`
  if (window.limit != null) return `—/${window.limit}`
  return '—'
}

/**
 * Renders the provider quota windows (5-hour / 7-day) as compact
 * `used/limit` lines. Only supported windows that carry at least one number
 * render; anything else (unsupported provider, sync error, empty payload)
 * degrades to an em dash instead of a fabricated "0/0".
 */
function QuotaCell({ client }: { client: OAuthClient }) {
  const { t } = useTranslation()
  const quota = client.quota
  if (!quota || quota.status !== 'supported' || !quota.windows) {
    return <span className='text-muted-foreground text-sm'>—</span>
  }
  const supportedWindows = [
    { labelKey: 'oauth.columns.quotaFiveHour', window: quota.windows.fiveHour },
    { labelKey: 'oauth.columns.quotaSevenDay', window: quota.windows.sevenDay },
  ].filter(
    ({ window }) =>
      window.supported && (window.used != null || window.limit != null)
  )
  if (supportedWindows.length === 0) {
    return <span className='text-muted-foreground text-sm'>—</span>
  }
  return (
    <div className='flex flex-col gap-0.5 text-xs tabular-nums'>
      {supportedWindows.map(({ labelKey, window }) => (
        <span key={labelKey} className='whitespace-nowrap'>
          <span className='text-muted-foreground'>{t(labelKey)}</span>{' '}
          {formatQuotaWindowUsage(window)}
        </span>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Row actions cell
// ---------------------------------------------------------------------------

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
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')

  const columns: ColumnDef<OAuthClient>[] = [
    {
      id: 'provider',
      accessorKey: 'provider',
      size: 100,
      meta: { mobileTitle: true, mobileOrder: 0 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.provider')}
        />
      ),
      cell: ({ row }) => (
        <span
          className='block max-w-[5.5rem] truncate font-medium'
          title={row.original.provider}
        >
          {row.original.provider}
        </span>
      ),
    },
    {
      id: 'username',
      accessorFn: (row) => row.username ?? row.email ?? '',
      size: 170,
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
      size: 100,
      meta: { mobileHidden: true, mobileOrder: 10 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.site')}
        />
      ),
      cell: ({ row }) => {
        const siteName = row.original.site?.name
        return siteName ? (
          <span
            className='text-muted-foreground block max-w-[6rem] truncate text-sm'
            title={siteName}
          >
            {siteName}
          </span>
        ) : (
          <span className='text-muted-foreground text-sm'>—</span>
        )
      },
    },
    {
      id: 'status',
      accessorKey: 'status',
      size: 100,
      meta: { mobileBadge: true, mobileOrder: 2 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.status')}
        />
      ),
      cell: ({ row }) => <OAuthStatusBadge status={row.original.status} />,
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
      id: 'planType',
      accessorFn: (row) =>
        row.planType ?? row.quota?.subscription?.planType ?? '',
      size: 85,
      meta: { mobileHidden: true, mobileOrder: 21 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.planType')}
        />
      ),
      cell: ({ row }) => {
        const planType =
          row.original.planType ??
          row.original.quota?.subscription?.planType ??
          null
        return (
          <span className='text-muted-foreground text-sm'>
            {planType ?? '—'}
          </span>
        )
      },
    },
    {
      id: 'quota',
      enableSorting: false,
      size: 100,
      meta: { mobileHidden: true, mobileOrder: 22 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.quota')}
        />
      ),
      cell: ({ row }) => <QuotaCell client={row.original} />,
    },
    {
      id: 'modelCount',
      accessorKey: 'modelCount',
      size: 85,
      meta: { mobileOrder: 20 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.modelCount')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {formatInt(row.original.modelCount)}
        </span>
      ),
    },
    {
      id: 'routeChannelCount',
      accessorFn: (row) => row.routeChannelCount ?? -1,
      size: 150,
      meta: { mobileHidden: true, mobileOrder: 23 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.routeChannelCount')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {row.original.routeChannelCount ?? '—'}
        </span>
      ),
    },
    {
      id: 'lastModelSyncAt',
      accessorKey: 'lastModelSyncAt',
      size: 135,
      meta: { mobileHidden: true, mobileOrder: 30 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.lastSync')}
        />
      ),
      cell: ({ row }) => (
        <span
          className='text-muted-foreground block max-w-[7rem] truncate text-sm tabular-nums'
          title={formatDateTime(row.original.lastModelSyncAt, locale)}
        >
          {formatDateTime(row.original.lastModelSyncAt, locale)}
        </span>
      ),
    },
    {
      id: 'lastModelSyncError',
      enableSorting: false,
      size: 135,
      meta: { mobileHidden: true, mobileOrder: 31 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.lastModelSyncError')}
        />
      ),
      cell: ({ row }) => {
        const syncError = row.original.lastModelSyncError?.trim() || null
        if (!syncError) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <span
            className='flex max-w-28 items-center gap-1.5 text-sm'
            title={syncError}
          >
            <TriangleAlertIcon
              className='text-warning size-4 shrink-0'
              aria-label={t('oauth.columns.lastModelSyncError')}
            />
            <span className='text-muted-foreground truncate'>{syncError}</span>
          </span>
        )
      },
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
