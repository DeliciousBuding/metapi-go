import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { ClientPill } from '@/components/common/client-pill'
import { DetailField } from '@/components/common/detail-field'
import { Separator } from '@/components/ui/separator'
import { routeCheckEndpointFromPath } from '@/features/token-routes'
import type { ProxyLogAttempt } from '@/lib/api'
import { parseProxyLogPathMeta } from '@/lib/helpers/proxyLogPathMeta'

import type { ProxyLogDetail } from '../types'
import { CurrentRouteDiagnosis } from './current-route-diagnosis'
import { AttemptsSection } from './proxy-log-attempts-section'
import { RelayBridgeTraceCard } from './relay-bridge-trace'
import { StreamTimingCard } from './stream-timing-card'

export function DetailDiagnostics({
  detail,
  onViewAttempt,
}: {
  detail: ProxyLogDetail
  onViewAttempt?: (attempt: ProxyLogAttempt) => void
}) {
  const pathMeta = useMemo(
    () => parseProxyLogPathMeta(detail.errorMessage ?? undefined),
    [detail.errorMessage]
  )
  return (
    <>
      {detail.streamTiming ? (
        <>
          <Separator />
          <StreamTimingCard
            streamTiming={detail.streamTiming}
            latencyMs={detail.latencyMs}
          />
        </>
      ) : null}
      {detail.conversion ? (
        <>
          <Separator />
          <RelayBridgeTraceCard conversion={detail.conversion} />
        </>
      ) : null}
      <CurrentRouteDiagnosis
        model={detail.modelRequested || null}
        endpoint={routeCheckEndpointFromPath(detail.downstreamPath)}
        keyId={detail.downstreamKeyId ?? null}
      />
      <ConversationFileLinks
        downstreamPath={detail.downstreamPath ?? null}
        upstreamPath={pathMeta.upstreamPath}
        clientFamily={pathMeta.clientFamily}
        clientAppName={detail.clientAppName ?? null}
        sessionId={pathMeta.sessionId}
        usageSource={pathMeta.usageSource}
      />
      <AttemptsSection detail={detail} onViewAttempt={onViewAttempt} />
      <ErrorSection errorMessage={pathMeta.errorMessage} />
    </>
  )
}

function ConversationFileLinks({
  downstreamPath,
  upstreamPath,
  clientFamily,
  clientAppName,
  sessionId,
  usageSource,
}: {
  downstreamPath: string | null
  upstreamPath: string | null
  clientFamily: string | null
  clientAppName: string | null
  sessionId: string | null
  usageSource: string | null
}) {
  const { t } = useTranslation()
  if (
    !downstreamPath &&
    !upstreamPath &&
    !clientFamily &&
    !clientAppName &&
    !sessionId
  ) {
    return null
  }
  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>
          {t('proxyLogs.detail.sectionConversation')}
        </h3>
        <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
          <DetailField label={t('proxyLogs.detail.client')}>
            {clientFamily || clientAppName ? (
              <ClientPill appName={clientAppName} family={clientFamily} />
            ) : (
              '—'
            )}
          </DetailField>
          <DetailField label={t('proxyLogs.detail.sessionId')}>
            {sessionId || '—'}
          </DetailField>
          <DetailField
            label={t('proxyLogs.detail.downstreamPath')}
            full
            title={downstreamPath || undefined}
          >
            {downstreamPath ? (
              <code className='bg-muted rounded px-1.5 py-0.5 text-xs'>
                {downstreamPath}
              </code>
            ) : (
              '—'
            )}
          </DetailField>
          <DetailField
            label={t('proxyLogs.detail.upstreamPath')}
            full
            title={upstreamPath || undefined}
          >
            {upstreamPath ? (
              <code className='bg-muted rounded px-1.5 py-0.5 text-xs'>
                {upstreamPath}
              </code>
            ) : (
              '—'
            )}
          </DetailField>
          <DetailField label={t('proxyLogs.detail.usageSource')}>
            {usageSource || '—'}
          </DetailField>
        </dl>
      </section>
    </>
  )
}

function ErrorSection({ errorMessage }: { errorMessage: string }) {
  const { t } = useTranslation()
  if (!errorMessage) return null
  return (
    <>
      <Separator />
      <section>
        <h3 className='text-destructive mb-2 text-sm font-medium'>
          {t('proxyLogs.detail.sectionError')}
        </h3>
        <pre className='border-destructive/30 bg-destructive/5 text-destructive-soft-fg max-h-60 overflow-auto rounded border p-2 text-xs break-all whitespace-pre-wrap'>
          {errorMessage}
        </pre>
      </section>
    </>
  )
}
