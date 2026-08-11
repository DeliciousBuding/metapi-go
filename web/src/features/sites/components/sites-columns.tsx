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
import { cn } from '@/lib/utils'

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
    <Badge variant={resolved === 'active' ? 'default' : 'secondary'}>
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
 * page so the columns stay free of mutation/query concerns.
 */
export function useSitesColumns(
  actions: SitesColumnActions
): ColumnDef<Site>[] {
  const { t } = useTranslation()

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
      meta: { mobileTitle: true, mobileOrder: 0 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.name')}
        />
      ),
      cell: ({ row }) => {
        const site = row.original
        return (
          <div className='flex items-center gap-2'>
            {site.isPinned && (
              <PinIcon className='text-muted-foreground size-3.5 shrink-0' />
            )}
            <span className='font-medium'>{site.name}</span>
          </div>
        )
      },
    },
    {
      id: 'url',
      accessorKey: 'url',
      size: 280,
      meta: { mobileHidden: true, mobileOrder: 10 },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('sites.columns.url')} />
      ),
      cell: ({ row }) => <TruncatedUrl url={row.original.url} />,
    },
    {
      id: 'status',
      accessorKey: 'status',
      size: 120,
      meta: { mobileBadge: true, mobileOrder: 1 },
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
      meta: { mobileHidden: true, mobileOrder: 20 },
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
      meta: { mobileOrder: 30 },
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
      id: 'globalWeight',
      accessorKey: 'globalWeight',
      size: 120,
      meta: { mobileHidden: true, mobileOrder: 40 },
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
                <DropdownMenuItem onClick={() => actions.onToggleStatus(site)}>
                  <PowerIcon className='text-muted-foreground/70 size-3.5' />
                  {isActive
                    ? t('sites.actions.disable')
                    : t('sites.actions.enable')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => actions.onTogglePin(site)}>
                  {site.isPinned ? (
                    <PinOffIcon className='text-muted-foreground/70 size-3.5' />
                  ) : (
                    <PinIcon className='text-muted-foreground/70 size-3.5' />
                  )}
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
