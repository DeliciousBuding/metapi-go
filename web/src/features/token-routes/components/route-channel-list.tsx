/* eslint-disable no-nested-ternary -- status labels reflect channel and credential state */
import { useQueries } from '@tanstack/react-query'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
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
    <div className='min-w-0 space-y-3'>
      <div className='flex flex-wrap items-start justify-between gap-2'>
        <div>
          <h3 className='text-sm font-medium'>
            {t('tokenRoutes.detail.channelTruthList')}
          </h3>
          <p className='text-muted-foreground text-xs'>
            {t('tokenRoutes.detail.channelTruthDescription')}
          </p>
        </div>
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
        <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-center text-sm'>
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
  return (
    <li className='min-w-0 p-3 text-sm'>
      <div className='flex items-start justify-between gap-2'>
        <div className='min-w-0 flex-1'>
          <div className='flex flex-wrap items-center gap-x-1.5 gap-y-0.5'>
            <span className='font-medium [overflow-wrap:anywhere] break-words'>
              {accountLabel}
            </span>
            {siteLabel && (
              <span className='text-muted-foreground [overflow-wrap:anywhere] break-words'>
                @ {siteLabel}
              </span>
            )}
          </div>
          <div className='text-muted-foreground mt-0.5 flex flex-wrap items-center gap-1.5 text-xs [overflow-wrap:anywhere]'>
            <span
              // The label names the credential this channel relays with; the
              // title carries the mechanism and, for the credential-less case,
              // the action that fixes it (on the account, not the channel).
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
            <span className='text-muted-foreground block text-xs'>
              {channel.enabled
                ? t('tokenRoutes.detail.channelShareUnavailable')
                : t('tokenRoutes.detail.channelShareExcluded')}
            </span>
          )}
        </ChannelMetric>
        <ChannelMetric label={t('tokenRoutes.detail.channelHits')}>
          <span className='tabular-nums'>
            <span>{formatInt(channel.successCount)}</span>
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
      <dt className='text-muted-foreground text-xs'>{label}</dt>
      <dd className='font-medium'>{children}</dd>
    </div>
  )
}

function PriceValue({ value }: { value: number | null }) {
  return (
    <span className='tabular-nums'>
      {formatPrice(value, { fractionDigits: 4 })}
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
      {ratesSource === 'fallback' ? (
        <span className='text-muted-foreground'>
          — · {t('tokenRoutes.detail.channelPriceUnavailable')}
        </span>
      ) : (
        <PriceGradeBadge grade={ratesSource} />
      )}
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
