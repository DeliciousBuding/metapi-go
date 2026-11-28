import {
  Check,
  CircleAlert,
  CircleCheck,
  CircleDollarSign,
  CirclePause,
  Gauge,
  KeyRound,
  Network,
  Server,
  Snowflake,
  X,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { Badge } from '@/components/ui/badge'
import { normalizePriceGrade } from '@/features/models'
import { formatRemainingCooldown } from '@/hooks/use-cooldown-clock'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime, formatInt, formatPrice } from '@/lib/format'
import { getPlatformDefinition } from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

import {
  formatRouteWeightShare,
  type RouteChannelAllocation,
  type RouteChannelPriceTruth,
} from '../lib/route-price-truth'
import type { RouteChannel } from '../types'

function hasAccountCredential(channel: RouteChannel): boolean {
  return Boolean(
    channel.account?.accessTokenMasked || channel.account?.apiTokenMasked
  )
}

export function resolveChannelState(channel: RouteChannel): ChannelState {
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
  remaining,
}: {
  state: ChannelState
  title?: string
  remaining?: string | null
}) {
  const { t } = useTranslation()
  const label = t(CHANNEL_STATUS_LABEL_KEY[state])
  return (
    <Badge
      data-testid='channel-status'
      variant={CHANNEL_STATUS_BADGE_VARIANT[state]}
      title={title ?? label}
      className='gap-1 tabular-nums'
    >
      <ChannelStatusIcon state={state} />
      <span>{label}</span>
      {state === 'cooling' && remaining ? (
        <span className='font-mono'>({remaining})</span>
      ) : null}
    </Badge>
  )
}

export function RouteChannelRow({
  channel,
  state,
  allocation,
  priceTruth,
  showSourceModel,
  priceUnavailable,
  cooldownNow,
}: {
  channel: RouteChannel
  state: ChannelState
  allocation: RouteChannelAllocation
  priceTruth: RouteChannelPriceTruth
  showSourceModel: boolean
  priceUnavailable: boolean
  cooldownNow: number
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

        <ChannelStatusBadge
          state={state}
          title={statusTitle}
          remaining={
            state === 'cooling'
              ? formatRemainingCooldown(channel.cooldownUntil, cooldownNow)
              : null
          }
        />
      </div>
    </li>
  )
}
