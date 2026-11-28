import {
  Eye,
  MoreHorizontal,
  Pencil,
  Power,
  Snowflake,
  Trash2,
} from 'lucide-react'
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

import type { RouteRowActions, RouteSummaryRow } from '../types'

export function RoutesRowActions({
  route,
  readOnly,
  actions,
  pendingToggleId = null,
  pendingCooldownId = null,
  cooldownRouteIds,
}: {
  route: RouteSummaryRow
  readOnly: boolean
  actions: RouteRowActions
  pendingToggleId?: number | null
  pendingCooldownId?: number | null
  /** Route ids with at least one channel in cooldown/breaker state (derived
   * from the channels-list query by the page). `undefined` means still
   * loading — the item renders disabled until the set resolves. */
  cooldownRouteIds?: ReadonlySet<number>
}) {
  const { t } = useTranslation()
  const isTogglePending = pendingToggleId === route.id
  const isCooldownPending = pendingCooldownId === route.id
  // No active cooldown → hide the item entirely. Clicking it with nothing
  // to clear only produced a false "已清除冷却" success toast (the detail
  // sheet gates the same action on `hasActiveCooldown`).
  const showClearCooldown =
    cooldownRouteIds === undefined || cooldownRouteIds.has(route.id)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon-sm'
            className='data-popup-open:bg-accent'
            aria-label={t('tokenRoutes.columns.rowActions')}
            data-hit-area
          />
        }
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' sideOffset={4}>
        <DropdownMenuItem onClick={() => actions.onViewDetail(route)}>
          <Eye />
          {t('tokenRoutes.columns.viewDetails')}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => actions.onToggleEnabled(route)}
          disabled={readOnly || isTogglePending}
        >
          {isTogglePending ? <Spinner /> : <Power />}
          {route.enabled
            ? t('tokenRoutes.columns.disable')
            : t('tokenRoutes.columns.enable')}
        </DropdownMenuItem>
        {showClearCooldown && (
          <DropdownMenuItem
            onClick={() => actions.onClearCooldown(route)}
            disabled={readOnly || isCooldownPending}
          >
            {isCooldownPending ? <Spinner /> : <Snowflake />}
            {t('tokenRoutes.columns.clearCooldown')}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => actions.onEdit(route)}
          disabled={readOnly}
        >
          <Pencil />
          {t('common.edit')}
        </DropdownMenuItem>
        <DropdownMenuItem
          variant='destructive'
          onClick={() => actions.onDelete(route)}
          disabled={readOnly || actions.isDeletePending}
        >
          <Trash2 />
          {t('common.delete')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
