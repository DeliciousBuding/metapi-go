// metapi-go features/accounts/components — TanStack Table column definitions
// for the accounts list. Column meta drives the mobile card layout
// (mobileTitle / mobileBadge / mobileHidden / mobileOrder) so the data-table
// package's automatic mobile degradation needs zero per-feature card code.

import type { ColumnDef } from '@tanstack/react-table'
import {
  Eye,
  MoreHorizontal,
  Pencil,
  Pin,
  PinOff,
  Power,
  RefreshCw,
  CalendarCheck,
  Trash2,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import {
  type Account,
  type AccountRowActions,
  type RuntimeHealthState,
  accountSchema,
} from '../types'

// ---------------------------------------------------------------------------
// Health badge mapping
// ---------------------------------------------------------------------------

interface HealthBadgeConfig {
  label: string
  variant: 'default' | 'secondary' | 'destructive' | 'warning' | 'outline'
  dotClassName: string
}

const HEALTH_BADGE_CONFIG: Record<RuntimeHealthState, HealthBadgeConfig> = {
  healthy: { label: '健康', variant: 'default', dotClassName: 'bg-emerald-500' },
  degraded: { label: '降级', variant: 'warning', dotClassName: 'bg-amber-500' },
  unhealthy: { label: '异常', variant: 'destructive', dotClassName: 'bg-red-500' },
  disabled: { label: '已禁用', variant: 'secondary', dotClassName: 'bg-muted-foreground' },
  unknown: { label: '未知', variant: 'outline', dotClassName: 'bg-muted-foreground' },
}

function resolveHealth(account: Account): HealthBadgeConfig {
  if (account.status === 'expired') {
    return {
      label: '已过期',
      variant: 'destructive',
      dotClassName: 'bg-red-500',
    }
  }
  const state = account.runtimeHealth?.state ?? 'unknown'
  return HEALTH_BADGE_CONFIG[state] ?? HEALTH_BADGE_CONFIG.unknown
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

function formatBalance(value: number | undefined | null): string {
  if (value === undefined || value === null) return '—'
  return `$${value.toFixed(2)}`
}

function formatPercent(used: number, total: number): string {
  if (!total || total <= 0) return '—'
  const ratio = Math.min(1, Math.max(0, used / total))
  return `${(ratio * 100).toFixed(1)}%`
}

function resolveDisplayName(account: Account): string {
  if (account.username && account.username.trim()) return account.username
  return account.credentialMode === 'apikey' ? 'API Key 连接' : '未命名连接'
}

// ---------------------------------------------------------------------------
// Row actions cell
// ---------------------------------------------------------------------------

function AccountsRowActions({
  account,
  actions,
}: {
  account: Account
  actions: AccountRowActions
}) {
  const canCheckin = account.capabilities?.canCheckin ?? false
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className='inline-flex size-8 items-center justify-center rounded-md text-muted-foreground outline-none transition-colors hover:bg-muted hover:text-foreground'
        aria-label='账号操作'
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' sideOffset={4}>
        <DropdownMenuItem onClick={() => actions.onViewDetail(account)}>
          <Eye />
          查看详情
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => actions.onRefresh(account)}>
          <RefreshCw />
          刷新余额
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => actions.onTogglePin(account)}>
          {account.isPinned ? <PinOff /> : <Pin />}
          {account.isPinned ? '取消置顶' : '置顶'}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => actions.onToggleStatus(account)}
        >
          <Power />
          {account.status === 'disabled' ? '启用' : '禁用'}
        </DropdownMenuItem>
        {canCheckin && (
          <DropdownMenuItem onClick={() => actions.onToggleCheckin(account)}>
            <CalendarCheck />
            {account.checkinEnabled ? '关闭签到' : '开启签到'}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => actions.onEdit(account)}>
          <Pencil />
          编辑
        </DropdownMenuItem>
        <DropdownMenuItem
          variant='destructive'
          onClick={() => actions.onDelete(account)}
        >
          <Trash2 />
          删除
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// ---------------------------------------------------------------------------
// Columns hook
// ---------------------------------------------------------------------------

export function useAccountsColumns(
  actions: AccountRowActions,
): ColumnDef<Account>[] {
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
      id: 'name',
      header: '连接名称',
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        return (
          <div className='flex flex-col gap-1'>
            <div className='flex items-center gap-2'>
              <span className='font-medium truncate max-w-[220px]'>
                {resolveDisplayName(account)}
              </span>
              <Badge variant={account.credentialMode === 'session' ? 'default' : 'secondary'}>
                {account.credentialMode === 'session' ? 'Session' : 'API Key'}
              </Badge>
            </div>
            {account.tags && account.tags.length > 0 && (
              <div className='flex flex-wrap gap-1'>
                {account.tags.slice(0, 3).map((tag) => (
                  <Badge key={tag} variant='outline' className='text-[10px]'>
                    {tag}
                  </Badge>
                ))}
              </div>
            )}
          </div>
        )
      },
      meta: { mobileTitle: true },
    },
    {
      id: 'site',
      header: '站点',
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        const site = account.site
        if (!site) return <span className='text-muted-foreground'>—</span>
        return (
          <div className='flex flex-col'>
            <span className='truncate max-w-[160px]'>{site.name || site.url || `#${site.id}`}</span>
            {site.platform && (
              <span className='text-[11px] text-muted-foreground'>{site.platform}</span>
            )}
          </div>
        )
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) return true
        const account = accountSchema.parse(row.original)
        return filterValue.includes(String(account.siteId))
      },
      meta: { mobileOrder: 1 },
    },
    {
      id: 'status',
      header: '状态',
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        const config = resolveHealth(account)
        return (
          <Badge variant={config.variant}>
            <span className={cn('size-1.5 rounded-full', config.dotClassName)} />
            {config.label}
          </Badge>
        )
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) return true
        const account = accountSchema.parse(row.original)
        return filterValue.includes(account.status)
      },
      meta: { mobileBadge: true },
    },
    {
      accessorKey: 'balance',
      header: '余额',
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        return (
          <div className='flex flex-col'>
            <span className='tabular-nums'>{formatBalance(account.balance)}</span>
            {account.todayReward ? (
              <span className='text-[11px] text-emerald-600'>
                +{formatBalance(account.todayReward)}
              </span>
            ) : null}
          </div>
        )
      },
      meta: { mobileOrder: 2 },
    },
    {
      accessorKey: 'balanceUsed',
      header: '已用',
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        return (
          <div className='flex flex-col'>
            <span className='tabular-nums'>{formatBalance(account.balanceUsed)}</span>
            <span className='text-[11px] text-muted-foreground'>
              {formatPercent(account.balanceUsed ?? 0, account.balance ?? 0)}
            </span>
          </div>
        )
      },
      meta: { mobileHidden: true },
    },
    {
      id: 'checkin',
      header: '签到',
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        if (!account.capabilities?.canCheckin) {
          return <span className='text-muted-foreground text-xs'>不支持</span>
        }
        return (
          <Badge variant={account.checkinEnabled ? 'default' : 'outline'}>
            {account.checkinEnabled ? '已开启' : '未开启'}
          </Badge>
        )
      },
      meta: { mobileOrder: 3 },
    },
    {
      id: 'actions',
      size: 48,
      enableSorting: false,
      enableHiding: false,
      header: () => <span className='sr-only'>操作</span>,
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        return <AccountsRowActions account={account} actions={actions} />
      },
      meta: { pinned: 'right' },
    },
  ]
}
