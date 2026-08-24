/* eslint-disable react/only-export-components */
// metapi-go/features/sites — TanStack Table column definitions.
//
// `useSitesColumns` is a hook (calls `useTranslation` during render) that
// returns the `ColumnDef<Site>[]` for the list page. Column `meta` flags
// drive the mobile-card layout (`mobileTitle` / `mobileBadge` / `mobileOrder`
// / `mobileHidden`) so the same definition serves the desktop table and the
// phone card list without a parallel layout file.

import type { ColumnDef } from '@tanstack/react-table'
import {
  Eye as EyeIcon,
  MoreHorizontal as MoreHorizontalIcon,
  Pencil as PencilIcon,
  Pin as PinIcon,
  PinOff as PinOffIcon,
  Power as PowerIcon,
  Trash2 as Trash2Icon,
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
import { Spinner } from '@/components/ui/spinner'
import {
  formatAbsoluteDateTime,
  formatCurrency,
  formatRelativeTime,
} from '@/lib/format'
import { cn } from '@/lib/utils'

import { resolveSiteBalanceUsd } from '../lib/site-balance'
import type { Site, SiteStatus } from '../types'

export type SitesColumnActions = {
  onEdit: (site: Site) => void
  onView: (site: Site) => void
  onToggleStatus: (site: Site) => void
  onTogglePin: (site: Site) => void
  onDelete: (site: Site) => void
}

const STATUS_LABEL_KEY: Record<SiteStatus, string> = {
  active: 'sites.status.active',
  disabled: 'sites.status.disabled',
}

function StatusBadge({ status }: { status: SiteStatus | undefined }) {
  const { t } = useTranslation()
  const resolved: SiteStatus = status === 'disabled' ? 'disabled' : 'active'
  return (
    <Badge variant={resolved === 'active' ? 'success' : 'secondary'}>
      {t(STATUS_LABEL_KEY[resolved])}
    </Badge>
  )
}

function TruncatedUrl({ url }: { url: string }) {
  return (
    <span className='block max-w-[18rem] truncate text-sm' title={url}>
      {url}
    </span>
  )
}

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

  const columns: ColumnDef<Site>[] = [
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
          indeterminate={
            table.getIsSomeRowsSelected() && !table.getIsAllRowsSelected()
          }
          onCheckedChange={(value) => table.toggleAllRowsSelected(!!value)}
          aria-label={t('sites.columns.selectAll')}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          onClick={(event) => event.stopPropagation()}
          aria-label={t('sites.columns.selectRow')}
        />
      ),
    },
    {
      id: 'name',
      accessorKey: 'name',
      size: 200,
      meta: {
        label: t('sites.columns.name'),
        mobileTitle: true,
        mobileOrder: 0,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.name')}
        />
      ),
      cell: ({ row }) => {
        const site = row.original
        return (
          <div className='flex min-w-0 items-center gap-2'>
            {site.isPinned && (
              <PinIcon className='text-muted-foreground size-3.5 shrink-0' />
            )}
            <span
              className='max-w-[220px] truncate font-medium'
              title={site.name}
            >
              {site.name}
            </span>
          </div>
        )
      },
    },
    {
      id: 'url',
      accessorKey: 'url',
      size: 280,
      meta: {
        label: t('sites.columns.url'),
        mobileOrder: 10,
      },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('sites.columns.url')} />
      ),
      cell: ({ row }) => <TruncatedUrl url={row.original.url} />,
    },
    {
      id: 'status',
      accessorKey: 'status',
      size: 120,
      meta: {
        label: t('sites.columns.status'),
        mobileBadge: true,
        mobileOrder: 1,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.status')}
        />
      ),
      cell: ({ row }) => <StatusBadge status={row.original.status} />,
      filterFn: (row, columnId, filterValue) => {
        const value = row.getValue(columnId) as SiteStatus | undefined
        const resolved: SiteStatus =
          value === 'disabled' ? 'disabled' : 'active'
        if (Array.isArray(filterValue)) {
          return filterValue.length === 0 || filterValue.includes(resolved)
        }
        return String(filterValue) === resolved
      },
    },
    {
      id: 'platform',
      accessorKey: 'platform',
      size: 140,
      meta: {
        label: t('sites.columns.platform'),
        mobileOrder: 20,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.platform')}
        />
      ),
      cell: ({ row }) => {
        const platform = row.original.platform
        return (
          <span className='text-muted-foreground text-sm'>
            {platform ? platform : '—'}
          </span>
        )
      },
    },
    {
      id: 'accountCount',
      accessorKey: 'accountCount',
      size: 120,
      meta: { label: t('sites.columns.accountCount'), mobileOrder: 30 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.accountCount')}
        />
      ),
      cell: ({ row }) => {
        const count = row.original.accountCount
        const activeFromSubscription =
          row.original.subscriptionSummary?.activeCount
        let resolved: number | null = null
        if (typeof count === 'number') {
          resolved = count
        } else if (typeof activeFromSubscription === 'number') {
          resolved = activeFromSubscription
        }
        return (
          <span className='text-sm tabular-nums'>
            {resolved === null ? '—' : resolved}
          </span>
        )
      },
    },
    {
      id: 'balance',
      accessorFn: (row) => resolveSiteBalanceUsd(row),
      size: 140,
      meta: {
        label: t('sites.columns.balance'),
        mobileOrder: 35,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.balance')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {formatCurrency(resolveSiteBalanceUsd(row.original))}
        </span>
      ),
    },
    {
      id: 'globalWeight',
      accessorKey: 'globalWeight',
      size: 120,
      meta: {
        label: t('sites.columns.globalWeight'),
        mobileOrder: 40,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.globalWeight')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm tabular-nums'>
          {row.original.globalWeight ?? 1}
        </span>
      ),
    },
    {
      id: 'useSystemProxy',
      accessorKey: 'useSystemProxy',
      size: 130,
      meta: {
        label: t('sites.columns.useSystemProxy'),
        mobileHidden: true,
        mobileOrder: 45,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.useSystemProxy')}
        />
      ),
      cell: ({ row }) => {
        const enabled = row.original.useSystemProxy
        return (
          <Badge variant={enabled ? 'success' : 'secondary'}>
            {enabled ? t('sites.detail.yes') : t('sites.detail.no')}
          </Badge>
        )
      },
    },
    {
      id: 'externalCheckinUrl',
      accessorKey: 'externalCheckinUrl',
      size: 240,
      meta: {
        label: t('sites.columns.externalCheckinUrl'),
        mobileHidden: true,
        mobileOrder: 46,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.externalCheckinUrl')}
        />
      ),
      cell: ({ row }) => {
        const url = row.original.externalCheckinUrl
        if (!url) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <span className='block max-w-[14rem] truncate text-sm' title={url}>
            {url}
          </span>
        )
      },
    },
    {
      id: 'createdAt',
      accessorKey: 'createdAt',
      size: 170,
      meta: {
        label: t('sites.columns.createdAt'),
        mobileHidden: true,
        mobileOrder: 47,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.createdAt')}
        />
      ),
      cell: ({ row }) => {
        const createdAt = row.original.createdAt
        if (!createdAt) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <span
            className='text-muted-foreground text-sm whitespace-nowrap'
            title={formatAbsoluteDateTime(createdAt, i18n.language)}
          >
            {formatRelativeTime(createdAt, i18n.language)}
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
          {t('sites.columns.actions')}
        </span>
      ),
      cell: ({ row }) => {
        const site = row.original
        const isActive = site.status !== 'disabled'
        const isThisRowPending =
          pendingSiteId !== null && pendingSiteId === site.id

        let pinToggleIcon = (
          <PinIcon className='text-muted-foreground/70 size-3.5' />
        )
        if (isThisRowPending) {
          pinToggleIcon = <Spinner className='size-3.5' />
        } else if (site.isPinned) {
          pinToggleIcon = (
            <PinOffIcon className='text-muted-foreground/70 size-3.5' />
          )
        }

        return (
          <div className={cn('flex justify-end')}>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    className='data-popup-open:bg-accent'
                    aria-label={t('sites.columns.rowActions')}
                  />
                }
              >
                <MoreHorizontalIcon className='size-4' />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end' className='w-44'>
                <DropdownMenuItem onClick={() => actions.onView(site)}>
                  <EyeIcon className='text-muted-foreground/70 size-3.5' />
                  {t('sites.actions.viewDetails')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => actions.onEdit(site)}>
                  <PencilIcon className='text-muted-foreground/70 size-3.5' />
                  {t('sites.actions.edit')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={isThisRowPending}
                  onClick={() => actions.onToggleStatus(site)}
                >
                  {isThisRowPending ? (
                    <Spinner className='size-3.5' />
                  ) : (
                    <PowerIcon className='text-muted-foreground/70 size-3.5' />
                  )}
                  {isActive
                    ? t('sites.actions.disable')
                    : t('sites.actions.enable')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={isThisRowPending}
                  onClick={() => actions.onTogglePin(site)}
                >
                  {pinToggleIcon}
                  {site.isPinned
                    ? t('sites.actions.unpin')
                    : t('sites.actions.pin')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant='destructive'
                  onClick={() => actions.onDelete(site)}
                >
                  <Trash2Icon className='size-3.5' />
                  {t('sites.actions.delete')}
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

export const SITES_STATUS_FILTER_OPTIONS = [
  { label: 'sites.status.active', value: 'active' },
  { label: 'sites.status.disabled', value: 'disabled' },
]
