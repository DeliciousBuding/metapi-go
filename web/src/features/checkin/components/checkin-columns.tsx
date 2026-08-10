// metapi-go features/checkin/components — TanStack Table column definitions
// for the checkin log list. Column meta drives the mobile card layout
// (mobileTitle / mobileBadge / mobileHidden / mobileOrder) so the
// data-table package's automatic mobile degradation needs zero per-feature
// card code — same pattern as accounts-columns.tsx.

import type { ColumnDef } from '@tanstack/react-table'
import { Eye, MoreHorizontal, Play } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import {
  type CheckinLogRow,
  type CheckinRowActions,
  checkinLogRowSchema,
} from '../types'
import { formatCheckinLogTime } from '../lib/checkin-time'
import { FailureReasonBadge } from './failure-reason-badge'

// ---------------------------------------------------------------------------
// Status badge
// ---------------------------------------------------------------------------

interface StatusConfig {
  variant: 'default' | 'secondary' | 'destructive'
  dotClassName: string
  label: string
}

function resolveStatusConfig(status: string): StatusConfig {
  if (status === 'success') {
    return { variant: 'default', dotClassName: 'bg-emerald-500', label: '成功' }
  }
  if (status === 'skipped') {
    return {
      variant: 'secondary',
      dotClassName: 'bg-muted-foreground',
      label: '跳过',
    }
  }
  return { variant: 'destructive', dotClassName: 'bg-red-500', label: '失败' }
}

function StatusBadge({ status }: { status: string }) {
  const config = resolveStatusConfig(status)
  return (
    <Badge variant={config.variant}>
      <span className={cn('size-1.5 rounded-full', config.dotClassName)} />
      {config.label}
    </Badge>
  )
}

// ---------------------------------------------------------------------------
// Row actions cell
// ---------------------------------------------------------------------------

function CheckinRowActions({
  row,
  actions,
}: {
  row: CheckinLogRow
  actions: CheckinRowActions
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className='inline-flex size-8 items-center justify-center rounded-md text-muted-foreground outline-none transition-colors hover:bg-muted hover:text-foreground'
        aria-label='签到记录操作'
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' sideOffset={4}>
        <DropdownMenuItem onClick={() => actions.onViewDetail(row)}>
          <Eye />
          查看详情
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => actions.onTriggerAccount(row)}>
          <Play />
          触发签到
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// ---------------------------------------------------------------------------
// Columns hook
// ---------------------------------------------------------------------------

export function useCheckinColumns(
  actions: CheckinRowActions,
): ColumnDef<CheckinLogRow>[] {
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
          aria-label='全选'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(Boolean(value))}
          aria-label='选择此行'
        />
      ),
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'checkin_logs.createdAt',
      id: 'createdAt',
      header: '签到时间',
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const createdAt = log.checkin_logs.createdAt
        return (
          <span className='tabular-nums text-sm'>
            {formatCheckinLogTime(createdAt)}
          </span>
        )
      },
      meta: { mobileTitle: true },
    },
    {
      id: 'account',
      header: '账号',
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const username = log.accounts?.username
        return (
          <span className='truncate max-w-[160px]'>
            {username || `#${log.checkin_logs.accountId}`}
          </span>
        )
      },
      meta: { mobileOrder: 1 },
    },
    {
      id: 'site',
      header: '站点',
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const site = log.sites
        if (!site || (!site.name && !site.url)) {
          return <span className='text-muted-foreground'>—</span>
        }
        return (
          <span className='truncate max-w-[160px]'>
            {site.name || site.url}
          </span>
        )
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) return true
        const log = checkinLogRowSchema.parse(row.original)
        const siteName = log.sites?.name ?? ''
        return filterValue.includes(siteName)
      },
      meta: { mobileOrder: 2 },
    },
    {
      id: 'status',
      header: '状态',
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
      header: '失败原因',
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
      header: '信息',
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const message = log.checkin_logs.message
        if (!message) return <span className='text-muted-foreground'>—</span>
        return (
          <span
            className='block max-w-[360px] truncate text-sm text-muted-foreground'
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
      header: '奖励',
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        const reward = log.checkin_logs.reward
        if (!reward) return <span className='text-muted-foreground'>—</span>
        return <span className='tabular-nums text-sm'>{reward}</span>
      },
      meta: { mobileHidden: true },
    },
    {
      id: 'actions',
      size: 48,
      enableSorting: false,
      enableHiding: false,
      header: () => <span className='sr-only'>操作</span>,
      cell: ({ row }) => {
        const log = checkinLogRowSchema.parse(row.original)
        return <CheckinRowActions row={log} actions={actions} />
      },
      meta: { pinned: 'right' },
    },
  ]
}
