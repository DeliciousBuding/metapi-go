/* eslint-disable no-nested-ternary -- status labels reflect channel and credential state */
import { useQueries } from '@tanstack/react-query'
import { useMemo } from 'react'
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
import { formatInt, formatPrice } from '@/lib/format'

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

export function RouteChannelList({ route }: { route: RouteSummaryRow }) {
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

  return (
    <div className='@container/channel-list min-w-0 space-y-2'>
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>
          {t('tokenRoutes.detail.channels')}
          <span className='text-muted-foreground ms-1.5 font-normal tabular-nums'>
            {channels.length}
          </span>
        </h3>
        {(channelsQuery.isFetching ||
          priceQueries.some((query) => query.isFetching)) && (
          <Spinner
            className='text-muted-foreground size-3.5'
            aria-label={t('common.loading')}
          />
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
        <p role='status' className='text-muted-foreground py-4 text-sm'>
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
        <ul className='divide-y rounded-lg border'>
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
  const { t } = useTranslation()
  const accountLabel =
    channel.account?.username ||
    t('tokenRoutes.detail.fallbackAccount', { id: channel.accountId })
  const siteLabel = channel.site?.name || channel.site?.platform || ''
  const tokenBound = Boolean(channel.token?.name || channel.tokenId)
  // An account-scoped channel (no token_id) is NOT unbound: the selector relays
  // it with the account's own credential — the access token for an OAuth
  // account, the API key otherwise (routing.resolveChannelTokenValue). Route
  // rebuild deliberately creates both flavours for the same account and model,
  // so calling the account-scoped one "Unbound" labelled a working channel as
  // broken and sent operators hunting for a binding step that does not exist.
  // Only when the wire reports neither masked credential is the channel really
  // unable to serve, and that is the one case worth shouting about.
  const accountCredential = Boolean(
    channel.account?.accessTokenMasked || channel.account?.apiTokenMasked
  )
  const tokenLabel =
    channel.token?.name ||
    (channel.tokenId
      ? t('tokenRoutes.detail.fallbackToken', { id: channel.tokenId })
      : accountCredential
        ? t('tokenRoutes.detail.channelTokenAccount')
        : t('tokenRoutes.detail.channelTokenMissing'))
  const sourceModel = priceTruth.concreteModel || '—'
  const cooldownActive =
    Boolean(channel.cooldownUntil) &&
    new Date(channel.cooldownUntil as string) > new Date()
  const ratesFallback =
    normalizePriceGrade(priceTruth.provenance.ratesSource) === 'fallback'
  const priceUnavailable =
    priceQueryState?.hasError ||
    !priceTruth.price ||
    priceTruth.price.missingPrice ||
    !priceTruth.provenance.ratesSource ||
    ratesFallback
  const costSourceDiffers =
    priceTruth.provenance.source &&
    priceTruth.provenance.source !== priceTruth.provenance.ratesSource
  return (
    <li className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 px-3 py-2 text-sm @[54rem]/channel-list:grid-cols-[minmax(0,1.8fr)_minmax(0,0.9fr)_minmax(0,1.2fr)_auto] @[54rem]/channel-list:items-center'>
      <div className='min-w-0'>
        <div className='font-medium [overflow-wrap:anywhere]'>
          {accountLabel}
        </div>
        <div className='text-muted-foreground mt-0.5 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs'>
          {siteLabel && (
            <span className='max-w-full truncate' title={siteLabel}>
              @ {siteLabel}
            </span>
          )}
          <span
            className={
              !tokenBound && !accountCredential
                ? 'text-destructive font-medium'
                : undefined
            }
            title={
              tokenBound
                ? undefined
                : accountCredential
                  ? t('tokenRoutes.detail.channelTokenAccountHint')
                  : t('tokenRoutes.detail.channelTokenMissingHint')
            }
          >
            {t('tokenRoutes.detail.channelToken')}: {tokenLabel}
          </span>
          <span className='max-w-full truncate' title={sourceModel}>
            {t('tokenRoutes.detail.channelUpstream')} {sourceModel}
          </span>
          <span className='shrink-0'>
            {t('tokenRoutes.detail.channelPriority')} {channel.priority}
          </span>
        </div>
      </div>

      <div className='text-muted-foreground col-span-2 flex flex-wrap gap-x-3 gap-y-0.5 text-xs tabular-nums @[54rem]/channel-list:col-span-1 @[54rem]/channel-list:col-start-2 @[54rem]/channel-list:row-start-1'>
        <span className='inline-flex items-baseline gap-1'>
          <span>{t('tokenRoutes.detail.channelConfiguredWeight')}</span>
          <strong className='text-foreground font-medium'>
            {allocation.configuredWeight}
          </strong>
        </span>
        <span className='inline-flex items-baseline gap-1'>
          <span>{t('tokenRoutes.detail.channelEnabledShare')}</span>
          <strong className='text-foreground font-medium'>
            {formatRouteWeightShare(allocation.enabledWeightShare)}
          </strong>
        </span>
      </div>

      <div className='text-muted-foreground col-span-2 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5 text-xs tabular-nums @[54rem]/channel-list:col-span-1 @[54rem]/channel-list:col-start-3 @[54rem]/channel-list:row-start-1'>
        <span className='inline-flex items-baseline gap-1'>
          <span>{t('tokenRoutes.detail.channelHits')}</span>
          <span className='text-foreground'>
            {formatInt(channel.successCount)}
            <span className='text-muted-foreground'> / </span>
            <span className={channel.failCount > 0 ? 'text-destructive' : ''}>
              {formatInt(channel.failCount)}
            </span>
          </span>
        </span>
        {priceQueryState?.isLoading ? (
          <span className='inline-flex items-center gap-1'>
            <Spinner className='size-3' />
            {t('tokenRoutes.detail.channelPriceLoading')}
          </span>
        ) : priceUnavailable ? (
          <span>{t('tokenRoutes.detail.channelPriceUnavailable')}</span>
        ) : (
          <span className='text-foreground'>
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
          !priceUnavailable &&
          priceTruth.provenance.ratesSource && (
            <span className='inline-flex items-center gap-1'>
              <span>{t('tokenRoutes.detail.channelRateProvenance')}</span>
              <PriceGradeBadge grade={priceTruth.provenance.ratesSource} />
            </span>
          )}
        {costSourceDiffers && priceTruth.provenance.source && (
          <span className='inline-flex items-center gap-1'>
            <span>{t('tokenRoutes.detail.channelCostProvenance')}</span>
            <PriceGradeBadge grade={priceTruth.provenance.source} />
          </span>
        )}
      </div>

      <div className='col-start-2 row-start-1 flex items-center gap-1.5 @[54rem]/channel-list:col-start-4'>
        {cooldownActive && (
          <Badge variant='warning'>{t('tokenRoutes.detail.cooldown')}</Badge>
        )}
        <Badge variant={channel.enabled ? 'success' : 'secondary'}>
          {channel.enabled
            ? t('tokenRoutes.columns.enable')
            : t('tokenRoutes.columns.disable')}
        </Badge>
      </div>
    </li>
  )
}
