/* eslint-disable react/only-export-components -- column definitions co-located with cell renderers */
// metapi-go features/accounts/components — TanStack Table column definitions
// for the accounts list. Column meta drives the mobile card layout
// (mobileTitle / mobileBadge / mobileHidden / mobileOrder) so the data-table
// package's automatic mobile degradation needs zero per-feature card code.

import type { ColumnDef } from '@tanstack/react-table'
import {
  AlertTriangle,
  CalendarCheck,
  CheckCircle2,
  Clock,
  Eye,
  HelpCircle,
  Loader2,
  MoreHorizontal,
  PauseCircle,
  Pencil,
  Pin,
  PinOff,
  Power,
  RefreshCw,
  Trash2,
  XCircle,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
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
  labelKey: string
  variant: 'default' | 'secondary' | 'destructive' | 'warning' | 'outline'
  dotClassName: string
  icon: LucideIcon
}

const HEALTH_BADGE_CONFIG: Record<RuntimeHealthState, HealthBadgeConfig> = {
  healthy: {
    labelKey: 'accounts.columns.healthHealthy',
    variant: 'default',
    dotClassName: 'bg-success',
    icon: CheckCircle2,
  },
  degraded: {
    labelKey: 'accounts.columns.healthDegraded',
    variant: 'warning',
    dotClassName: 'bg-warning',
    icon: AlertTriangle,
  },
  unhealthy: {
    labelKey: 'accounts.columns.healthUnhealthy',
    variant: 'destructive',
    dotClassName: 'bg-destructive',
    icon: XCircle,
  },
  disabled: {
    labelKey: 'accounts.columns.healthDisabled',
    variant: 'secondary',
    dotClassName: 'bg-muted-foreground',
    icon: PauseCircle,
  },
  unknown: {
    labelKey: 'accounts.columns.healthUnknown',
    variant: 'outline',
    dotClassName: 'bg-muted-foreground',
    icon: HelpCircle,
  },
}

function useResolveHealth() {
  const { t } = useTranslation()
  return function resolveHealth(
    account: Account
  ): HealthBadgeConfig & { label: string } {
    if (account.status === 'expired') {
      return {
        labelKey: 'accounts.columns.healthExpired',
        label: t('accounts.columns.healthExpired'),
        variant: 'destructive',
        dotClassName: 'bg-destructive',
        icon: Clock,
      }
    }
    const state = account.runtimeHealth?.state ?? 'unknown'
    const config = HEALTH_BADGE_CONFIG[state] ?? HEALTH_BADGE_CONFIG.unknown
    return { ...config, label: t(config.labelKey) }
  }
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

function useResolveDisplayName() {
  const { t } = useTranslation()
  return function resolveDisplayName(account: Account): string {
    if (account.username && account.username.trim()) return account.username
    return account.credentialMode === 'apikey'
      ? t('accounts.columns.fallbackApiKey')
      : t('accounts.columns.fallbackUnnamed')
  }
}

// ---------------------------------------------------------------------------
// Row actions cell
// ---------------------------------------------------------------------------

/**
 * Inline enable/disable button + the existing "more actions" dropdown.
 *
 * The inline `Power` button surfaces the highest-frequency row action (status
 * toggle) as a single click, BEFORE the `MoreHorizontal` dropdown trigger. The
 * pending state is per-row: when `pendingStatusId === account.id`, this row's
 * button shows a `Loader2` spinner and is disabled, while every other row's
 * button stays clickable (no global lock). The dropdown menu items are
 * unchanged — refresh/pin/checkin/edit/delete all stay where they were.
 */
export function AccountsRowActions({
  account,
  actions,
  pendingStatusId = null,
}: {
  account: Account
  actions: AccountRowActions
  pendingStatusId?: number | null
}) {
  const { t } = useTranslation()
  const canCheckin = account.capabilities?.canCheckin ?? false
  const isThisRowPending = pendingStatusId === account.id
  const toggleLabel =
    account.status === 'disabled'
      ? t('accounts.columns.enable')
      : t('accounts.columns.disable')
  return (
    <div className='flex items-center gap-1'>
      <TooltipProvider delay={200}>
        <Tooltip>
          <TooltipTrigger
            render={
              <button
                type='button'
                disabled={isThisRowPending}
                aria-label={toggleLabel}
                onClick={() => actions.onToggleStatus(account)}
                className='text-muted-foreground hover:bg-muted hover:text-foreground inline-flex size-8 items-center justify-center rounded-md transition-colors outline-none disabled:opacity-50'
              >
                {isThisRowPending ? (
                  <Loader2 className='size-4 animate-spin' />
                ) : (
                  <Power className='size-4' />
                )}
              </button>
            }
          />
          <TooltipContent side='top'>{toggleLabel}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <DropdownMenu>
        <DropdownMenuTrigger
          className='text-muted-foreground hover:bg-muted hover:text-foreground inline-flex size-8 items-center justify-center rounded-md transition-colors outline-none'
          aria-label={t('accounts.columns.rowActions')}
        >
          <MoreHorizontal className='size-4' />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' sideOffset={4}>
          <DropdownMenuItem onClick={() => actions.onViewDetail(account)}>
            <Eye />
            {t('accounts.columns.viewDetails')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => actions.onRefresh(account)}>
            <RefreshCw />
            {t('accounts.columns.refreshBalance')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => actions.onTogglePin(account)}>
            {account.isPinned ? <PinOff /> : <Pin />}
            {account.isPinned
              ? t('accounts.columns.unpin')
              : t('accounts.columns.pin')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => actions.onToggleStatus(account)}>
            <Power />
            {account.status === 'disabled'
              ? t('accounts.columns.enable')
              : t('accounts.columns.disable')}
          </DropdownMenuItem>
          {canCheckin && (
            <DropdownMenuItem onClick={() => actions.onToggleCheckin(account)}>
              <CalendarCheck />
              {account.checkinEnabled
                ? t('accounts.columns.disableCheckin')
                : t('accounts.columns.enableCheckin')}
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => actions.onEdit(account)}>
            <Pencil />
            {t('common.edit')}
          </DropdownMenuItem>
          <DropdownMenuItem
            variant='destructive'
            onClick={() => actions.onDelete(account)}
          >
            <Trash2 />
            {t('common.delete')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Columns hook
// ---------------------------------------------------------------------------

export function useAccountsColumns(
  actions: AccountRowActions,
  pendingStatusId: number | null = null
): ColumnDef<Account>[] {
  const { t } = useTranslation()
  const resolveHealth = useResolveHealth()
  const resolveDisplayName = useResolveDisplayName()
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
          aria-label={t('accounts.columns.selectAll')}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(Boolean(value))}
          aria-label={t('accounts.columns.selectRow')}
        />
      ),
      meta: { mobileHidden: true },
    },
    {
      id: 'name',
      accessorFn: (row) => {
        const account = accountSchema.parse(row)
        return account.username ?? ''
      },
      header: t('accounts.columns.name'),
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        return (
          <div className='flex flex-col gap-1'>
            <div className='flex items-center gap-2'>
              <span className='max-w-[220px] truncate font-medium'>
                {resolveDisplayName(account)}
              </span>
              <Badge
                variant={
                  account.credentialMode === 'session' ? 'default' : 'secondary'
                }
              >
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
      accessorFn: (row) => String(accountSchema.parse(row).siteId),
      header: t('accounts.columns.site'),
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        const site = account.site
        if (!site) return <span className='text-muted-foreground'>—</span>
        return (
          <div className='flex flex-col'>
            <span className='max-w-[160px] truncate'>
              {site.name || site.url || `#${site.id}`}
            </span>
            {site.platform && (
              <span className='text-muted-foreground text-[11px]'>
                {site.platform}
              </span>
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
      accessorFn: (row) => accountSchema.parse(row).status,
      header: t('common.status'),
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        const config = resolveHealth(account)
        const HealthIcon = config.icon
        const reason = account.runtimeHealth?.reason?.trim()
        const badge = (
          <Badge variant={config.variant}>
            <HealthIcon className='size-3' aria-hidden='true' />
            <span
              className={cn('size-1.5 rounded-full', config.dotClassName)}
              aria-hidden='true'
            />
            {config.label}
          </Badge>
        )
        if (!reason) {
          return badge
        }
        return (
          <TooltipProvider delay={200}>
            <Tooltip>
              <TooltipTrigger render={<span className='w-fit'>{badge}</span>} />
              <TooltipContent side='top' className='max-w-xs'>
                <div className='flex flex-col gap-0.5'>
                  <span className='font-medium'>
                    {t('accounts.columns.healthDetail')}
                  </span>
                  <span className='text-xs'>{reason}</span>
                </div>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
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
      header: t('accounts.columns.balance'),
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        return (
          <div className='flex flex-col'>
            <span className='tabular-nums'>
              {formatBalance(account.balance)}
            </span>
            {account.todayReward ? (
              <span className='text-success text-[11px]'>
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
      header: t('accounts.columns.used'),
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        return (
          <div className='flex flex-col'>
            <span className='tabular-nums'>
              {formatBalance(account.balanceUsed)}
            </span>
            <span className='text-muted-foreground text-[11px]'>
              {formatPercent(account.balanceUsed ?? 0, account.quota ?? 0)}
            </span>
          </div>
        )
      },
      meta: { mobileHidden: true },
    },
    {
      id: 'checkin',
      accessorFn: (row) => {
        const account = accountSchema.parse(row)
        return account.checkinEnabled ? 'on' : 'off'
      },
      header: t('accounts.columns.checkin'),
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        if (!account.capabilities?.canCheckin) {
          return (
            <span className='text-muted-foreground text-xs'>
              {t('accounts.columns.checkinUnsupported')}
            </span>
          )
        }
        return (
          <Badge variant={account.checkinEnabled ? 'default' : 'outline'}>
            {account.checkinEnabled
              ? t('accounts.columns.checkinOn')
              : t('accounts.columns.checkinOff')}
          </Badge>
        )
      },
      meta: { mobileOrder: 3 },
    },
    {
      id: 'actions',
      size: 80,
      enableSorting: false,
      enableHiding: false,
      header: () => <span className='sr-only'>{t('common.actions')}</span>,
      cell: ({ row }) => {
        const account = accountSchema.parse(row.original)
        return (
          <AccountsRowActions
            account={account}
            actions={actions}
            pendingStatusId={pendingStatusId}
          />
        )
      },
      meta: { pinned: 'right' },
    },
  ]
}
