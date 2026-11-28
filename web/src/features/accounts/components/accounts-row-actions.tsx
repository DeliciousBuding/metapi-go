import {
  CalendarCheck,
  Eye,
  MoreHorizontal,
  Pencil,
  Pin,
  PinOff,
  Power,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { Account, AccountRowActions } from '../types'

/**
 * Desktop inline enable/disable button + the existing "more actions" dropdown.
 *
 * The inline `Power` button surfaces the highest-frequency row action (status
 * toggle) as a single click, BEFORE the `MoreHorizontal` dropdown trigger. The
 * pending state is per-row: when `pendingStatusId === account.id`, this row's
 * button shows a `Spinner` and is disabled, while every other row's
 * button stays clickable (no global lock). The pin / check-in dropdown items
 * mirror the same pattern with their OWN pending ids (`pendingPinId` /
 * `pendingCheckinId`): while one of those toggles is in flight for this row,
 * the matching item is disabled and shows a `Spinner` — no cross-talk between
 * the three toggles, no global lock.
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
  pendingCheckinId = null,
  pendingPinId = null,
  pendingManualCheckinId = null,
}: {
  account: Account
  actions: AccountRowActions
  pendingStatusId?: number | null
  pendingCheckinId?: number | null
  pendingPinId?: number | null
  pendingManualCheckinId?: number | null
}) {
  const { t } = useTranslation()
  const canCheckin = account.capabilities?.canCheckin ?? false
  const isThisRowPending = pendingStatusId === account.id
  const isPinPending = pendingPinId === account.id
  const isCheckinPending = pendingCheckinId === account.id
  const isManualCheckinPending = pendingManualCheckinId === account.id
  const toggleLabel =
    account.status === 'disabled'
      ? t('accounts.columns.enable')
      : t('accounts.columns.disable')
  let pinIcon = <Pin />
  if (isPinPending) {
    pinIcon = <Spinner />
  } else if (account.isPinned) {
    pinIcon = <PinOff />
  }
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
                className='max-[640px]:hidden'
                data-hit-area
                onClick={() => actions.onToggleStatus(account)}
              />
            }
          >
            {isThisRowPending ? <Spinner /> : <Power className='size-4' />}
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
          <DropdownMenuItem
            disabled={isPinPending}
            onClick={() => actions.onTogglePin(account)}
          >
            {pinIcon}
            {account.isPinned
              ? t('accounts.columns.unpin')
              : t('accounts.columns.pin')}
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isThisRowPending}
            onClick={() => actions.onToggleStatus(account)}
          >
            <Power />
            {account.status === 'disabled'
              ? t('accounts.columns.enable')
              : t('accounts.columns.disable')}
          </DropdownMenuItem>
          {canCheckin && (
            <DropdownMenuItem
              disabled={pendingManualCheckinId !== null}
              onClick={() => actions.onTriggerCheckin(account)}
            >
              {isManualCheckinPending ? <Spinner /> : <CalendarCheck />}
              {t('accounts.columns.triggerCheckin')}
            </DropdownMenuItem>
          )}
          {canCheckin && (
            <DropdownMenuItem
              disabled={isCheckinPending}
              onClick={() => actions.onToggleCheckin(account)}
            >
              {isCheckinPending ? <Spinner /> : <CalendarCheck />}
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
