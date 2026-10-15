// metapi-go/features/proxy-logs/components — proxy log detail Sheet.
// i18n: all user-visible strings migrated to t() calls.

import { Link } from '@tanstack/react-router'
import { Copy as CopyIcon, Check as CheckIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ClientPill } from '@/components/common/client-pill'
import { DetailField } from '@/components/common/detail-field'
import { HttpStatusBadge } from '@/components/common/http-status-badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { routeCheckEndpointFromPath } from '@/features/token-routes'
import { toBcp47 } from '@/i18n/languages'
import type { ProxyLogAttempt } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import {
  EM_DASH,
  formatCurrency,
  formatDateTime,
  formatInt,
} from '@/lib/format'
import { parseProxyLogPathMeta } from '@/lib/helpers/proxyLogPathMeta'
import { toast } from '@/lib/toast'

import { useProxyLog } from '../api'
import type { ProxyLog, ProxyLogBillingDetails, ProxyLogDetail } from '../types'
import { TimingCell } from './timing-cell'
import { UsageCell } from './usage-cell'

/** Count 0 renders as "0" (no retry happened); only a missing count is a dash. */
function formatRetryCount(retryCount: number | null | undefined): string {
  if (retryCount === null || retryCount === undefined) return EM_DASH
  if (retryCount <= 0) return '0'
  return `×${retryCount}`
}

type ProxyLogDetailSheetProps = {
  log: ProxyLog | null
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Switch the sheet to another attempt of the same request. */
  onViewAttempt?: (attempt: ProxyLogAttempt) => void
}

export function ProxyLogDetailSheet({
  log,
  open,
  onOpenChange,
  onViewAttempt,
}: ProxyLogDetailSheetProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const detailQuery = useProxyLog(log?.id ?? null)
  const detail: ProxyLogDetail | null =
    detailQuery.data ?? (log ? ({ ...log } as unknown as ProxyLogDetail) : null)
  const pathMeta = useMemo(
    () => parseProxyLogPathMeta(detail?.errorMessage ?? undefined),
    [detail?.errorMessage]
  )

  if (!log) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' className='sm:max-w-lg' />
      </Sheet>
    )
  }

  const isDetailLoading = detailQuery.isLoading && detailQuery.isFetching

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-lg'
      >
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2 pr-6'>
            <span className='truncate'>
              {log.modelRequested || log.modelActual || `#${log.id}`}
            </span>
            <HttpStatusBadge
              status={log.status}
              httpStatus={detail?.httpStatus ?? null}
            />
          </SheetTitle>
          <SheetDescription className='truncate'>
            {t('proxyLogs.detail.description', {
              id: log.id,
              time: formatDateTime(log.createdAt, locale),
            })}
          </SheetDescription>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          {isDetailLoading && (
            <div className='text-muted-foreground flex items-center justify-center gap-2 py-6 text-sm'>
              <Spinner className='size-4' />
              {t('proxyLogs.detail.loading')}
            </div>
          )}
          {detail && (
            <>
              <DetailOverview detail={detail} />
              <CurrentRouteDiagnosis
                model={detail.modelRequested || null}
                endpoint={routeCheckEndpointFromPath(pathMeta.downstreamPath)}
                keyId={detail.downstreamKeyId ?? null}
              />
              <ConversationFileLinks
                downstreamPath={pathMeta.downstreamPath}
                upstreamPath={pathMeta.upstreamPath}
                clientFamily={pathMeta.clientFamily}
                clientAppName={detail.clientAppName ?? null}
                sessionId={pathMeta.sessionId}
                usageSource={pathMeta.usageSource}
              />
              <AttemptsSection detail={detail} onViewAttempt={onViewAttempt} />
              <ErrorSection errorMessage={pathMeta.errorMessage} />
              {detail.billingDetails ? (
                <BillingSection billing={detail.billingDetails} />
              ) : null}
              <RawBodySection
                title={t('proxyLogs.detail.rawRequest')}
                body={detail.requestBody}
              />
              <RawBodySection
                title={t('proxyLogs.detail.rawResponse')}
                body={detail.responseBody}
              />
              <RawBodySection
                title={t('proxyLogs.detail.rawRequestHeaders')}
                body={detail.requestHeaders}
              />
              <RawBodySection
                title={t('proxyLogs.detail.rawResponseHeaders')}
                body={detail.responseHeaders}
              />
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function DetailOverview({ detail }: { detail: ProxyLogDetail }) {
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

function CurrentRouteDiagnosis({
  model,
  endpoint,
  keyId,
}: {
  model: string | null
  endpoint: string | null
  keyId: number | null
}) {
  const { t } = useTranslation()
  if (!model || !endpoint) return null
  return (
    <>
      <Separator />
      <section className='flex flex-wrap items-center justify-between gap-2'>
        <div className='min-w-0'>
          <h3 className='text-sm font-medium'>
            {t('proxyLogs.detail.currentRoute.title')}
          </h3>
          <p className='text-muted-foreground text-xs'>
            {keyId
              ? t('proxyLogs.detail.currentRoute.keyHint')
              : t('proxyLogs.detail.currentRoute.adminHint')}
          </p>
        </div>
        <Button
          size='sm'
          variant='outline'
          render={
            <Link
              to='/token-routes'
              search={{
                workspaceModel: model,
                workspaceEndpoint: endpoint,
                ...(keyId ? { workspaceKeyId: keyId } : {}),
              }}
            />
          }
        >
          {t('proxyLogs.detail.currentRoute.open')}
        </Button>
      </section>
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

/**
 * Request-level attempt view (route-decision D batch). Every dispatch attempt
 * persists its own log row; the backend correlates siblings by requestId and
 * reports ordinal gaps / missing correlation honestly via partialReasons.
 */
function AttemptsSection({
  detail,
  onViewAttempt,
}: {
  detail: ProxyLogDetail
  onViewAttempt?: (attempt: ProxyLogAttempt) => void
}) {
  const { t } = useTranslation()
  const ra = detail.requestAttempts
  if (!ra) return null

  const partialReasonText = (reason: string): string => {
    switch (reason) {
      case 'ordinalGap':
        return t('proxyLogs.detail.attempts.partialOrdinalGap')
      case 'requestIdMissing':
        return t('proxyLogs.detail.attempts.partialRequestIdMissing')
      case 'attemptQueryFailed':
        return t('proxyLogs.detail.attempts.partialAttemptQueryFailed')
      default:
        return reason
    }
  }

  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>
          {t('proxyLogs.detail.attempts.title')}
        </h3>
        <div className='space-y-2 text-sm'>
          <div className='flex flex-wrap items-center gap-x-3 gap-y-1'>
            <span>
              {t('proxyLogs.detail.attempts.summary', {
                count: Math.max(ra.attempts.length, 1),
              })}
            </span>
            {ra.terminal ? (
              <span className='text-muted-foreground inline-flex items-center gap-1'>
                {t('proxyLogs.detail.attempts.terminal')}
                <HttpStatusBadge
                  status={ra.terminal.status}
                  httpStatus={ra.terminal.httpStatus}
                />
              </span>
            ) : null}
            {ra.partial ? (
              <span className='text-warning-soft-fg bg-warning/10 inline-flex items-center rounded px-1.5 py-0.5 text-xs'>
                {t('proxyLogs.detail.attempts.partial')}
              </span>
            ) : null}
          </div>
          {ra.partial && ra.partialReasons.length > 0 ? (
            <ul className='text-muted-foreground list-disc space-y-1 pl-5 text-xs'>
              {ra.partialReasons.map((reason) => (
                <li key={reason}>{partialReasonText(reason)}</li>
              ))}
            </ul>
          ) : null}
          {ra.attempts.length > 0 ? (
            <ol className='divide-border divide-y rounded-md border'>
              {ra.attempts.map((attempt) => (
                <li key={attempt.id}>
                  <button
                    type='button'
                    disabled={attempt.isCurrent || !onViewAttempt}
                    onClick={() => onViewAttempt?.(attempt)}
                    className='hover:bg-muted/50 flex w-full flex-wrap items-center gap-x-2 gap-y-0.5 px-2 py-1.5 text-left text-xs disabled:cursor-default disabled:hover:bg-transparent'
                  >
                    <span className='text-muted-foreground font-mono'>
                      {`#${attempt.ordinal + 1}`}
                    </span>
                    <HttpStatusBadge
                      status={attempt.status}
                      httpStatus={attempt.httpStatus}
                    />
                    <span className='min-w-0 flex-1 truncate'>
                      {attempt.siteName ||
                        attempt.accountUsername ||
                        attempt.modelActual ||
                        EM_DASH}
                    </span>
                    <span className='text-muted-foreground'>
                      {formatInt(attempt.latencyMs)} ms
                    </span>
                    {attempt.isCurrent ? (
                      <span className='text-primary'>
                        {t('proxyLogs.detail.attempts.current')}
                      </span>
                    ) : null}
                  </button>
                  {attempt.errorMessage ? (
                    <p className='text-destructive-soft-fg truncate px-2 pb-1.5 text-xs'>
                      {attempt.errorMessage}
                    </p>
                  ) : null}
                </li>
              ))}
            </ol>
          ) : (
            <p className='text-muted-foreground text-xs'>
              {t('proxyLogs.detail.attempts.uncorrelated')}
            </p>
          )}
        </div>
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

function BillingSection({ billing }: { billing: ProxyLogBillingDetails }) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  if (!billing) return null
  const usage = billing.usage
  const breakdown = billing.breakdown
  const rows =
    usage &&
    breakdown &&
    (typeof usage.promptTokens === 'number' ||
      typeof usage.billablePromptTokens === 'number') &&
    typeof breakdown.inputCost === 'number'
      ? [
          {
            label: t(
              typeof usage.billablePromptTokens === 'number'
                ? 'proxyLogs.usage.billableInput'
                : 'proxyLogs.usage.input'
            ),
            tokens: usage.billablePromptTokens ?? usage.promptTokens,
            cost: breakdown.inputCost,
          },
          {
            label: t('proxyLogs.usage.output'),
            tokens: usage.completionTokens,
            cost: breakdown.outputCost,
          },
          {
            label: t('proxyLogs.usage.cacheRead'),
            tokens: usage.cacheReadTokens,
            cost: breakdown.cacheReadCost,
          },
          {
            label: t('proxyLogs.usage.cacheCreation'),
            tokens: usage.cacheCreationTokens,
            cost: breakdown.cacheCreationCost,
          },
        ]
      : []
  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>
          {t('proxyLogs.detail.sectionBilling')}
        </h3>
        {rows.length > 0 ? (
          <>
            <dl className='divide-border divide-y rounded-md border px-3'>
              {rows.map((row) => (
                <div
                  key={row.label}
                  className='grid grid-cols-[1fr_auto_auto] items-baseline gap-3 py-2 text-xs'
                >
                  <dt className='text-muted-foreground'>{row.label}</dt>
                  <dd className='tabular-nums'>
                    {formatInt(row.tokens, locale)}
                  </dd>
                  <dd className='tabular-nums'>
                    {formatCurrency(row.cost, { fractionDigits: 6, locale })}
                  </dd>
                </div>
              ))}
              <div className='flex justify-between gap-3 py-2 text-sm font-medium'>
                <dt>{t('proxyLogs.usage.totalCost')}</dt>
                <dd className='tabular-nums'>
                  {formatCurrency(breakdown.totalCost, {
                    fractionDigits: 6,
                    locale,
                  })}
                </dd>
              </div>
            </dl>
            <details className='mt-3 text-xs'>
              <summary className='text-muted-foreground cursor-pointer py-1'>
                {t('proxyLogs.usage.rawBilling')}
              </summary>
              <JsonBlock value={billing} />
            </details>
          </>
        ) : (
          <JsonBlock value={billing} />
        )}
      </section>
    </>
  )
}

function RawBodySection({
  title,
  body,
}: {
  title: string
  body?: string | null
}) {
  if (!body || !body.trim()) return null
  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>{title}</h3>
        <JsonBlock value={body} />
      </section>
    </>
  )
}

function prettyPrintJson(value: unknown): string {
  if (typeof value === 'string') {
    const trimmed = value.trim()
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      return trimmed
    }
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function JsonBlock({ value }: { value: unknown }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const text = useMemo(() => prettyPrintJson(value), [value])
  async function handleCopy() {
    if (await copyText(text)) {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } else {
      // Clipboard may be unavailable (non-secure context / permissions).
      toast.error(t('common.copyFailed'))
    }
  }
  return (
    <div className='relative'>
      <Button
        variant='ghost'
        size='icon-sm'
        onClick={handleCopy}
        className='absolute top-1 right-1'
        aria-label={t('proxyLogs.detail.copy')}
      >
        {copied ? (
          <CheckIcon className='size-3.5' />
        ) : (
          <CopyIcon className='size-3.5' />
        )}
      </Button>
      <pre className='bg-muted max-h-80 overflow-auto rounded p-2 pr-9 text-xs leading-relaxed break-all whitespace-pre-wrap'>
        {text}
      </pre>
    </div>
  )
}
