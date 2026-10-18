/* eslint-disable no-nested-ternary -- status label selection uses chained ternary */
// metapi-go features/token-routes/components — route detail side sheet.
// i18n: all user-visible strings migrated to t() calls.
// `routingStrategyLabel()` returns an i18n key; wrapped with `t()`.

import { Pencil, RefreshCw, Snowflake } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DetailField } from '@/components/common/detail-field'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'

import {
  useClearRouteCooldown,
  useRebuildRoutes,
  useRouteChannels,
} from '../api'
import type { RouteSummaryRow } from '../types'
import {
  formatContextLength,
  isExplicitGroupRoute,
  resolveRouteTitle,
  routingStrategyLabel,
} from '../utils'
import { RouteChannelList } from './route-channel-list'
import { RouteDecisionPanel } from './route-decision-panel'

interface RouteDetailSheetProps {
  route: RouteSummaryRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onEdit?: (route: RouteSummaryRow) => void
}

export function RouteDetailSheet({
  route,
  open,
  onOpenChange,
  onEdit,
}: RouteDetailSheetProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const channelsQuery = useRouteChannels(route?.id ?? null)
  const clearCooldownMutation = useClearRouteCooldown()
  const rebuildMutation = useRebuildRoutes()
  // Rebuild recomposes the route's channels from current model availability —
  // the same impact as the page-level rebuild, which already confirms. The
  // sheet button got no gate, so mirror the confirmation here.
  const [rebuildConfirmOpen, setRebuildConfirmOpen] = useState(false)
  const channels = useMemo(() => channelsQuery.data ?? [], [channelsQuery.data])
  if (!route) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' className='sm:max-w-md' />
      </Sheet>
    )
  }

  const isReadOnly = route.kind === 'zero_channel' || route.readOnly === true
  const title = resolveRouteTitle(route)
  const decision = route.decisionSnapshot ?? null
  // A clear-cooldown action with nothing to clear is a false "已清除冷却"
  // toast. The backend endpoint clears the WHOLE route's channels, so the
  // button only appears while at least one channel is actually cooling
  // (mirrors the channel detail sheet's status gate).
  const hasActiveCooldown = channels.some(
    (channel) =>
      Boolean(channel.cooldownUntil) &&
      new Date(channel.cooldownUntil as string) > new Date()
  )

  const handleClearCooldown = async () => {
    if (!route) return
    try {
      await clearCooldownMutation.mutateAsync(route.id)
    } catch {
      // Failure is already toasted by the http-client response interceptor;
      // nothing to add here.
    }
  }

  const handleRebuild = async () => {
    try {
      await rebuildMutation.mutateAsync({ refreshModels: true })
    } catch {
      // Failure is already toasted by the http-client response interceptor;
      // nothing to add here.
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-xl'
      >
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2'>
            <span className='truncate'>{title}</span>
            {isExplicitGroupRoute(route) ? (
              <Badge variant='secondary'>
                {t('tokenRoutes.detail.badgeGroup')}
              </Badge>
            ) : (
              <Badge variant='outline'>
                {t('tokenRoutes.detail.badgeMatch')}
              </Badge>
            )}
            {isReadOnly && (
              <Badge variant='outline' className='text-muted-foreground'>
                {t('tokenRoutes.detail.notGenerated')}
              </Badge>
            )}
          </SheetTitle>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField label={t('tokenRoutes.detail.matchRule')}>
              <code className='font-mono text-xs'>{route.modelPattern}</code>
            </DetailField>
            <DetailField
              label={t('tokenRoutes.detail.displayName')}
              title={route.displayName || undefined}
            >
              {route.displayName || '—'}
            </DetailField>
            <DetailField label={t('common.status')}>
              {isReadOnly
                ? t('tokenRoutes.columns.notEnabled')
                : route.enabled
                  ? t('tokenRoutes.columns.enable')
                  : t('tokenRoutes.columns.disable')}
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.strategy')}>
              {isReadOnly
                ? '—'
                : t(routingStrategyLabel(route.routingStrategy))}
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.context')}>
              {formatContextLength(route.contextLength) || '—'}
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.channels')}>
              {t('tokenRoutes.detail.channelCount', {
                total: route.channelCount,
                enabled: route.enabledChannelCount,
              })}
            </DetailField>
            <DetailField
              label={t('tokenRoutes.detail.sites')}
              title={
                route.siteNames?.length ? route.siteNames.join(', ') : undefined
              }
            >
              {route.siteNames?.length ? route.siteNames.join(', ') : '—'}
            </DetailField>
            <DetailField
              label={t('tokenRoutes.detail.decisionRefresh')}
              title={formatDateTime(route.decisionRefreshedAt, locale)}
            >
              {formatDateTime(route.decisionRefreshedAt, locale)}
            </DetailField>
          </dl>

          {route.modelMapping && (
            <div className='bg-muted/40 rounded-lg border p-2'>
              <div className='text-muted-foreground text-2xs'>
                {t('tokenRoutes.detail.modelMapping')}
              </div>
              <code className='block font-mono text-xs break-all'>
                {route.modelMapping}
              </code>
            </div>
          )}

          {!isReadOnly && hasActiveCooldown && (
            <div className='flex flex-wrap justify-end gap-2'>
              <Button
                variant='outline'
                size='sm'
                onClick={handleClearCooldown}
                disabled={clearCooldownMutation.isPending}
              >
                <Snowflake />
                {t('tokenRoutes.detail.clearCooldown')}
              </Button>
            </div>
          )}

          <Separator />

          {open && <RouteChannelList route={route} />}

          <Separator />
          <RouteDecisionPanel
            key={route.id}
            route={route}
            snapshot={decision}
          />
        </div>

        <SheetFooter>
          {!isReadOnly && onEdit ? (
            <Button variant='outline' onClick={() => onEdit(route)}>
              <Pencil className='size-4' />
              {t('common.edit')}
            </Button>
          ) : null}
          <Button onClick={() => setRebuildConfirmOpen(true)} variant='default'>
            <RefreshCw
              className={rebuildMutation.isPending ? 'animate-spin' : undefined}
            />
            {isReadOnly
              ? t('tokenRoutes.detail.rebuildRoutes')
              : t('tokenRoutes.detail.rebuild')}
          </Button>
        </SheetFooter>

        <ConfirmDialog
          open={rebuildConfirmOpen}
          title={t('tokenRoutes.page.rebuildConfirmTitle')}
          description={t('tokenRoutes.page.rebuildConfirmDescription')}
          confirmLabel={t('tokenRoutes.page.rebuild')}
          cancelLabel={t('common.cancel')}
          destructive
          onConfirm={() => {
            setRebuildConfirmOpen(false)
            void handleRebuild()
          }}
          onCancel={() => setRebuildConfirmOpen(false)}
        />
      </SheetContent>
    </Sheet>
  )
}
