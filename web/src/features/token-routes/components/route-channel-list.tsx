import { useQueries } from '@tanstack/react-query'
import {
  Check,
  CircleAlert,
  CircleCheck,
  CircleDollarSign,
  CirclePause,
  Gauge,
  KeyRound,
  Network,
  RadioTower,
  Server,
  Snowflake,
  X,
} from 'lucide-react'
import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
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
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime, formatInt, formatPrice } from '@/lib/format'
import { getPlatformDefinition } from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

import { useRouteChannels } from '../api'
import {
  calculateRouteChannelAllocations,
  formatRouteWeightShare,
  normalizeModelKey,
  resolveDistinctConcreteModels,
  resolveRouteChannelPriceTruth,
  type RouteChannelAllocation,
  type RouteChannelPriceTruth,
} from '../lib/route-price-truth'
import type { RouteChannel, RouteSummaryRow } from '../types'

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

function hasAccountCredential(channel: RouteChannel): boolean {
  return Boolean(
    channel.account?.accessTokenMasked || channel.account?.apiTokenMasked
  )
}

function resolveChannelState(channel: RouteChannel): ChannelState {
  if (!channel.enabled) return 'disabled'
  const tokenBound = Boolean(channel.token?.name || channel.tokenId)
  if (!tokenBound && !hasAccountCredential(channel)) return 'missing'
  if (
    channel.cooldownUntil &&
    new Date(channel.cooldownUntil).getTime() > Date.now()
  ) {
    return 'cooling'
  }
  return 'healthy'
}

function ChannelStatusIcon({
  state,
  className,
}: {
  state: ChannelState
  className?: string
}) {
  let Icon = CirclePause
  let color = 'text-muted-foreground'
  if (state === 'healthy') {
    Icon = CircleCheck
    color = 'text-success'
  } else if (state === 'cooling') {
    Icon = Snowflake
    color = 'text-warning'
  } else if (state === 'missing') {
    Icon = CircleAlert
    color = 'text-destructive'
  }
  return <Icon aria-hidden='true' className={cn('size-4', color, className)} />
}

type ChannelState = 'healthy' | 'disabled' | 'cooling' | 'missing'

const CHANNEL_STATUS_LABEL_KEY = {
  healthy: 'tokenRoutes.detail.channelStatusHealthy',
  disabled: 'tokenRoutes.detail.channelStatusDisabled',
  cooling: 'tokenRoutes.detail.cooldown',
  missing: 'tokenRoutes.detail.channelStatusMissingCredential',
} as const

const CHANNEL_STATUS_BADGE_VARIANT = {
  healthy: 'success',
  disabled: 'secondary',
  cooling: 'warning',
  missing: 'destructive',
} as const

function ChannelStatusBadge({
  state,
  title,
}: {
  state: ChannelState
  title?: string
}) {
  const { t } = useTranslation()
  const label = t(CHANNEL_STATUS_LABEL_KEY[state])
  return (
    <Badge
      data-testid='channel-status'
      variant={CHANNEL_STATUS_BADGE_VARIANT[state]}
      title={title ?? label}
      className='gap-1'
    >
      <ChannelStatusIcon state={state} />
      <span>{label}</span>
    </Badge>
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
      priceQueryState,
      priceUnavailable: isPriceUnavailable(priceTruth, priceQueryState),
      showSourceModel: Boolean(
        priceTruth.concreteModel &&
        priceTruth.concreteModel.trim().toLowerCase() !== routePattern
      ),
    }
  })
  const stateCounts = channelViews.reduce<Record<ChannelState, number>>(
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
            <ChannelRow
              key={view.channel.id}
              channel={view.channel}
              state={view.state}
              allocation={view.allocation}
              priceTruth={view.priceTruth}
              showSourceModel={view.showSourceModel}
              priceQueryState={view.priceQueryState}
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

function ChannelRow({
  channel,
  state,
  allocation,
  priceTruth,
  showSourceModel,
  priceQueryState,
}: {
  channel: RouteChannel
  state: ChannelState
  allocation: RouteChannelAllocation
  priceTruth: RouteChannelPriceTruth
  showSourceModel: boolean
  priceQueryState: PriceQueryState | undefined
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const accountLabel =
    channel.account?.username ||
    t('tokenRoutes.detail.fallbackAccount', { id: channel.accountId })
  const siteLabel = channel.site?.name || channel.site?.platform || ''
  const tokenBound = Boolean(channel.token?.name || channel.tokenId)
  const accountCredential = hasAccountCredential(channel)
  let tokenLabel: string
  if (channel.token?.name) {
    tokenLabel = channel.token.name
  } else if (channel.tokenId) {
    tokenLabel = t('tokenRoutes.detail.fallbackToken', { id: channel.tokenId })
  } else if (accountCredential) {
    tokenLabel = t('tokenRoutes.detail.channelTokenAccount')
  } else {
    tokenLabel = t('tokenRoutes.detail.channelTokenMissing')
  }
  const sourceModel = priceTruth.concreteModel || ''
  const priceUnavailable = isPriceUnavailable(priceTruth, priceQueryState)
  const missingCredential = state === 'missing'
  let tokenHint: string | undefined
  if (!tokenBound) {
    tokenHint = accountCredential
      ? t('tokenRoutes.detail.channelTokenAccountHint')
      : t('tokenRoutes.detail.channelTokenMissingHint')
  }
  const statusTitle =
    state === 'cooling'
      ? t('tokenRoutes.detail.channelCooldownUntil', {
          time: formatDateTime(channel.cooldownUntil, locale),
        })
      : undefined
  const inputPrice = formatPrice(priceTruth.inputPerMillion)
  const outputPrice = formatPrice(priceTruth.outputPerMillion)
  const priceTitle = priceUnavailable
    ? undefined
    : `${t('tokenRoutes.detail.channelInputPrice')}: ${inputPrice} · ${t(
        'tokenRoutes.detail.channelOutputPrice'
      )}: ${outputPrice} · ${t('tokenRoutes.detail.channelRateProvenance')}: ${t(
        `priceCompare.grade.${normalizePriceGrade(priceTruth.provenance.ratesSource)}`
      )}`
  const weightPercent =
    allocation.enabledWeightShare === null
      ? null
      : Math.round(allocation.enabledWeightShare * 100)
  const platform = getPlatformDefinition(channel.site?.platform)
  const siteIcon = platform?.icon
  let PlatformFallback = Server
  if (platform?.group === 'gateway') PlatformFallback = Network
  else if (platform?.group === 'oauth') PlatformFallback = KeyRound

  return (
    <li
      className={cn(
        'flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5 rounded-lg px-2 py-2 text-sm transition-colors hover:bg-muted/60',
        state === 'disabled' && 'bg-muted/20 text-muted-foreground',
        state === 'cooling' && 'bg-warning/5',
        state === 'missing' && 'bg-destructive/5'
      )}
    >
      <div
        className='shrink-0 self-start @[42rem]/channel-list:self-center'
        title={siteLabel || accountLabel}
        aria-hidden='true'
      >
        <span className='bg-background grid size-8 place-items-center rounded-lg shadow-sm ring-1 ring-black/10 dark:ring-white/20'>
          {siteIcon ? (
            <BrandGlyph icon={siteIcon} size={21} alt='' />
          ) : (
            <PlatformFallback
              className='text-muted-foreground size-4'
              aria-hidden='true'
            />
          )}
        </span>
      </div>
      <div className='min-w-0 flex-1 basis-56'>
        <div className='flex min-w-0 items-center gap-x-2'>
          <span
            className={cn(
              'truncate text-sm font-medium',
              state === 'disabled' && 'text-muted-foreground'
            )}
            title={accountLabel}
          >
            {accountLabel}
          </span>
          <Badge
            variant='outline'
            className='text-2xs h-4 shrink-0 rounded-md px-1 font-mono'
            title={`${t('tokenRoutes.detail.channelColumnPriority')} ${channel.priority + 1}`}
          >
            #{channel.priority + 1}
          </Badge>
        </div>
        <div className='text-muted-foreground mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs'>
          {siteLabel && (
            <span
              className='inline-flex min-w-0 items-center gap-1'
              title={siteLabel}
            >
              <Server className='size-3 shrink-0' aria-hidden='true' />
              <span className='max-w-40 truncate'>{siteLabel}</span>
            </span>
          )}
          {siteLabel && <span aria-hidden='true'>·</span>}
          <span className='inline-flex min-w-0 items-center gap-1'>
            <KeyRound className='size-3 shrink-0' aria-hidden='true' />
            <span
              className={cn(
                'min-w-0 truncate',
                missingCredential && 'text-destructive font-medium'
              )}
              title={tokenHint ?? tokenLabel}
            >
              {tokenLabel}
            </span>
          </span>
          {!priceUnavailable && (
            <>
              <span aria-hidden='true'>·</span>
              <span
                className='text-foreground inline-flex items-center gap-1'
                title={priceTitle}
              >
                <CircleDollarSign
                  className='size-3 shrink-0'
                  aria-hidden='true'
                />
                <span className='tabular-nums'>
                  {inputPrice} <span aria-hidden='true'>→</span> {outputPrice}
                </span>
              </span>
            </>
          )}
          {showSourceModel && sourceModel && (
            <span
              className='bg-muted/70 text-muted-foreground text-2xs inline-flex max-w-44 shrink-0 items-center gap-1 truncate rounded px-1.5 py-0.5'
              title={`${t('tokenRoutes.detail.channelSourceModel')}: ${sourceModel}`}
            >
              <BrandGlyph model={sourceModel} size={11} />
              <code className='truncate font-mono'>{sourceModel}</code>
            </span>
          )}
        </div>
      </div>

      <div className='flex w-full min-w-0 flex-wrap items-center gap-x-3 gap-y-1 pl-11 @[42rem]/channel-list:ml-auto @[42rem]/channel-list:w-auto @[42rem]/channel-list:flex-nowrap @[42rem]/channel-list:pl-0'>
        <div
          className='min-w-24 flex-1 @[42rem]/channel-list:w-24 @[42rem]/channel-list:flex-none'
          title={`${t('tokenRoutes.detail.channelColumnWeightShare')}: ${allocation.configuredWeight} · ${formatRouteWeightShare(allocation.enabledWeightShare)}`}
        >
          <div className='text-muted-foreground flex items-center justify-between gap-2 text-xs'>
            <Gauge className='size-3.5' aria-hidden='true' />
            <span className='sr-only'>
              {t('tokenRoutes.detail.channelColumnWeightShare')}
            </span>
            <span className='text-foreground font-medium tabular-nums'>
              {formatRouteWeightShare(allocation.enabledWeightShare)}
            </span>
          </div>
          <span className='bg-muted mt-1 block h-1 overflow-hidden rounded-full'>
            <span
              className='bg-primary block h-full rounded-full'
              style={{ width: `${weightPercent ?? 0}%` }}
            />
          </span>
        </div>

        <div
          data-testid='channel-health'
          role='group'
          aria-label={`${t('tokenRoutes.detail.channelColumnHealth')}: ${formatInt(
            channel.successCount
          )} / ${formatInt(channel.failCount)}`}
          className='flex min-w-0 items-center gap-2 text-xs whitespace-nowrap tabular-nums'
          title={t('tokenRoutes.detail.channelColumnHealth')}
        >
          <span className='inline-flex items-center gap-1'>
            <Check className='text-success size-3' aria-hidden='true' />
            <span className='text-foreground'>
              {formatInt(channel.successCount)}
            </span>
          </span>
          <span className='inline-flex items-center gap-1'>
            <X
              className={cn(
                'size-3',
                channel.failCount > 0
                  ? 'text-destructive'
                  : 'text-muted-foreground'
              )}
              aria-hidden='true'
            />
            <span
              className={
                channel.failCount > 0 ? 'text-destructive' : 'text-foreground'
              }
            >
              {formatInt(channel.failCount)}
            </span>
          </span>
        </div>

        <ChannelStatusBadge state={state} title={statusTitle} />
      </div>
    </li>
  )
}
