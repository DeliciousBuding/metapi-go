import { Link } from '@tanstack/react-router'
import { ChevronDown, ChevronUp, FlaskConical } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { HttpStatusBadge } from '@/components/common/http-status-badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { routeCheckEndpointFromPath } from '@/features/token-routes'
import { toBcp47 } from '@/i18n/languages'
import { EM_DASH, formatCurrency, formatDateTime } from '@/lib/format'
import { buildModelTesterSearch } from '@/lib/helpers/model-tester-link'

import type { ProxyLog, ProxyLogDetail } from '../types'
import { TimingCell } from './timing-cell'
import { UsageCell } from './usage-cell'

export type DetailNavigation = {
  hasPrev?: boolean
  hasNext?: boolean
  onNavigatePrev?: () => void
  onNavigateNext?: () => void
}

export function DetailHeader({
  log,
  detail,
  hasPrev,
  hasNext,
  onNavigatePrev,
  onNavigateNext,
}: DetailNavigation & { log: ProxyLog; detail: ProxyLogDetail | null }) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const endpoint = routeCheckEndpointFromPath(detail?.downstreamPath)
  const reproSearch = useMemo(
    () =>
      buildModelTesterSearch({
        model: detail?.modelRequested,
        endpoint,
        channelId: detail?.channelId,
      }),
    [detail?.modelRequested, endpoint, detail?.channelId]
  )
  return (
    <SheetHeader>
      <div className='flex items-center justify-between gap-2 pr-8'>
        <SheetTitle className='flex min-w-0 items-center gap-2'>
          <span className='truncate'>
            {log.modelRequested || log.modelActual || `#${log.id}`}
          </span>
          <HttpStatusBadge
            status={log.status}
            httpStatus={detail?.httpStatus ?? null}
          />
        </SheetTitle>
        {(onNavigatePrev || onNavigateNext) && (
          <div className='flex shrink-0 items-center gap-1'>
            <Button
              type='button'
              size='icon-xs'
              variant='outline'
              aria-label={t('requestLogs.prevRecord')}
              disabled={!hasPrev}
              onClick={onNavigatePrev}
            >
              <ChevronUp className='size-3.5' />
            </Button>
            <Button
              type='button'
              size='icon-xs'
              variant='outline'
              aria-label={t('requestLogs.nextRecord')}
              disabled={!hasNext}
              onClick={onNavigateNext}
            >
              <ChevronDown className='size-3.5' />
            </Button>
          </div>
        )}
      </div>
      <SheetDescription className='truncate'>
        {t('proxyLogs.detail.description', {
          id: log.id,
          time: formatDateTime(log.createdAt, locale),
        })}
      </SheetDescription>
      {reproSearch && (
        <div className='pt-1'>
          <Link
            to='/model-tester'
            search={reproSearch}
            className={buttonVariants({ size: 'xs', variant: 'outline' })}
          >
            <FlaskConical className='size-3.5' aria-hidden='true' />
            {t('requestLogs.reproduceInTester')}
          </Link>
        </div>
      )}
    </SheetHeader>
  )
}

/** Count 0 renders as "0" (no retry happened); only a missing count is a dash. */
function formatRetryCount(retryCount: number | null | undefined): string {
  if (retryCount === null || retryCount === undefined) return EM_DASH
  if (retryCount <= 0) return '0'
  return `×${retryCount}`
}

export function DetailOverview({ detail }: { detail: ProxyLogDetail }) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const renderChannelIdentity = () => {
    if (detail.upstreamGrantId) {
      return t('proxyLogs.detail.directGrant', {
        channelId: detail.upstreamChannelId ?? '—',
        grantId: detail.upstreamGrantId,
      })
    }
    if (!detail.channelId) return '—'
    return (
      <Link
        to='/channels'
        search={{ channelId: detail.channelId }}
        className='text-primary hover:underline'
      >
        {`#${detail.channelId}`}
      </Link>
    )
  }
  let streamLabel = '—'
  if (detail.isStream != null) {
    streamLabel = t(
      detail.isStream ? 'proxyLogs.detail.yes' : 'proxyLogs.detail.no'
    )
  }
  return (
    <section>
      <h3 className='mb-2 text-sm font-medium'>
        {t('proxyLogs.detail.sectionOverview')}
      </h3>
      <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
        <DetailField label={t('proxyLogs.detail.createdAt')}>
          {formatDateTime(detail.createdAt, locale)}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.requestId')} full>
          <span className='font-mono text-xs break-all'>
            {detail.requestId || '—'}
          </span>
        </DetailField>
        <DetailField label={t('proxyLogs.detail.httpStatus')}>
          <HttpStatusBadge
            status={detail.status}
            httpStatus={detail.httpStatus ?? null}
          />
        </DetailField>
        <DetailField label={t('proxyLogs.detail.account')}>
          {detail.username || (detail.accountId ? `#${detail.accountId}` : '—')}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.site')}>
          {detail.siteName || (detail.siteId ? `#${detail.siteId}` : '—')}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.modelRequested')}>
          {detail.modelRequested || '—'}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.modelActual')}>
          {detail.modelActual || '—'}
        </DetailField>
        <DetailField
          label={t('proxyLogs.detail.upstreamReportedModel')}
          title={t('proxyLogs.detail.upstreamReportedModelHint')}
        >
          {detail.upstreamReportedModel || '—'}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.latency')}>
          <TimingCell
            latencyMs={detail.latencyMs}
            firstByteLatencyMs={detail.firstByteLatencyMs}
            firstOutputLatencyMs={detail.firstOutputLatencyMs}
            isStream={detail.isStream}
          />
        </DetailField>
        <DetailField label={t('proxyLogs.detail.isStream')}>
          {streamLabel}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.token')}>
          {detail.downstreamKeyName ||
            (detail.downstreamKeyId ? `#${detail.downstreamKeyId}` : '—')}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.retry')}>
          {formatRetryCount(detail.retryCount)}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.route')}>
          {detail.routeId ? (
            <Link
              to='/token-routes'
              search={{ routeId: detail.routeId }}
              className='text-primary hover:underline'
            >
              {`#${detail.routeId}`}
            </Link>
          ) : (
            '—'
          )}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.channel')}>
          {renderChannelIdentity()}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.estimatedCost')}>
          {formatCurrency(detail.estimatedCost, { fractionDigits: 6, locale })}
        </DetailField>
        <DetailField label={t('proxyLogs.columns.usage')}>
          <UsageCell log={detail} />
        </DetailField>
      </dl>
    </section>
  )
}
