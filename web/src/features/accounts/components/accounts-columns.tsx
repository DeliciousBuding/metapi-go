/* eslint-disable react/only-export-components -- column definitions co-located with cell renderers */
// metapi-go features/accounts/components — TanStack Table column definitions
// for the accounts list. Column meta drives the mobile card layout
// (mobileTitle / mobileBadge / mobileHidden / mobileOrder) so the data-table
// package's automatic mobile degradation needs zero per-feature card code.

import type { ColumnDef } from '@tanstack/react-table'
import {
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
  TriangleAlert,
  XCircle,
  type LucideIcon,
} from 'lucide-react'
import { memo, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/utils'

import { resolveAccountDisplayName } from '../lib/accounts-display-name'
import type { Account, AccountRowActions, RuntimeHealthState } from '../types'

// ---------------------------------------------------------------------------
// Health badge mapping
// ---------------------------------------------------------------------------

interface HealthBadgeConfig {
  labelKey: string
  variant: 'success' | 'secondary' | 'destructive' | 'warning' | 'outline'
  dotClassName: string
  icon: LucideIcon
}

const HEALTH_BADGE_CONFIG: Record<RuntimeHealthState, HealthBadgeConfig> = {
  healthy: {
    labelKey: 'accounts.columns.healthHealthy',
    variant: 'success',
    dotClassName: 'bg-success',
    icon: CheckCircle2,
  },
  degraded: {
    labelKey: 'accounts.columns.healthDegraded',
    variant: 'warning',
    dotClassName: 'bg-warning',
    icon: TriangleAlert,
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
  // Stable identity: the columns array is memoized below and must not
  // re-build just because this page re-rendered, otherwise every row's
  // memoization is defeated on each unrelated state change.
  return useCallback(
    function resolveHealth(
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
    },
    [t]
  )
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

function formatPercent(used: number, total: number): string {
  if (!total || total <= 0) return '—'
  const ratio = Math.min(1, Math.max(0, used / total))
  return `${(ratio * 100).toFixed(1)}%`
}

function useResolveDisplayName() {
  const { t } = useTranslation()
  // Stable identity — see useResolveHealth.
  return useCallback(
    function resolveDisplayName(account: Account): string {
      return resolveAccountDisplayName(
        account,
        t('accounts.columns.fallbackApiKey'),
        t('accounts.columns.fallbackUnnamed')
      )
    },
    [t]
  )
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
 *
 * Memoized: the row re-renders on selection/pending-state changes while
 * `account` / `actions` keep the same identity, and this cell (Tooltip +
 * DropdownMenu) is the heaviest subtree per row — skipping it keeps bulk
 * select and status toggles from re-rendering every row's popover machinery.
 */
export const AccountsRowActions = memo(function AccountsRowActions({
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
              <Button
                variant='ghost'
                size='icon-sm'
                disabled={isThisRowPending}
                aria-label={toggleLabel}
                data-hit-area
                onClick={() => actions.onToggleStatus(account)}
              />
            }
          >
            {isThisRowPending ? (
              <Loader2 className='size-4 animate-spin' />
            ) : (
              <Power className='size-4' />
            )}
          </TooltipTrigger>
          <TooltipContent side='top'>{toggleLabel}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              className='data-popup-open:bg-accent'
              aria-label={t('accounts.columns.rowActions')}
              data-hit-area
            />
          }
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
})

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
  // Memoized: the shared data-table row component skips re-rendering a row
  // only when the column defs keep the same identity (table.options.columns),
  // so a fresh array every render re-renders all 100+ rows on any unrelated
  // state change — the previous ~2s freeze on interaction.
  return useMemo<ColumnDef<Account>[]>(
    () => [
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
        accessorFn: (row) => row.username ?? '',
        header: t('accounts.columns.name'),
        cell: ({ row }) => {
          const account = row.original
          return (
            <div className='flex flex-col gap-1'>
              <div className='flex items-center gap-2'>
                <span className='max-w-[220px] truncate font-medium'>
                  {resolveDisplayName(account)}
                </span>
                <Badge
                  variant={
                    account.credentialMode === 'session'
                      ? 'default'
                      : 'secondary'
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
        accessorFn: (row) => String(row.siteId),
        header: t('accounts.columns.site'),
        cell: ({ row }) => {
          const account = row.original
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
          if (!Array.isArray(filterValue) || filterValue.length === 0) {
            return true
          }
          return filterValue.includes(String(row.original.siteId))
        },
        meta: { mobileOrder: 1 },
      },
      {
        id: 'status',
        accessorFn: (row) => row.status,
        header: t('common.status'),
        cell: ({ row }) => {
          const account = row.original
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
                <TooltipTrigger
                  render={<span className='w-fit'>{badge}</span>}
                />
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
          if (!Array.isArray(filterValue) || filterValue.length === 0) {
            return true
          }
          return filterValue.includes(row.original.status)
        },
        meta: { mobileBadge: true },
      },
      {
        accessorKey: 'balance',
        header: t('accounts.columns.balance'),
        cell: ({ row }) => {
          const account = row.original
          return (
            <div className='flex flex-col'>
              <span className='tabular-nums'>
                {formatCurrency(account.balance)}
              </span>
              {account.todayReward ? (
                <span className='text-success text-[11px]'>
                  +{formatCurrency(account.todayReward)}
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
          const account = row.original
          return (
            <div className='flex flex-col'>
              <span className='tabular-nums'>
                {formatCurrency(account.balanceUsed)}
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
        accessorFn: (row) => (row.checkinEnabled ? 'on' : 'off'),
        header: t('accounts.columns.checkin'),
        cell: ({ row }) => {
          const account = row.original
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
          const account = row.original
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
    ],
    [actions, pendingStatusId, resolveHealth, resolveDisplayName, t]
  )
}
