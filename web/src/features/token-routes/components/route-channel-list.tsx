import { useQueries } from '@tanstack/react-query'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

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

/**
 * The route's channel composition. The wide layout is a compact table (header
 * labels live in this component); the narrow layout falls back to labelled
 * pairs so a phone never has to guess what a number means.
 */
export function RouteChannelList({
  route,
  actions,
}: {
  route: RouteSummaryRow
  actions?: ReactNode
}) {
  const { t } = useTranslation()
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
  const isBusy =
    channelsQuery.isFetching || priceQueries.some((query) => query.isFetching)

  return (
    <div className='@container/channel-list min-w-0 space-y-1.5'>
      <div className='flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1'>
        <div className='flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2 gap-y-0.5'>
          <h3 className='text-sm font-medium'>
            {t('tokenRoutes.detail.channels')}
          </h3>
          {channels.length > 0 && (
            <span className='text-muted-foreground text-xs tabular-nums'>
              {t('tokenRoutes.detail.channelEnabledSummary', {
                enabled: enabledCount,
                total: channels.length,
                weight: enabledWeight,
              })}
            </span>
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

      {channelsQuery.isError ? (
        <QueryErrorBanner
          error={channelsQuery.error}
          messageKey='tokenRoutes.detail.channelsLoadError'
          onRetry={() => {
            void channelsQuery.refetch()
          }}
          isRetrying={channelsQuery.isFetching}
        />
      ) : channelsQuery.isPending && !isReadOnly ? (
        <p
          role='status'
          className='text-muted-foreground border-border rounded-lg border px-3 py-2 text-sm'
        >
          {t('common.loading')}
        </p>
      ) : channels.length === 0 ? (
        <p className='text-muted-foreground rounded-lg border border-dashed px-3 py-2 text-center text-sm'>
          {t(
            isReadOnly
              ? 'tokenRoutes.detail.channelEmptyReadOnly'
              : 'tokenRoutes.detail.channelEmptyEditable'
          )}
        </p>
      ) : (
        <div className='overflow-hidden rounded-md border'>
          {/* One quiet header per expansion; the rows below stay single-line
              so several open routes do not turn into a wall of labels. */}
          <div
            aria-hidden='true'
            className='bg-muted/40 text-muted-foreground text-2xs hidden grid-cols-[minmax(0,1fr)_2.5rem_5.5rem_5rem_4rem] items-center gap-x-3 border-b px-2.5 py-1 @[36rem]/channel-list:grid @[44rem]/channel-list:grid-cols-[minmax(0,1fr)_2.5rem_6rem_5.5rem_7rem_4.5rem]'
          >
            <span>{t('tokenRoutes.detail.channelColumnIdentity')}</span>
            <span>{t('tokenRoutes.detail.channelColumnPriority')}</span>
            <span>{t('tokenRoutes.detail.channelColumnWeightShare')}</span>
            <span>{t('tokenRoutes.detail.channelColumnHealth')}</span>
            <span className='hidden @[44rem]/channel-list:block'>
              {t('tokenRoutes.detail.channelColumnPrice')}
            </span>
            <span className='text-right'>
              {t('tokenRoutes.detail.channelColumnStatus')}
            </span>
          </div>
          <ul className='divide-y'>
            {channels.map((channel) => {
              const priceTruth = resolveRouteChannelPriceTruth(
                route,
                channel,
                priceRowsByModel
              )
              const allocation = allocations.get(channel.id) ?? {
                channelId: channel.id,
                configuredWeight: channel.weight,
                enabledWeightShare: null,
              }
              return (
                <ChannelRow
                  key={channel.id}
                  channel={channel}
                  allocation={allocation}
                  priceTruth={priceTruth}
                  priceQueryState={
                    priceTruth.concreteModel
                      ? priceQueryStateByModel.get(
                          normalizeModelKey(priceTruth.concreteModel)
                        )
                      : undefined
                  }
                />
              )
            })}
          </ul>
        </div>
      )}

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
  allocation,
  priceTruth,
  priceQueryState,
}: {
  channel: RouteChannel
  allocation: RouteChannelAllocation
  priceTruth: RouteChannelPriceTruth
  priceQueryState: PriceQueryState | undefined
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const accountLabel =
    channel.account?.username ||
    t('tokenRoutes.detail.fallbackAccount', { id: channel.accountId })
  const siteLabel = channel.site?.name || channel.site?.platform || ''
  const tokenBound = Boolean(channel.token?.name || channel.tokenId)
  // An account-scoped channel (no token_id) relays with the account's own
  // credential. Only the absence of both masked credentials is the broken
  // state; see the route-detail-sheet tests for the three wire shapes.
  const accountCredential = Boolean(
    channel.account?.accessTokenMasked || channel.account?.apiTokenMasked
  )
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
  const sourceModel = priceTruth.concreteModel || '—'
  const priceUnavailable = isPriceUnavailable(priceTruth, priceQueryState)
  const cooldownActive =
    Boolean(channel.cooldownUntil) &&
    new Date(channel.cooldownUntil as string) > new Date()
  const missingCredential = !tokenBound && !accountCredential
  let tokenHint: string | undefined
  if (!tokenBound) {
    tokenHint = accountCredential
      ? t('tokenRoutes.detail.channelTokenAccountHint')
      : t('tokenRoutes.detail.channelTokenMissingHint')
  }
  let status: { label: string; dot: string; title?: string }
  if (!channel.enabled) {
    status = {
      label: t('tokenRoutes.columns.disable'),
      dot: 'bg-muted-foreground',
    }
  } else if (missingCredential) {
    status = {
      label: t('tokenRoutes.detail.channelStatusMissingCredential'),
      dot: 'bg-destructive',
    }
  } else if (cooldownActive) {
    status = {
      label: t('tokenRoutes.detail.cooldown'),
      dot: 'bg-warning',
      title: t('tokenRoutes.detail.channelCooldownUntil', {
        time: formatDateTime(channel.cooldownUntil, locale),
      }),
    }
  } else {
    status = {
      label: t('tokenRoutes.detail.channelStatusHealthy'),
      dot: 'bg-success',
    }
  }

  return (
    <li className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 px-3 py-2 text-sm @[36rem]/channel-list:grid-cols-[minmax(0,1fr)_2.5rem_5.5rem_5rem_4rem] @[36rem]/channel-list:px-2.5 @[36rem]/channel-list:py-1.5 @[44rem]/channel-list:grid-cols-[minmax(0,1fr)_2.5rem_6rem_5.5rem_7rem_4.5rem]'>
      <div className='col-start-1 row-start-1 min-w-0'>
        <div className='flex min-w-0 items-baseline gap-1.5'>
          <span
            className='max-w-full shrink-0 truncate font-medium'
            title={accountLabel}
          >
            {accountLabel}
          </span>
          {siteLabel && (
            <span
              className='text-muted-foreground truncate text-xs'
              title={siteLabel}
            >
              @ {siteLabel}
            </span>
          )}
        </div>
        <div className='text-muted-foreground text-2xs mt-0.5 flex min-w-0 items-center gap-1.5'>
          <code className='truncate font-mono' title={sourceModel}>
            {sourceModel}
          </code>
          <span
            className={cn(
              'truncate',
              missingCredential && 'text-destructive font-medium'
            )}
            title={tokenHint}
          >
            {t('tokenRoutes.detail.channelToken')} {tokenLabel}
          </span>
        </div>
      </div>

      <div className='col-start-2 row-start-1 justify-self-end @[36rem]/channel-list:col-start-5 @[44rem]/channel-list:col-start-6'>
        <span
          data-testid='channel-status'
          className='inline-flex items-center gap-1.5 text-xs font-medium whitespace-nowrap'
          title={status.title}
        >
          <span className={cn('size-1.5 rounded-full', status.dot)} />
          {status.label}
        </span>
      </div>

      {/* Four metrics read as one inline run on phones and dissolve into the
          table columns once the container can show the single-line header. */}
      <div className='text-muted-foreground text-2xs col-span-2 row-start-2 mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 @[36rem]/channel-list:contents'>
        <ChannelMetric
          label={t('tokenRoutes.detail.channelColumnPriority')}
          compactLabel={t('tokenRoutes.detail.channelMetricPriority')}
          className='@[36rem]/channel-list:col-start-2'
        >
          {channel.priority}
        </ChannelMetric>

        <ChannelMetric
          label={t('tokenRoutes.detail.channelColumnWeightShare')}
          compactLabel={t('tokenRoutes.detail.channelMetricWeight')}
          className='@[36rem]/channel-list:col-start-3'
        >
          {allocation.configuredWeight}
          <span className='text-muted-foreground'> · </span>
          {formatRouteWeightShare(allocation.enabledWeightShare)}
        </ChannelMetric>

        <ChannelMetric
          label={t('tokenRoutes.detail.channelColumnHealth')}
          compactLabel={t('tokenRoutes.detail.channelMetricHealth')}
          className='@[36rem]/channel-list:col-start-4'
          valueTestId='channel-health'
        >
          <span>{formatInt(channel.successCount)}</span>
          <span className='text-muted-foreground'> / </span>
          <span className={channel.failCount > 0 ? 'text-destructive' : ''}>
            {formatInt(channel.failCount)}
          </span>
        </ChannelMetric>

        <ChannelMetric
          label={t('tokenRoutes.detail.channelColumnPrice')}
          compactLabel={t('tokenRoutes.detail.channelMetricPrice')}
          className={cn(
            '@[44rem]/channel-list:col-start-5 @[44rem]/channel-list:block',
            // An unavailable price is not worth a wrapped line on a phone;
            // the detail sheet still spells it out.
            priceUnavailable ? 'hidden' : '@[36rem]/channel-list:hidden'
          )}
        >
          <ChannelPrice
            priceTruth={priceTruth}
            priceQueryState={priceQueryState}
          />
        </ChannelMetric>
      </div>
    </li>
  )
}

function ChannelMetric({
  label,
  compactLabel,
  className,
  valueTestId,
  children,
}: {
  label: string
  compactLabel?: string
  className?: string
  valueTestId?: string
  children: ReactNode
}) {
  return (
    <div
      className={cn(
        'flex min-w-0 items-baseline gap-1 whitespace-nowrap',
        className
      )}
    >
      <span
        className={cn(
          compactLabel
            ? '@[36rem]/channel-list:hidden'
            : '@[36rem]/channel-list:sr-only'
        )}
      >
        {compactLabel ?? label}
      </span>
      {compactLabel && (
        <span className='hidden @[36rem]/channel-list:sr-only'>{label}</span>
      )}{' '}
      <div
        data-testid={valueTestId}
        className='text-foreground min-w-0 tabular-nums @[36rem]/channel-list:text-sm'
      >
        {children}
      </div>
    </div>
  )
}

function ChannelPrice({
  priceTruth,
  priceQueryState,
}: {
  priceTruth: RouteChannelPriceTruth
  priceQueryState: PriceQueryState | undefined
}) {
  const { t } = useTranslation()
  const ratesFallback =
    normalizePriceGrade(priceTruth.provenance.ratesSource) === 'fallback'
  const priceUnavailable = isPriceUnavailable(priceTruth, priceQueryState)
  const costSource = priceTruth.provenance.source
  const costSourceDiffers =
    Boolean(costSource) && costSource !== priceTruth.provenance.ratesSource

  return (
    <div className='flex min-w-0 items-center gap-1.5'>
      {priceQueryState?.isLoading ? (
        <span className='text-muted-foreground inline-flex items-center gap-1 whitespace-nowrap'>
          <Spinner className='size-3' />
          <span className='sr-only'>
            {t('tokenRoutes.detail.channelPriceLoading')}
          </span>
          <span aria-hidden='true'>—</span>
        </span>
      ) : priceUnavailable ? (
        <span
          className='text-muted-foreground whitespace-nowrap'
          title={t('tokenRoutes.detail.channelPriceUnavailable')}
        >
          <span aria-hidden='true'>—</span>
          <span className='sr-only'>
            {t('tokenRoutes.detail.channelPriceUnavailable')}
          </span>
        </span>
      ) : (
        <span className='whitespace-nowrap'>
          {formatPrice(priceTruth.inputPerMillion, {
            fractionDigits: 4,
          })}
          <span className='text-muted-foreground'> / </span>
          {formatPrice(priceTruth.outputPerMillion, {
            fractionDigits: 4,
          })}
        </span>
      )}
      {!priceQueryState?.isLoading &&
        !ratesFallback &&
        priceTruth.provenance.ratesSource && (
          <span className='hidden items-center @[36rem]/channel-list:inline-flex'>
            <span className='sr-only'>
              {t('tokenRoutes.detail.channelRateProvenance')}{' '}
            </span>
            <PriceGradeBadge grade={priceTruth.provenance.ratesSource} />
          </span>
        )}
      {!priceQueryState?.isLoading && costSourceDiffers && costSource && (
        <span className='hidden items-center @[36rem]/channel-list:inline-flex'>
          <span className='sr-only'>
            {t('tokenRoutes.detail.channelCostProvenance')}{' '}
          </span>
          <PriceGradeBadge grade={costSource} />
        </span>
      )}
    </div>
  )
}
