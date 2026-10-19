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
    <div className='@container/channel-list min-w-0 space-y-2'>
      <div className='flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5'>
        <div className='flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5'>
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
        {actions && (
          <div className='flex shrink-0 flex-wrap items-center gap-1'>
            {actions}
          </div>
        )}
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
          className='text-muted-foreground border-border rounded-lg border px-3 py-3 text-sm'
        >
          {t('common.loading')}
        </p>
      ) : channels.length === 0 ? (
        <p className='text-muted-foreground rounded-lg border border-dashed px-3 py-2.5 text-center text-sm'>
          {t(
            isReadOnly
              ? 'tokenRoutes.detail.channelEmptyReadOnly'
              : 'tokenRoutes.detail.channelEmptyEditable'
          )}
        </p>
      ) : (
        <div className='overflow-hidden rounded-lg border'>
          <div
            aria-hidden='true'
            className='bg-muted/30 text-muted-foreground text-2xs hidden grid-cols-[minmax(0,1.7fr)_3.5rem_6.5rem_7rem_9.5rem_5rem] items-center gap-x-3 border-b px-3 py-1.5 @[50rem]/channel-list:grid'
          >
            <span>{t('tokenRoutes.detail.channelColumnIdentity')}</span>
            <span>{t('tokenRoutes.detail.channelColumnPriority')}</span>
            <span>{t('tokenRoutes.detail.channelColumnWeightShare')}</span>
            <span>{t('tokenRoutes.detail.channelColumnHealth')}</span>
            <span>{t('tokenRoutes.detail.channelColumnPrice')}</span>
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
    <li className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-start gap-x-3 gap-y-2 px-3 py-2.5 text-sm @[50rem]/channel-list:grid-cols-[minmax(0,1.7fr)_3.5rem_6.5rem_7rem_9.5rem_5rem] @[50rem]/channel-list:items-center @[50rem]/channel-list:gap-y-0'>
      <ChannelField
        label={t('tokenRoutes.detail.channelColumnIdentity')}
        className='min-w-0'
      >
        <div className='line-clamp-2 font-medium' title={accountLabel}>
          {accountLabel}
        </div>
        <div className='text-muted-foreground mt-0.5 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs'>
          {siteLabel && (
            <span className='max-w-full truncate' title={siteLabel}>
              @ {siteLabel}
            </span>
          )}
          <span
            className={cn(
              'max-w-full truncate',
              missingCredential && 'text-destructive font-medium'
            )}
            title={tokenHint}
          >
            {t('tokenRoutes.detail.channelToken')} {tokenLabel}
          </span>
          <span className='max-w-full truncate' title={sourceModel}>
            → <code className='font-mono'>{sourceModel}</code>
          </span>
        </div>
      </ChannelField>

      <div className='col-start-2 row-start-1 flex justify-end @[50rem]/channel-list:col-start-6'>
        <span
          data-testid='channel-status'
          className='inline-flex items-center gap-1.5 text-xs font-medium whitespace-nowrap'
          title={status.title}
        >
          <span className={cn('size-1.5 rounded-full', status.dot)} />
          {status.label}
        </span>
      </div>

      <div className='col-span-2 mt-1 grid grid-cols-2 gap-x-4 gap-y-1.5 @[34rem]/channel-list:grid-cols-[3.5rem_7rem_7rem_minmax(0,1fr)] @[50rem]/channel-list:contents'>
        <ChannelField
          label={t('tokenRoutes.detail.channelColumnPriority')}
          className='tabular-nums @[50rem]/channel-list:col-start-2'
        >
          {channel.priority}
        </ChannelField>

        <ChannelField
          label={t('tokenRoutes.detail.channelColumnWeightShare')}
          className='tabular-nums @[50rem]/channel-list:col-start-3'
        >
          {allocation.configuredWeight}
          <span className='text-muted-foreground'> · </span>
          {formatRouteWeightShare(allocation.enabledWeightShare)}
        </ChannelField>

        <ChannelField
          label={t('tokenRoutes.detail.channelColumnHealth')}
          className='tabular-nums @[50rem]/channel-list:col-start-4'
          valueTestId='channel-health'
        >
          <span>{formatInt(channel.successCount)}</span>
          <span className='text-muted-foreground'> / </span>
          <span className={channel.failCount > 0 ? 'text-destructive' : ''}>
            {formatInt(channel.failCount)}
          </span>
        </ChannelField>

        <ChannelField
          label={t('tokenRoutes.detail.channelColumnPrice')}
          className='@[50rem]/channel-list:col-start-5'
        >
          <ChannelPrice
            priceTruth={priceTruth}
            priceQueryState={priceQueryState}
          />
        </ChannelField>
      </div>
    </li>
  )
}

function ChannelField({
  label,
  className,
  valueTestId,
  children,
}: {
  label: string
  className?: string
  valueTestId?: string
  children: ReactNode
}) {
  return (
    <div className={cn('min-w-0', className)}>
      <div className='text-muted-foreground text-2xs @[50rem]/channel-list:sr-only'>
        {label}
      </div>
      <div
        data-testid={valueTestId}
        className='mt-0.5 @[50rem]/channel-list:mt-0'
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
  const priceUnavailable =
    priceQueryState?.hasError ||
    !priceTruth.price ||
    priceTruth.price.missingPrice ||
    !priceTruth.provenance.ratesSource ||
    ratesFallback
  const costSource = priceTruth.provenance.source
  const costSourceDiffers =
    Boolean(costSource) && costSource !== priceTruth.provenance.ratesSource

  return (
    <div className='flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1'>
      {priceQueryState?.isLoading ? (
        <span className='text-muted-foreground inline-flex items-center gap-1 text-xs'>
          <Spinner className='size-3' />
          {t('tokenRoutes.detail.channelPriceLoading')}
        </span>
      ) : priceUnavailable ? (
        <span className='text-muted-foreground text-xs'>
          {t('tokenRoutes.detail.channelPriceUnavailable')}
        </span>
      ) : (
        <span className='tabular-nums'>
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
          <span className='inline-flex items-center'>
            <span className='sr-only'>
              {t('tokenRoutes.detail.channelRateProvenance')}{' '}
            </span>
            <PriceGradeBadge grade={priceTruth.provenance.ratesSource} />
          </span>
        )}
      {!priceQueryState?.isLoading && costSourceDiffers && costSource && (
        <span className='inline-flex items-center'>
          <span className='sr-only'>
            {t('tokenRoutes.detail.channelCostProvenance')}{' '}
          </span>
          <PriceGradeBadge grade={costSource} />
        </span>
      )}
    </div>
  )
}
