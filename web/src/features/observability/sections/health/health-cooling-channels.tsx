import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ExpandableText } from '@/components/common/expandable-text'
import { Badge } from '@/components/ui/badge'
import { Empty, EmptyDescription, EmptyHeader } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatRemainingCooldown } from '@/hooks/use-cooldown-clock'
import { formatDateTime, formatInt } from '@/lib/format'

import type { CooldownChannel, MonitorHealthResponse } from '../../types'
import { FailedAttemptsLink } from './health-inspection-actions'
import { StatCell } from './health-metrics'

type Translate = (
  key: string,
  options?: Record<string, string | number>
) => string

const KNOWN_COOLDOWN_REASON_CODES = new Set([
  'usage_limit',
  'rate_limited',
  'auth_error',
  'upstream_error',
  'client_error',
  'timeout',
  'network_error',
  'probe_failure',
  'unknown',
])

function formatCooldownReasonCode(code: string, t: Translate): string {
  return KNOWN_COOLDOWN_REASON_CODES.has(code)
    ? t(`channels.reason.codes.${code}`)
    : code
}

export function HealthCoolingChannels({
  loading,
  cooldown,
  locale,
  now,
}: {
  loading: boolean
  cooldown?: MonitorHealthResponse['cooldown']
  locale: string
  now: number
}) {
  const { t } = useTranslation()
  if (loading) {
    return <Skeleton className='h-40 w-full rounded-md' />
  }
  return (
    <>
      <div className='mb-3 grid grid-cols-3 gap-2'>
        <StatCell
          label={t('observability.health.cooldown.channelsCooling')}
          value={formatInt(cooldown?.channelsCooling)}
          tone={cooldown?.channelsCooling ? 'warning' : 'success'}
        />
        <StatCell
          label={t('observability.health.cooldown.channelsWithFailures')}
          value={formatInt(cooldown?.channelsWithFailures)}
        />
        <StatCell
          label={t('observability.health.cooldown.channelsRecentlyFailed')}
          value={formatInt(cooldown?.channelsRecentlyFailed)}
        />
      </div>
      {renderCoolingTable(cooldown?.cooling ?? [], t, locale, now)}
    </>
  )
}

function renderCoolingTable(
  channels: CooldownChannel[],
  t: Translate,
  locale: string,
  now: number
) {
  if (channels.length === 0) {
    return (
      <Empty className='min-h-16 border'>
        <EmptyHeader>
          <EmptyDescription>
            {t('observability.health.cooldown.empty')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  const hasAnyReason = channels.some(
    (channel) =>
      Boolean(channel.cooldownReasonCode?.trim()) ||
      Boolean(channel.cooldownReason?.trim())
  )
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('observability.health.cooldown.colSite')}</TableHead>
          <TableHead className='text-right'>
            {t('observability.health.cooldown.colChannel')}
          </TableHead>
          {hasAnyReason && (
            <TableHead>
              {t('observability.health.cooldown.colReason')}
            </TableHead>
          )}
          <TableHead className='text-right'>
            {t('observability.health.cooldown.colFailCount')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.health.cooldown.colUntil')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.health.cooldown.colActions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {channels.map((channel) => {
          const remaining = formatRemainingCooldown(channel.cooldownUntil, now)
          const reasonCode = channel.cooldownReasonCode?.trim() || null
          const reasonText = channel.cooldownReason?.trim() || null
          return (
            <TableRow key={channel.channelId}>
              <TableCell className='truncate'>
                {channel.siteName || `#${channel.siteId}`}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                <Link
                  to='/channels'
                  search={{ channelId: channel.channelId }}
                  className='text-primary hover:underline'
                >
                  #{channel.channelId}
                </Link>
              </TableCell>
              {hasAnyReason && (
                <TableCell className='max-w-56 align-top'>
                  {reasonCode || reasonText ? (
                    <div className='flex min-w-0 flex-col gap-1'>
                      {reasonCode && (
                        <Badge
                          variant='outline'
                          className='w-fit font-normal'
                          title={reasonCode}
                        >
                          {formatCooldownReasonCode(reasonCode, t)}
                        </Badge>
                      )}
                      {reasonText && (
                        <ExpandableText
                          text={reasonText}
                          threshold={72}
                          className='text-muted-foreground text-xs leading-snug'
                        />
                      )}
                    </div>
                  ) : null}
                </TableCell>
              )}
              <TableCell className='text-right tabular-nums'>
                {channel.failCount}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                <div className='flex flex-col items-end gap-0.5'>
                  <span>{formatDateTime(channel.cooldownUntil, locale)}</span>
                  {remaining && (
                    <span className='text-warning-soft-fg text-2xs'>
                      {t('observability.health.cooldown.remaining', {
                        remaining,
                      })}
                    </span>
                  )}
                </div>
              </TableCell>
              <TableCell className='text-right'>
                <FailedAttemptsLink
                  filters={{ channelId: channel.channelId }}
                />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
