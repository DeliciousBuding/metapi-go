/* eslint-disable react/only-export-components -- column definitions co-located with cell renderers */
// metapi-go/features/site-announcements — TanStack Table column definitions.
//
// `useAnnouncementsColumns` is a hook (calls `useTranslation` during render)
// that returns the `ColumnDef<SiteAnnouncement>[]` for the list page.
// Column `meta` flags drive the mobile-card layout (`mobileTitle` /
// `mobileBadge` / `mobileOrder` / `mobileHidden`) so the same definition
// serves the desktop table and the phone card list without a parallel
// layout file.

import type { ColumnDef } from '@tanstack/react-table'
import {
  ExternalLink as ExternalLinkIcon,
  MoreHorizontal as MoreHorizontalIcon,
  Pencil as PencilIcon,
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

import type { AnnouncementSeverity, SiteAnnouncement } from '../types'

export type AnnouncementsColumnActions = {
  onEdit: (item: SiteAnnouncement) => void
  onDelete: (item: SiteAnnouncement) => void
}

const SEVERITY_LABEL_KEY: Record<AnnouncementSeverity, string> = {
  info: 'siteAnnouncements.severity.info',
  warning: 'siteAnnouncements.severity.warning',
  critical: 'siteAnnouncements.severity.critical',
}

const SEVERITY_BADGE_VARIANT: Record<
  AnnouncementSeverity,
  'default' | 'secondary' | 'destructive'
> = {
  info: 'secondary',
  warning: 'default',
  critical: 'destructive',
}

function SeverityBadge({
  severity,
}: {
  severity: AnnouncementSeverity | undefined
}) {
  const { t } = useTranslation()
  const resolved: AnnouncementSeverity = severity ?? 'info'
  return (
    <Badge variant={SEVERITY_BADGE_VARIANT[resolved]}>
      {t(SEVERITY_LABEL_KEY[resolved])}
    </Badge>
  )
}

function EnabledBadge({ enabled }: { enabled: boolean | undefined }) {
  const { t } = useTranslation()
  const isOn = enabled ?? false
  return (
    <Badge variant={isOn ? 'default' : 'secondary'}>
      {isOn
        ? t('siteAnnouncements.enabled.on')
        : t('siteAnnouncements.enabled.off')}
    </Badge>
  )
}

function formatTimestamp(value?: string | null): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

/**
 * Build the announcement list columns. Must be called during render (it is
 * a hook because it reads i18n state). The `actions` callbacks are supplied
 * by the page so the columns stay free of mutation/query concerns.
 */
export function useAnnouncementsColumns(
  actions: AnnouncementsColumnActions
): ColumnDef<SiteAnnouncement>[] {
  const { t } = useTranslation()

  const columns: ColumnDef<SiteAnnouncement>[] = [
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
          aria-label={t('siteAnnouncements.columns.selectAll')}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          onClick={(event) => event.stopPropagation()}
          aria-label={t('siteAnnouncements.columns.selectRow')}
        />
      ),
    },
    {
      id: 'title',
      accessorKey: 'title',
      size: 240,
      meta: { mobileTitle: true, mobileOrder: 0 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('siteAnnouncements.columns.title')}
        />
      ),
      cell: ({ row }) => (
        <span className='font-medium'>{row.original.title}</span>
      ),
    },
    {
      id: 'severity',
      accessorKey: 'severity',
      size: 120,
      meta: { mobileBadge: true, mobileOrder: 1 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('siteAnnouncements.columns.severity')}
        />
      ),
      cell: ({ row }) => <SeverityBadge severity={row.original.severity} />,
      filterFn: (row, columnId, filterValue) => {
        const value = row.getValue(columnId) as AnnouncementSeverity | undefined
        const resolved: AnnouncementSeverity = value ?? 'info'
        if (Array.isArray(filterValue)) {
          return filterValue.length === 0 || filterValue.includes(resolved)
        }
        return String(filterValue) === resolved
      },
    },
    {
      id: 'enabled',
      accessorKey: 'enabled',
      size: 110,
      meta: { mobileHidden: true, mobileOrder: 10 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('siteAnnouncements.columns.enabled')}
        />
      ),
      cell: ({ row }) => <EnabledBadge enabled={row.original.enabled} />,
      filterFn: (row, columnId, filterValue) => {
        const value = row.getValue(columnId) as boolean | undefined
        const resolved = value ? 'true' : 'false'
        if (Array.isArray(filterValue)) {
          if (filterValue.length === 0) return true
          return filterValue.includes(resolved)
        }
        return String(filterValue) === resolved
      },
    },
    {
      id: 'link',
      accessorKey: 'link',
      size: 80,
      enableSorting: false,
      meta: { mobileHidden: true, mobileOrder: 20 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('siteAnnouncements.columns.link')}
        />
      ),
      cell: ({ row }) => {
        const link = row.original.link
        if (!link) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <a
            href={link}
            target='_blank'
            rel='noopener noreferrer'
            className='text-muted-foreground hover:text-foreground inline-flex transition-colors'
            aria-label={t('siteAnnouncements.columns.openLink')}
          >
            <ExternalLinkIcon className='size-4' />
          </a>
        )
      },
    },
    {
      id: 'createdAt',
      accessorKey: 'createdAt',
      size: 180,
      meta: { mobileHidden: true, mobileOrder: 30 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('siteAnnouncements.columns.createdAt')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm tabular-nums'>
          {formatTimestamp(row.original.createdAt)}
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
          {t('siteAnnouncements.columns.actions')}
        </span>
      ),
      cell: ({ row }) => {
        const item = row.original
        return (
          <div className={cn('flex justify-end')}>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    className='data-popup-open:bg-accent'
                    aria-label={t('siteAnnouncements.columns.rowActions')}
                  />
                }
              >
                <MoreHorizontalIcon className='size-4' />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end' className='w-44'>
                <DropdownMenuItem onClick={() => actions.onEdit(item)}>
                  <PencilIcon className='text-muted-foreground/70 size-3.5' />
                  {t('siteAnnouncements.actions.edit')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant='destructive'
                  onClick={() => actions.onDelete(item)}
                >
                  <Trash2Icon className='size-3.5' />
                  {t('siteAnnouncements.actions.delete')}
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

export const ANNOUNCEMENTS_SEVERITY_FILTER_OPTIONS = [
  { label: 'siteAnnouncements.severity.info', value: 'info' },
  { label: 'siteAnnouncements.severity.warning', value: 'warning' },
  { label: 'siteAnnouncements.severity.critical', value: 'critical' },
]

export const ANNOUNCEMENTS_ENABLED_FILTER_OPTIONS = [
  { label: 'siteAnnouncements.enabled.on', value: 'true' },
  { label: 'siteAnnouncements.enabled.off', value: 'false' },
]
