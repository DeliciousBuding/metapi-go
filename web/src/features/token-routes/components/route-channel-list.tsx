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

/**
 * The route's channel composition, rendered as a detail panel under the route
 * row rather than a second table: no nested card, one shared column legend,
 * single-line channel rows on wide containers, labelled pairs on phones.
 *
 * The panel has to stay readable when several routes are open, so every piece
 * of chrome is charged against the six facts an operator needs per channel.
 * The legend and the rows share one grid template; drift between the two would
 * make every number ambiguous.
 */
const CHANNEL_ROW_GRID =
  'grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-x-2.5 gap-y-0.5 @[36rem]/channel-list:grid-cols-[auto_minmax(0,1fr)_5.5rem_6rem_1.5rem] @[36rem]/channel-list:items-center @[36rem]/channel-list:gap-y-0'

type ChannelState = 'healthy' | 'disabled' | 'cooling' | 'missing'

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
      <div className='flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1'>
        <div className='flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2 gap-y-0.5'>
          <h3 className='text-sm font-medium'>
            {t('tokenRoutes.detail.channels')}
          </h3>
          {channels.length > 0 && (
            <span
              className='text-muted-foreground inline-flex items-center gap-1 text-xs tabular-nums'
              title={t('tokenRoutes.detail.channelEnabledSummary', {
                enabled: enabledCount,
                total: channels.length,
                weight: enabledWeight,
              })}
            >
              <RadioTower className='size-3.5' aria-hidden='true' />
              <span className='@[36rem]/channel-list:hidden'>
                {enabledCount}/{channels.length}
              </span>
              <span className='hidden @[36rem]/channel-list:inline'>
                {t('tokenRoutes.detail.channelHeaderEnabled', {
                  enabled: enabledCount,
                  total: channels.length,
                })}
              </span>
            </span>
          )}
          {channels.length > 0 && (
            <span
              className='text-muted-foreground inline-flex items-center gap-1 text-xs tabular-nums'
              title={t('tokenRoutes.detail.channelTotalWeight', {
                weight: enabledWeight,
              })}
            >
              <Gauge className='size-3.5' aria-hidden='true' />
              <span className='@[36rem]/channel-list:hidden'>
                {enabledWeight}
              </span>
              <span className='hidden @[36rem]/channel-list:inline'>
                {t('tokenRoutes.detail.channelHeaderWeight', {
                  weight: enabledWeight,
                })}
              </span>
            </span>
          )}
          {stateCounts.cooling > 0 && (
            <span
              className='text-muted-foreground inline-flex items-center gap-1 text-xs tabular-nums'
              title={t('tokenRoutes.detail.channelStatusCoolingSummary', {
                count: stateCounts.cooling,
              })}
            >
              <Snowflake className='text-warning size-3.5' aria-hidden='true' />
              <span className='@[36rem]/channel-list:hidden'>
                {stateCounts.cooling}
              </span>
              <span className='hidden @[36rem]/channel-list:inline'>
                {t('tokenRoutes.detail.channelHeaderCooling', {
                  count: stateCounts.cooling,
                })}
              </span>
            </span>
          )}
          {stateCounts.missing > 0 && (
            <span
              className='text-muted-foreground inline-flex items-center gap-1 text-xs tabular-nums'
              title={t('tokenRoutes.detail.channelStatusMissingSummary', {
                count: stateCounts.missing,
              })}
            >
              <CircleAlert
                className='text-destructive size-3.5'
                aria-hidden='true'
              />
              <span className='@[36rem]/channel-list:hidden'>
                {stateCounts.missing}
              </span>
              <span className='hidden @[36rem]/channel-list:inline'>
                {t('tokenRoutes.detail.channelHeaderMissing', {
                  count: stateCounts.missing,
                })}
              </span>
            </span>
          )}
          {showPriceSummary && (
            <span
              data-testid='channel-price-summary'
              className='text-muted-foreground inline-flex items-center gap-1 text-xs'
              title={t('tokenRoutes.detail.channelPriceUnavailable')}
            >
              <CircleDollarSign className='size-3.5' aria-hidden='true' />
              <span aria-hidden='true'>—</span>
              <span className='sr-only'>
                {t('tokenRoutes.detail.channelPriceUnavailable')}
              </span>
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

      {channels.length > 0 && (
        <div className='text-muted-foreground text-2xs mt-2 hidden grid-cols-[auto_minmax(0,1fr)_5.5rem_6rem_1.5rem] items-center gap-x-2.5 px-1.5 @[36rem]/channel-list:grid'>
          <span aria-hidden='true' />
          <span>{t('tokenRoutes.detail.channelColumnIdentity')}</span>
          <span className='text-right'>
            {t('tokenRoutes.detail.channelColumnWeightShare')}
          </span>
          <span className='text-right'>
            {t('tokenRoutes.detail.channelColumnHealth')}
          </span>
          <span className='sr-only'>
            {t('tokenRoutes.detail.channelColumnStatus')}
          </span>
        </div>
      )}

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
  let status: { label: string; title?: string }
  switch (state) {
    case 'disabled':
      status = {
        label: t('tokenRoutes.columns.disable'),
      }
      break
    case 'missing':
      status = {
        label: t('tokenRoutes.detail.channelStatusMissingCredential'),
      }
      break
    case 'cooling':
      status = {
        label: t('tokenRoutes.detail.cooldown'),
        title: t('tokenRoutes.detail.channelCooldownUntil', {
          time: formatDateTime(channel.cooldownUntil, locale),
        }),
      }
      break
    default:
      status = {
        label: t('tokenRoutes.detail.channelStatusHealthy'),
      }
  }
  const priceLabel = priceUnavailable
    ? null
    : `${formatPrice(priceTruth.inputPerMillion, {
        fractionDigits: 4,
      })} / ${formatPrice(priceTruth.outputPerMillion, {
        fractionDigits: 4,
      })}`
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
        CHANNEL_ROW_GRID,
        'rounded-lg px-1.5 py-1.5 text-sm transition-colors hover:bg-muted/60',
        state === 'disabled' && 'text-muted-foreground'
      )}
    >
      {/* Model glyph carries the model family; the small corner badge keeps
          the upstream site visible without repeating its name on every row. */}
      <div
        className='col-start-1 row-span-2 row-start-1 self-start @[36rem]/channel-list:self-center'
        title={siteLabel || accountLabel}
        aria-hidden='true'
      >
        <span className='bg-background grid size-7 place-items-center rounded-md shadow-sm ring-1 ring-black/10 dark:ring-white/20'>
          {siteIcon ? (
            <BrandGlyph icon={siteIcon} size={20} alt='' />
          ) : (
            <PlatformFallback
              className='text-muted-foreground size-4'
              aria-hidden='true'
            />
          )}
        </span>
      </div>
      <div className='col-start-2 row-start-1 min-w-0'>
        <div className='flex min-w-0 items-baseline gap-x-1.5'>
          <span
            className={cn(
              'truncate text-xs font-medium @[36rem]/channel-list:text-sm',
              state === 'disabled' && 'text-muted-foreground'
            )}
            title={`${accountLabel} · ${t('tokenRoutes.detail.channelColumnPriority')} ${channel.priority}`}
          >
            {accountLabel}
          </span>
          <span
            className='inline-flex shrink-0 items-center @[36rem]/channel-list:hidden'
            title={status.title}
          >
            <ChannelStatusIcon state={state} />
            <span className='sr-only'>{status.label}</span>
          </span>
        </div>
      </div>
      <div className='text-muted-foreground text-2xs col-start-2 row-start-2 flex min-w-0 items-center gap-x-1'>
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
        {siteLabel && <span className='sr-only'>{siteLabel}</span>}
        {showSourceModel && sourceModel && (
          <span
            className='bg-muted/70 text-muted-foreground text-2xs hidden max-w-32 shrink-0 items-center gap-1 truncate rounded px-1 py-0.5 @[36rem]/channel-list:inline-flex'
            title={`${t('tokenRoutes.detail.channelSourceModel')}: ${sourceModel}`}
          >
            <BrandGlyph model={sourceModel} size={11} />
            <code className='truncate font-mono'>{sourceModel}</code>
          </span>
        )}
        {priceLabel && (
          <span
            className='hidden shrink-0 items-center gap-1 @[44rem]/channel-list:inline-flex'
            title={`${t('tokenRoutes.detail.channelColumnPrice')} · ${t('tokenRoutes.detail.channelRateProvenance')}: ${t(
              `priceCompare.grade.${normalizePriceGrade(priceTruth.provenance.ratesSource)}`
            )}`}
          >
            <span className='text-muted-foreground'>·</span>
            <CircleDollarSign className='size-3' aria-hidden='true' />
            <span className='text-foreground tabular-nums'>{priceLabel}</span>
          </span>
        )}
      </div>

      <div className='col-start-3 row-start-1 justify-self-end text-right @[36rem]/channel-list:col-start-3'>
        <div
          className='flex items-center justify-end gap-1'
          title={`${t('tokenRoutes.detail.channelColumnWeightShare')}: ${allocation.configuredWeight} · ${formatRouteWeightShare(allocation.enabledWeightShare)}`}
        >
          <span className='bg-muted h-1 w-8 overflow-hidden rounded-full @[36rem]/channel-list:w-9'>
            <span
              className='bg-primary block h-full rounded-full'
              style={{ width: `${weightPercent ?? 0}%` }}
            />
          </span>
          <span className='text-2xs font-medium tabular-nums'>
            {formatRouteWeightShare(allocation.enabledWeightShare)}
          </span>
        </div>
      </div>

      <div
        data-testid='channel-health'
        className='text-2xs col-start-3 row-start-2 flex min-w-0 items-center gap-x-1 justify-self-end tabular-nums @[36rem]/channel-list:col-start-4 @[36rem]/channel-list:row-start-1'
        title={t('tokenRoutes.detail.channelColumnHealth')}
      >
        <span className='inline-flex items-center gap-0.5'>
          <Check className='text-success size-3' aria-hidden='true' />
          <span className='text-foreground text-xs'>
            {formatInt(channel.successCount)}
          </span>
        </span>
        <span className='text-muted-foreground'>{' / '}</span>
        <span className='inline-flex items-center gap-0.5'>
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
            className={cn(
              'text-xs',
              channel.failCount > 0 ? 'text-destructive' : 'text-foreground'
            )}
          >
            {formatInt(channel.failCount)}
          </span>
        </span>
      </div>

      <div className='hidden justify-self-end @[36rem]/channel-list:col-start-5 @[36rem]/channel-list:row-start-1 @[36rem]/channel-list:block'>
        <span
          data-testid='channel-status'
          className='inline-flex items-center'
          title={status.title ?? status.label}
        >
          <ChannelStatusIcon state={state} />
          <span className='sr-only'>{status.label}</span>
        </span>
      </div>
    </li>
  )
}
