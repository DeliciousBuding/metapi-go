/* eslint-disable react/only-export-components -- column definitions co-located with cell renderers */
// metapi-go features/checkin/components — TanStack Table column definitions.
// i18n: all user-visible strings migrated to t() calls.

import type { ColumnDef } from '@tanstack/react-table'
import { Eye, MoreHorizontal, Play } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import { formatCheckinLogTime } from '../lib/checkin-time'
import {
  type CheckinLogRow,
  type CheckinRowActions,
  checkinLogRowSchema,
} from '../types'
import { FailureReasonBadge } from './failure-reason-badge'

interface StatusConfig {
  variant: 'default' | 'secondary' | 'destructive'
  dotClassName: string
  labelKey: string
}

function resolveStatusConfig(status: string): StatusConfig {
  if (status === 'success')
    return {
      variant: 'default',
      dotClassName: 'bg-emerald-500',
      labelKey: 'checkin.columns.statusSuccess',
    }
  if (status === 'skipped')
    return {
      variant: 'secondary',
      dotClassName: 'bg-muted-foreground',
      labelKey: 'checkin.columns.statusSkipped',
    }
  return {
    variant: 'destructive',
    dotClassName: 'bg-red-500',
    labelKey: 'checkin.columns.statusFailed',
  }
}

function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  const config = resolveStatusConfig(status)
  return (
    <Badge variant={config.variant}>
      <span className={cn('size-1.5 rounded-full', config.dotClassName)} />
      {t(config.labelKey)}
    </Badge>
  )
}

function CheckinRowActions({
  row,
  actions,
}: {
  row: CheckinLogRow
  actions: CheckinRowActions
}) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className='text-muted-foreground hover:bg-muted hover:text-foreground inline-flex size-8 items-center justify-center rounded-md transition-colors outline-none'
        aria-label={t('checkin.columns.rowActions')}
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' sideOffset={4}>
        <DropdownMenuItem onClick={() => actions.onViewDetail(row)}>
          <Eye />
          {t('checkin.columns.viewDetails')}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => actions.onTriggerAccount(row)}>
          <Play />
          {t('checkin.columns.triggerCheckin')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function useCheckinColumns(
  actions: CheckinRowActions
): ColumnDef<CheckinLogRow>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'select',
      size: 40,
      enableSorting: false,
      enableHiding: false,
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          onCheckedChange={(value) =>
            table.toggleAllPageRowsSelected(Boolean(value))
          }
          aria-label={t('checkin.columns.selectAll')}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(Boolean(value))}
          aria-label={t('checkin.columns.selectRow')}
        />
      ),
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'checkin_logs.createdAt',
      id: 'createdAt',
      header: t('checkin.columns.createdAt'),
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        return (
          <span className='text-sm tabular-nums'>
            {formatCheckinLogTime(log.checkin_logs.createdAt)}
          </span>
        )
      },
      meta: { mobileTitle: true },
    },
    {
      id: 'account',
      header: t('checkin.columns.account'),
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const username = log.accounts?.username
        return (
          <span className='max-w-[160px] truncate'>
            {username || `#${log.checkin_logs.accountId}`}
          </span>
        )
      },
      meta: { mobileOrder: 1 },
    },
    {
      id: 'site',
      header: t('checkin.columns.site'),
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const site = log.sites
        if (!site || (!site.name && !site.url))
          return <span className='text-muted-foreground'>—</span>
        return (
          <span className='max-w-[160px] truncate'>
            {site.name || site.url}
          </span>
        )
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) return true
        const log = checkinLogRowSchema.parse(row.original)
        return filterValue.includes(log.sites?.name ?? '')
      },
      meta: { mobileOrder: 2 },
    },
    {
      id: 'status',
      header: t('common.status'),
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        return <StatusBadge status={log.checkin_logs.status} />
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) return true
        const log = checkinLogRowSchema.parse(row.original)
        return filterValue.includes(log.checkin_logs.status)
      },
      meta: { mobileBadge: true },
    },
    {
      id: 'reason',
      header: t('checkin.columns.failureReason'),
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        return <FailureReasonBadge reason={log.failureReason} />
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) return true
        const log = checkinLogRowSchema.parse(row.original)
        const category = log.failureReason?.category
        if (!category) return false
        return filterValue.includes(category)
      },
      meta: { mobileOrder: 3 },
    },
    {
      id: 'message',
      header: t('checkin.columns.message'),
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const message = log.checkin_logs.message
        if (!message) return <span className='text-muted-foreground'>—</span>
        return (
          <span
            className='text-muted-foreground block max-w-[360px] truncate text-sm'
            title={message}
          >
            {message}
          </span>
        )
      },
      meta: { mobileHidden: true },
    },
    {
      id: 'reward',
      header: t('checkin.columns.reward'),
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const reward = log.checkin_logs.reward
        if (!reward) return <span className='text-muted-foreground'>—</span>
        return <span className='text-sm tabular-nums'>{reward}</span>
      },
      meta: { mobileHidden: true },
    },
    {
      id: 'actions',
      size: 48,
      enableSorting: false,
      enableHiding: false,
      header: () => <span className='sr-only'>{t('common.actions')}</span>,
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        return <CheckinRowActions row={log} actions={actions} />
      },
      meta: { pinned: 'right' },
    },
  ]
}
