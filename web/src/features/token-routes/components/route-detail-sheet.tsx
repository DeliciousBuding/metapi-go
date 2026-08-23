/* eslint-disable no-nested-ternary -- status label selection uses chained ternary */
// metapi-go features/token-routes/components — route detail side sheet.
// i18n: all user-visible strings migrated to t() calls.
// `routingStrategyLabel()` returns an i18n key; wrapped with `t()`.

import { useQueries } from '@tanstack/react-query'
import { Pencil, RefreshCw, Snowflake } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

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
import { Spinner } from '@/components/ui/spinner'
import { priceCompareQueryOptions } from '@/features/models/price-compare/api'
import { PriceGradeBadge } from '@/features/models/price-compare/components/price-grade-badge'
import type { PriceCompareItem } from '@/features/models/price-compare/types'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime, formatInt } from '@/lib/format'

import {
  useClearRouteCooldown,
  useRebuildRoutes,
  useRouteChannels,
} from '../api'
import {
  calculateRouteChannelAllocations,
  formatRoutePrice,
  formatRouteWeightShare,
  normalizeModelKey,
  resolveDistinctConcreteModels,
  resolveRouteChannelPriceTruth,
  type RouteChannelAllocation,
  type RouteChannelPriceTruth,
} from '../lib/route-price-truth'
import type { RouteChannel, RouteDecision, RouteSummaryRow } from '../types'
import {
  formatContextLength,
  isExplicitGroupRoute,
  resolveRouteTitle,
  routingStrategyLabel,
} from '../utils'

interface RouteDetailSheetProps {
  route: RouteSummaryRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onEdit?: (route: RouteSummaryRow) => void
}

type PriceQueryState = {
  isLoading: boolean
  hasError: boolean
}

function resolveChannelAllocation(
  allocationsByChannelId: ReadonlyMap<number, RouteChannelAllocation>,
  channel: RouteChannel
): RouteChannelAllocation {
  const allocation = allocationsByChannelId.get(channel.id)
  if (allocation) return allocation
  // A refetch race could momentarily deliver a channel whose allocation
  // hasn't been computed yet. Throwing in the render path would surface to
  // the layout error boundary and blank the whole page, so fall back to a
  // zero-share allocation derived from the channel's own weight instead.
  if (import.meta.env.DEV) {
    console.warn(`Missing allocation for route channel ${channel.id}`)
  }
  return {
    channelId: channel.id,
    configuredWeight: channel.weight,
    enabledWeightShare: null,
  }
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
  const channels = useMemo(() => channelsQuery.data ?? [], [channelsQuery.data])
  const concreteModels = useMemo(
    () => (route ? resolveDistinctConcreteModels(route, channels) : []),
    [route, channels]
  )
  const priceQueries = useQueries({
    queries: concreteModels.map((concreteModel) => ({
      ...priceCompareQueryOptions({
        model: concreteModel,
        limit: 200,
        exactModel: true,
      }),
      enabled: open && route !== null,
    })),
  })

  const priceRowsByModel = new Map<string, readonly PriceCompareItem[]>()
  const priceQueryStateByModel = new Map<string, PriceQueryState>()
  concreteModels.forEach((concreteModel, modelIndex) => {
    const modelKey = normalizeModelKey(concreteModel)
    const priceQuery = priceQueries[modelIndex]
    priceRowsByModel.set(modelKey, priceQuery?.data ?? [])
    priceQueryStateByModel.set(modelKey, {
      isLoading: priceQuery?.isPending ?? false,
      hasError: priceQuery?.isError ?? false,
    })
  })

  const allocationsByChannelId = new Map(
    calculateRouteChannelAllocations(channels).map((allocation) => [
      allocation.channelId,
      allocation,
    ])
  )
  const isPriceFetching = priceQueries.some(
    (priceQuery) => priceQuery.isFetching
  )

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
    } catch {}
  }

  const handleRebuild = async () => {
    try {
      await rebuildMutation.mutateAsync({ refreshModels: true })
    } catch {}
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
              <div className='text-muted-foreground text-[11px]'>
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

          <div className='space-y-2'>
            <div className='flex items-center justify-between'>
              <div>
                <h3 className='text-sm font-medium'>
                  {t('tokenRoutes.detail.channelTruthList')}
                </h3>
                <p className='text-muted-foreground text-[11px]'>
                  {t('tokenRoutes.detail.channelTruthDescription')}
                </p>
              </div>
              {(channelsQuery.isFetching || isPriceFetching) && (
                <Spinner className='text-muted-foreground size-3.5' />
              )}
            </div>
            {channels.length === 0 ? (
              <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-center text-sm'>
                {isReadOnly
                  ? t('tokenRoutes.detail.channelEmptyReadOnly')
                  : t('tokenRoutes.detail.channelEmptyEditable')}
              </p>
            ) : (
              <ul className='space-y-1.5'>
                {channels.map((channel) => {
                  const allocation = resolveChannelAllocation(
                    allocationsByChannelId,
                    channel
                  )
                  const priceTruth = resolveRouteChannelPriceTruth(
                    route,
                    channel,
                    priceRowsByModel
                  )
                  const priceQueryState = priceTruth.concreteModel
                    ? priceQueryStateByModel.get(
                        normalizeModelKey(priceTruth.concreteModel)
                      )
                    : undefined
                  return (
                    <ChannelRow
                      key={channel.id}
                      channel={channel}
                      allocation={allocation}
                      priceTruth={priceTruth}
                      priceQueryState={priceQueryState}
                    />
                  )
                })}
              </ul>
            )}
          </div>

          <Separator />
          <DecisionSnapshotSection decision={decision} />
        </div>

        <SheetFooter>
          {!isReadOnly && onEdit ? (
            <Button variant='outline' onClick={() => onEdit(route)}>
              <Pencil className='size-4' />
              {t('common.edit')}
            </Button>
          ) : null}
          <Button onClick={handleRebuild} variant='default'>
            <RefreshCw
              className={rebuildMutation.isPending ? 'animate-spin' : undefined}
            />
            {isReadOnly
              ? t('tokenRoutes.detail.rebuildRoutes')
              : t('tokenRoutes.detail.rebuild')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function ChannelRow({
  channel,
  allocation,
  priceTruth,
  priceQueryState,
}: {
  channel: RouteChannel
  allocation: RouteChannelAllocation
  priceTruth: RouteChannelPriceTruth
  priceQueryState: PriceQueryState | undefined
}) {
  const { t } = useTranslation()
  const accountLabel =
    channel.account?.username ||
    t('tokenRoutes.detail.fallbackAccount', { id: channel.accountId })
  const siteLabel = channel.site?.name || channel.site?.platform || ''
  const tokenLabel =
    channel.token?.name ||
    (channel.tokenId
      ? t('tokenRoutes.detail.fallbackToken', { id: channel.tokenId })
      : t('tokenRoutes.detail.channelTokenUnbound'))
  const sourceModel = priceTruth.concreteModel || '—'
  const cooldownActive =
    Boolean(channel.cooldownUntil) &&
    new Date(channel.cooldownUntil as string) > new Date()
  return (
    <li className='rounded-lg border p-2 text-xs'>
      <div className='flex items-start justify-between gap-2'>
        <div className='min-w-0 flex-1'>
          <div className='flex items-center gap-1.5'>
            <span className='truncate font-medium'>{accountLabel}</span>
            {siteLabel && (
              <span className='text-muted-foreground truncate'>
                @ {siteLabel}
              </span>
            )}
          </div>
          <div className='text-muted-foreground mt-0.5 flex flex-wrap items-center gap-1.5'>
            <span>
              {t('tokenRoutes.detail.channelToken')}: {tokenLabel}
            </span>
            <span>
              {t('tokenRoutes.detail.channelUpstream')} {sourceModel}
            </span>
            <span>
              {t('tokenRoutes.detail.channelPriority')} {channel.priority}
            </span>
          </div>
        </div>
        <div className='flex flex-col items-end gap-1'>
          <Badge variant={channel.enabled ? 'success' : 'secondary'}>
            {channel.enabled
              ? t('tokenRoutes.columns.enable')
              : t('tokenRoutes.columns.disable')}
          </Badge>
          {cooldownActive && (
            <Badge variant='warning'>{t('tokenRoutes.detail.cooldown')}</Badge>
          )}
        </div>
      </div>

      <dl className='bg-muted/30 mt-2 grid grid-cols-2 gap-2 rounded-md p-2 sm:grid-cols-5'>
        <ChannelMetric label={t('tokenRoutes.detail.channelConfiguredWeight')}>
          {allocation.configuredWeight}
        </ChannelMetric>
        <ChannelMetric label={t('tokenRoutes.detail.channelEnabledShare')}>
          <span className='tabular-nums'>
            {formatRouteWeightShare(allocation.enabledWeightShare)}
          </span>
          {allocation.enabledWeightShare === null && (
            <span className='text-muted-foreground block text-[10px]'>
              {channel.enabled
                ? t('tokenRoutes.detail.channelShareUnavailable')
                : t('tokenRoutes.detail.channelShareExcluded')}
            </span>
          )}
        </ChannelMetric>
        <ChannelMetric label={t('tokenRoutes.detail.channelHits')}>
          <span className='tabular-nums'>
            <span className='text-success'>
              {formatInt(channel.successCount)}
            </span>
            <span className='text-muted-foreground'> / </span>
            <span
              className={
                channel.failCount > 0
                  ? 'text-destructive'
                  : 'text-muted-foreground'
              }
            >
              {formatInt(channel.failCount)}
            </span>
          </span>
        </ChannelMetric>
        <ChannelMetric label={t('tokenRoutes.detail.channelInputPrice')}>
          <PriceValue value={priceTruth.inputPerMillion} />
        </ChannelMetric>
        <ChannelMetric label={t('tokenRoutes.detail.channelOutputPrice')}>
          <PriceValue value={priceTruth.outputPerMillion} />
        </ChannelMetric>
      </dl>

      <div className='mt-2 flex flex-wrap items-center gap-x-3 gap-y-1'>
        <PriceProvenance
          priceTruth={priceTruth}
          priceQueryState={priceQueryState}
        />
      </div>
    </li>
  )
}

function ChannelMetric({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div>
      <dt className='text-muted-foreground text-[10px]'>{label}</dt>
      <dd className='font-medium'>{children}</dd>
    </div>
  )
}

function PriceValue({ value }: { value: number | null }) {
  return (
    <span className='tabular-nums'>
      {value === null ? '—' : `$${formatRoutePrice(value)}`}
    </span>
  )
}

function PriceProvenance({
  priceTruth,
  priceQueryState,
}: {
  priceTruth: RouteChannelPriceTruth
  priceQueryState: PriceQueryState | undefined
}) {
  const { t } = useTranslation()

  if (!priceTruth.concreteModel) {
    return (
      <span className='text-muted-foreground'>
        {t('tokenRoutes.detail.channelPriceProvenance')}: — ·{' '}
        {t('tokenRoutes.detail.channelPatternUnavailable')}
      </span>
    )
  }
  if (priceQueryState?.isLoading) {
    return (
      <span className='text-muted-foreground inline-flex items-center gap-1'>
        <Spinner className='size-3' />
        {t('tokenRoutes.detail.channelPriceLoading')}
      </span>
    )
  }
  if (
    priceQueryState?.hasError ||
    !priceTruth.price ||
    priceTruth.price.missingPrice ||
    !priceTruth.provenance.ratesSource
  ) {
    return (
      <span className='text-muted-foreground'>
        {t('tokenRoutes.detail.channelPriceProvenance')}: — ·{' '}
        {t('tokenRoutes.detail.channelPriceUnavailable')}
      </span>
    )
  }

  const costSource = priceTruth.provenance.source
  const ratesSource = priceTruth.provenance.ratesSource
  return (
    <>
      <span className='text-muted-foreground'>
        {t('tokenRoutes.detail.channelRateProvenance')}:
      </span>
      <PriceGradeBadge grade={ratesSource} />
      {costSource && costSource !== ratesSource && (
        <>
          <span className='text-muted-foreground'>
            {t('tokenRoutes.detail.channelCostProvenance')}:
          </span>
          <PriceGradeBadge grade={costSource} />
        </>
      )}
    </>
  )
}

function DecisionSnapshotSection({
  decision,
}: {
  decision: RouteDecision | null
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  if (!decision) {
    return (
      <div className='space-y-1'>
        <h3 className='text-sm font-medium'>
          {t('tokenRoutes.detail.decisionSnapshot')}
        </h3>
        <p className='text-muted-foreground text-xs'>
          {t('tokenRoutes.detail.decisionEmpty')}
        </p>
      </div>
    )
  }
  const candidates = decision.candidates ?? []
  const generatedAt = formatDateTime(decision.generatedAt, locale)
  const reasonText = decision.reasonText || decision.matchedRoutePattern || ''
  return (
    <div className='space-y-2'>
      <h3 className='text-sm font-medium'>
        {t('tokenRoutes.detail.decisionSnapshot')}
      </h3>
      <dl className='grid grid-cols-2 gap-x-3 gap-y-1 text-xs'>
        <DetailField
          label={t('tokenRoutes.detail.decisionModel')}
          title={decision.model || undefined}
        >
          {decision.model || '—'}
        </DetailField>
        <DetailField
          label={t('tokenRoutes.detail.decisionGeneratedAt')}
          title={generatedAt}
        >
          {generatedAt}
        </DetailField>
        <DetailField label={t('tokenRoutes.detail.decisionCandidateCount')}>
          {candidates.length}
        </DetailField>
        <DetailField label={t('tokenRoutes.detail.decisionSelectedChannel')}>
          {(() => {
            const selected = candidates.find(
              (candidate) => candidate.channelId === decision.selectedChannelId
            )
            if (selected?.username) return selected.username
            if (decision.selectedChannelId != null) {
              return t('tokenRoutes.detail.fallbackAccount', {
                id: decision.selectedChannelId,
              })
            }
            return '—'
          })()}
        </DetailField>
      </dl>
      {reasonText && (
        <div className='bg-muted/40 rounded-lg border p-2 text-xs'>
          <div className='text-muted-foreground text-[11px]'>
            {t('tokenRoutes.detail.decisionReason')}
          </div>
          <p className='break-words'>{reasonText}</p>
        </div>
      )}
      {candidates.length > 0 && (
        <ul className='space-y-1'>
          {candidates.slice(0, 6).map((candidate, index) => (
            <li
              key={candidate.channelId ?? index}
              className='flex items-center justify-between rounded border px-2 py-1 text-xs'
            >
              <span className='truncate'>
                {candidate.username ||
                  t('tokenRoutes.detail.fallbackAccount', {
                    id: candidate.accountId,
                  })}
                {candidate.sourceModel ? ` · ${candidate.sourceModel}` : ''}
              </span>
              <span className='text-muted-foreground tabular-nums'>
                {candidate.probability != null
                  ? `${(candidate.probability * 100).toFixed(1)}%`
                  : '—'}
              </span>
            </li>
          ))}
          {candidates.length > 6 && (
            <li className='text-muted-foreground px-2 text-[11px]'>
              {t('tokenRoutes.detail.moreCandidates', {
                count: candidates.length - 6,
              })}
            </li>
          )}
        </ul>
      )}
    </div>
  )
}
