// metapi-go features/accounts/components — TanStack Table column definitions
// for the accounts list. Column meta drives the mobile card layout
// (mobileTitle / mobileBadge / mobileHidden / mobileOrder) so the data-table
// package's automatic mobile degradation needs zero per-feature card code.

import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { ProbeHistoryMap } from '@/components/common/probe-health-bar'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { Account, AccountRowActions } from '../types'
import { accountHealthColumns } from './accounts-health-columns'
import {
  accountIdentityColumns,
  type AccountsRowDisclosureOptions,
} from './accounts-identity-columns'
import { AccountsRowActions } from './accounts-row-actions'

export { AccountsRowActions }
export type { AccountsRowDisclosureOptions }

function formatPercent(used: number, total: number): string {
  if (!total || total <= 0) return '—'
  const ratio = Math.min(1, Math.max(0, used / total))
  return `${(ratio * 100).toFixed(1)}%`
}
export function useAccountsColumns(
  actions: AccountRowActions,
  pendingStatusId: number | null = null,
  pendingCheckinId: number | null = null,
  pendingPinId: number | null = null,
  pendingManualCheckinId: number | null = null,
  probeHistory?: ProbeHistoryMap,
  disclosure?: AccountsRowDisclosureOptions
): ColumnDef<Account>[] {
  const { t } = useTranslation()
  // Memoized: the shared data-table row component skips re-rendering a row
  // only when the column defs keep the same identity (table.options.columns),
  // so a fresh array every render re-renders all 100+ rows on any unrelated
  // state change — the previous ~2s freeze on interaction.
  return useMemo<ColumnDef<Account>[]>(
    () => [
      ...accountIdentityColumns(t, actions, disclosure),
      ...accountHealthColumns(t, probeHistory),
      {
        accessorKey: 'balance',
        header: t('accounts.columns.balance'),
        cell: ({ row }) => {
          const account = row.original
          const hasReward =
            typeof account.todayReward === 'number' && account.todayReward > 0
          const hasSpend =
            typeof account.todaySpend === 'number' && account.todaySpend > 0
          return (
            <div className='flex flex-col gap-0.5'>
              <span className='font-medium tabular-nums'>
                {formatCurrency(account.balance)}
              </span>
              {(hasReward || hasSpend) && (
                <div className='text-2xs flex flex-wrap items-center gap-1.5 tabular-nums'>
                  {hasReward && (
                    <span className='text-success-soft-fg'>
                      +{formatCurrency(account.todayReward)}
                    </span>
                  )}
                  {hasSpend && (
                    <span className='text-muted-foreground'>
                      -{formatCurrency(account.todaySpend)}
                    </span>
                  )}
                </div>
              )}
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
              {account.balanceUsed != null &&
                account.quota != null &&
                account.quota > 0 && (
                  <span className='text-muted-foreground text-2xs'>
                    {formatPercent(
                      account.balanceUsed ?? 0,
                      account.quota ?? 0
                    )}
                  </span>
                )}
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
          // The badge reads as clickable, so it must BE clickable (like the
          // original TS Accounts page): a button that keeps the badge look
          // (pill, same default/outline colors) and toggles check-in inline.
          // size='xs' is h-6 = 24px, the WCAG 2.5.8 minimum hit target.
          const isPending = pendingCheckinId === account.id
          const label = account.checkinEnabled
            ? t('accounts.columns.checkinOn')
            : t('accounts.columns.checkinOff')
          const actionLabel = account.checkinEnabled
            ? t('accounts.columns.disableCheckin')
            : t('accounts.columns.enableCheckin')
          return (
            <TooltipProvider delay={200}>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant={account.checkinEnabled ? 'ghost' : 'outline'}
                      size='xs'
                      className={cn(
                        'rounded-4xl px-2.5 font-medium',
                        account.checkinEnabled &&
                          'bg-success/10 text-success hover:bg-success/20 hover:text-success'
                      )}
                      aria-label={actionLabel}
                      data-hit-area
                      disabled={isPending}
                      onClick={() => actions.onToggleCheckin(account)}
                    >
                      {isPending ? <Spinner /> : label}
                    </Button>
                  }
                >
                  {account.checkinEnabled
                    ? t('accounts.columns.checkinTooltipOn')
                    : t('accounts.columns.checkinTooltipOff')}
                </TooltipTrigger>
                <TooltipContent side='top'>
                  {account.checkinEnabled
                    ? t('accounts.columns.checkinTooltipOn')
                    : t('accounts.columns.checkinTooltipOff')}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
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
              pendingCheckinId={pendingCheckinId}
              pendingPinId={pendingPinId}
              pendingManualCheckinId={pendingManualCheckinId}
            />
          )
        },
        meta: { pinned: 'right' },
      },
    ],
    [
      actions,
      pendingStatusId,
      pendingCheckinId,
      pendingPinId,
      pendingManualCheckinId,
      t,
      probeHistory,
      disclosure,
    ]
  )
}
