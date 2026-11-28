import { useQueries } from '@tanstack/react-query'
import {
  CircleAlert,
  CircleDollarSign,
  Gauge,
  RadioTower,
  Snowflake,
} from 'lucide-react'
import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  normalizePriceGrade,
  priceCompareQueryOptions,
  PriceGradeBadge,
  type PriceCompareItem,
} from '@/features/models'
import { useCooldownClock } from '@/hooks/use-cooldown-clock'

import { useRouteChannels } from '../api'
import {
  calculateRouteChannelAllocations,
  normalizeModelKey,
  resolveDistinctConcreteModels,
  resolveRouteChannelPriceTruth,
  type RouteChannelPriceTruth,
} from '../lib/route-price-truth'
import type { RouteSummaryRow } from '../types'
import { resolveChannelState, RouteChannelRow } from './route-channel-row'

type PriceQueryState = { isLoading: boolean; hasError: boolean }

function isPriceUnavailable(
  priceTruth: RouteChannelPriceTruth,
  priceQueryState: PriceQueryState | undefined
): boolean {
  return Boolean(
    priceQueryState?.isLoading ||
    priceQueryState?.hasError ||
    !priceTruth.price ||
    priceTruth.price.missingPrice ||
    !priceTruth.provenance.ratesSource ||
    normalizePriceGrade(priceTruth.provenance.ratesSource) === 'fallback'
  )
}

export function RouteChannelList({
  route,
  actions,
}: {
  route: RouteSummaryRow
  actions?: ReactNode
}) {
  const { t } = useTranslation()
  const rootRef = useRef<HTMLDivElement>(null)
  const channelsQuery = useRouteChannels(
    route.kind === 'zero_channel' ? null : route.id
  )
  const channels = useMemo(() => channelsQuery.data ?? [], [channelsQuery.data])
  const cooldownNow = useCooldownClock(
    useMemo(() => channels.map((channel) => channel.cooldownUntil), [channels])
  )
  const concreteModels = useMemo(
    () => resolveDistinctConcreteModels(route, channels),
    [route, channels]
  )
  const priceQueries = useQueries({
    queries: concreteModels.map((model) =>
      priceCompareQueryOptions({ model, limit: 200, exactModel: true })
    ),
  })
  const priceRowsByModel = new Map<string, readonly PriceCompareItem[]>()
  const priceQueryStateByModel = new Map<string, PriceQueryState>()
  concreteModels.forEach((model, index) => {
    const query = priceQueries[index]
    priceRowsByModel.set(normalizeModelKey(model), query?.data ?? [])
    priceQueryStateByModel.set(normalizeModelKey(model), {
      isLoading: query?.isPending ?? false,
      hasError: query?.isError ?? false,
    })
  })
  const allocations = new Map(
    calculateRouteChannelAllocations(channels).map((entry) => [
      entry.channelId,
      entry,
    ])
  )
  const isReadOnly = route.kind === 'zero_channel' || route.readOnly === true
  const enabledCount = channels.reduce(
    (count, channel) => count + (channel.enabled ? 1 : 0),
    0
  )
  const enabledWeight = channels.reduce(
    (weight, channel) => weight + (channel.enabled ? channel.weight : 0),
    0
  )
  const isBusy = channelsQuery.isFetching
  const routePattern = route.modelPattern.trim().toLowerCase()
  const channelViews = channels.map((channel) => {
    const priceTruth = resolveRouteChannelPriceTruth(
      route,
      channel,
      priceRowsByModel
    )
    const priceQueryState = priceTruth.concreteModel
      ? priceQueryStateByModel.get(normalizeModelKey(priceTruth.concreteModel))
      : undefined
    return {
      channel,
      state: resolveChannelState(channel),
      allocation: allocations.get(channel.id) ?? {
        channelId: channel.id,
        configuredWeight: channel.weight,
        enabledWeightShare: null,
      },
      priceTruth,
      priceUnavailable: isPriceUnavailable(priceTruth, priceQueryState),
      showSourceModel: Boolean(
        priceTruth.concreteModel &&
        priceTruth.concreteModel.trim().toLowerCase() !== routePattern
      ),
    }
  })
  const stateCounts = channelViews.reduce(
    (counts, view) => {
      counts[view.state] += 1
      return counts
    },
    { healthy: 0, disabled: 0, cooling: 0, missing: 0 }
  )
  const pricesSettled =
    priceQueries.length > 0 && priceQueries.every((query) => !query.isPending)
  const priceUnavailableCount = channelViews.filter(
    (view) => view.priceUnavailable
  ).length
  const showPriceSummary =
    pricesSettled &&
    priceUnavailableCount > 0 &&
    priceUnavailableCount === channelViews.length
  const unavailableSources = Array.from(
    new Set(
      channelViews.flatMap((view) => {
        const source = view.priceTruth.provenance.source
        return source ? [source] : []
      })
    )
  )

  // Channels arrive after the disclosure mounts. The first reveal can only
  // measure the loading placeholder, so once the real rows (and the optional
  // price summary) are in the DOM, bring the completed panel back into view.
  useEffect(() => {
    if (channelsQuery.isPending) return
    const region = rootRef.current?.closest<HTMLElement>('[role="region"]')
    if (!region || typeof region.scrollIntoView !== 'function') return
    const frame = requestAnimationFrame(() => {
      const rect = region.getBoundingClientRect()
      if (rect.top >= 0 && rect.bottom <= window.innerHeight) return
      const reduceMotion = window.matchMedia?.(
        '(prefers-reduced-motion: reduce)'
      ).matches
      region.scrollIntoView({
        block: 'nearest',
        behavior: reduceMotion ? 'auto' : 'smooth',
      })
    })
    return () => cancelAnimationFrame(frame)
  }, [
    channelsQuery.isPending,
    channels.length,
    showPriceSummary,
    stateCounts.cooling,
    stateCounts.missing,
  ])

  let channelContent: ReactNode
  if (channelsQuery.isError) {
    channelContent = (
      <QueryErrorBanner
        error={channelsQuery.error}
        messageKey='tokenRoutes.detail.channelsLoadError'
        onRetry={() => {
          void channelsQuery.refetch()
        }}
        isRetrying={channelsQuery.isFetching}
      />
    )
  } else if (channelsQuery.isPending && !isReadOnly) {
    channelContent = (
      <p role='status' className='text-muted-foreground mt-1.5 text-xs'>
        {t('common.loading')}
      </p>
    )
  } else if (channels.length === 0) {
    channelContent = (
      <p className='text-muted-foreground mt-1.5 text-xs'>
        {t(
          isReadOnly
            ? 'tokenRoutes.detail.channelEmptyReadOnly'
            : 'tokenRoutes.detail.channelEmptyEditable'
        )}
      </p>
    )
  } else {
    channelContent = (
      <div className='mt-1.5 min-w-0'>
        <ul className='divide-border/50 divide-y'>
          {channelViews.map((view) => (
            <RouteChannelRow
              key={view.channel.id}
              {...view}
              cooldownNow={cooldownNow}
            />
          ))}
        </ul>
      </div>
    )
  }

  return (
    <div ref={rootRef} className='@container/channel-list min-w-0'>
      <div className='flex min-w-0 items-center gap-3'>
        <div className='flex min-w-0 flex-1 flex-wrap items-center gap-1.5'>
          <h3 className='mr-1 text-sm font-medium'>
            {t('tokenRoutes.detail.channels')}
          </h3>
          {channels.length > 0 && (
            <Badge
              variant='success'
              title={t('tokenRoutes.detail.channelEnabledSummary', {
                enabled: enabledCount,
                total: channels.length,
                weight: enabledWeight,
              })}
            >
              <RadioTower />
              {t('tokenRoutes.detail.channelHeaderEnabled', {
                enabled: enabledCount,
                total: channels.length,
              })}
            </Badge>
          )}
          {channels.length > 0 && (
            <Badge
              variant='outline'
              title={t('tokenRoutes.detail.channelTotalWeight', {
                weight: enabledWeight,
              })}
            >
              <Gauge />
              {t('tokenRoutes.detail.channelHeaderWeight', {
                weight: enabledWeight,
              })}
            </Badge>
          )}
          {stateCounts.cooling > 0 && (
            <Badge
              variant='warning'
              title={t('tokenRoutes.detail.channelStatusCoolingSummary', {
                count: stateCounts.cooling,
              })}
            >
              <Snowflake />
              {t('tokenRoutes.detail.channelHeaderCooling', {
                count: stateCounts.cooling,
              })}
            </Badge>
          )}
          {stateCounts.missing > 0 && (
            <Badge
              variant='destructive'
              title={t('tokenRoutes.detail.channelStatusMissingSummary', {
                count: stateCounts.missing,
              })}
            >
              <CircleAlert />
              {t('tokenRoutes.detail.channelHeaderMissing', {
                count: stateCounts.missing,
              })}
            </Badge>
          )}
          {showPriceSummary && (
            <span
              data-testid='channel-price-summary'
              className='inline-flex flex-wrap items-center gap-1'
              title={t('tokenRoutes.detail.channelPriceUnavailable')}
            >
              <Badge
                variant='outline'
                title={t('tokenRoutes.detail.channelPriceUnavailable')}
              >
                <CircleDollarSign />
                {t('tokenRoutes.detail.channelPriceUnavailable')}
              </Badge>
              {unavailableSources.map((source) => (
                <PriceGradeBadge key={source} grade={source} />
              ))}
            </span>
          )}
          {priceQueries.some((query) => query.isPending) && (
            <Spinner
              className='text-muted-foreground size-3.5'
              aria-label={t('tokenRoutes.detail.channelPriceLoading')}
            />
          )}
          {isBusy && (
            <Spinner
              className='text-muted-foreground size-3.5'
              aria-label={t('common.loading')}
            />
          )}
        </div>
        {actions && <div className='flex shrink-0 items-center'>{actions}</div>}
      </div>

      {channelContent}

      {priceQueries.some((query) => query.isError) && (
        <Button
          variant='outline'
          size='sm'
          onClick={() => {
            priceQueries
              .filter((query) => query.isError)
              .forEach((query) => {
                void query.refetch()
              })
          }}
        >
          {t('tokenRoutes.detail.retryPrices')}
        </Button>
      )}
    </div>
  )
}
