// metapi-go/features/proxy-logs/components — proxy log detail Sheet.
// i18n: all user-visible strings migrated to t() calls.

import { Link } from '@tanstack/react-router'
import { Copy as CopyIcon, Check as CheckIcon } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

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
import { toBcp47 } from '@/i18n/languages'
import { EM_DASH, formatCurrency, formatDateTime } from '@/lib/format'
import { parseProxyLogPathMeta } from '@/lib/helpers/proxyLogPathMeta'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

import { useProxyLog } from '../api'
import type { ProxyLog, ProxyLogBillingDetails, ProxyLogDetail } from '../types'
import { LatencyBadge } from './latency-badge'
import { StatusBadge } from './status-badge'

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
}

export function ProxyLogDetailSheet({
  log,
  open,
  onOpenChange,
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
            <StatusBadge
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
              <ConversationFileLinks
                downstreamPath={pathMeta.downstreamPath}
                upstreamPath={pathMeta.upstreamPath}
                clientFamily={pathMeta.clientFamily}
                sessionId={pathMeta.sessionId}
                usageSource={pathMeta.usageSource}
              />
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
  return (
    <section>
      <h3 className='mb-2 text-sm font-medium'>
        {t('proxyLogs.detail.sectionOverview')}
      </h3>
      <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
        <DetailField label={t('proxyLogs.detail.createdAt')}>
          {formatDateTime(detail.createdAt, locale)}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.httpStatus')}>
          <StatusBadge
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
        <DetailField label={t('proxyLogs.detail.latency')}>
          <LatencyBadge
            latencyMs={detail.latencyMs}
            firstByteLatencyMs={detail.firstByteLatencyMs}
            showDot
          />
        </DetailField>
        <DetailField label={t('proxyLogs.detail.isStream')}>
          {detail.isStream
            ? t('proxyLogs.detail.yes')
            : t('proxyLogs.detail.no')}
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
          {detail.channelId ? (
            <Link
              to='/channels'
              search={{ channelId: detail.channelId }}
              className='text-primary hover:underline'
            >
              {`#${detail.channelId}`}
            </Link>
          ) : (
            '—'
          )}
        </DetailField>
        <DetailField label={t('proxyLogs.detail.estimatedCost')}>
          {formatCurrency(detail.estimatedCost, { fractionDigits: 4 })}
        </DetailField>
      </dl>
    </section>
  )
}

function ConversationFileLinks({
  downstreamPath,
  upstreamPath,
  clientFamily,
  sessionId,
  usageSource,
}: {
  downstreamPath: string | null
  upstreamPath: string | null
  clientFamily: string | null
  sessionId: string | null
  usageSource: string | null
}) {
  const { t } = useTranslation()
  if (!downstreamPath && !upstreamPath && !clientFamily && !sessionId) {
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
            {clientFamily || '—'}
          </DetailField>
          <DetailField label={t('proxyLogs.detail.sessionId')}>
            {sessionId || '—'}
          </DetailField>
          <DetailField label={t('proxyLogs.detail.downstreamPath')} full>
            {downstreamPath ? (
              <code className='bg-muted block rounded px-1.5 py-0.5 text-xs break-all'>
                {downstreamPath}
              </code>
            ) : (
              '—'
            )}
          </DetailField>
          <DetailField label={t('proxyLogs.detail.upstreamPath')} full>
            {upstreamPath ? (
              <code className='bg-muted block rounded px-1.5 py-0.5 text-xs break-all'>
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
        <pre className='border-destructive/30 bg-destructive/5 text-destructive max-h-60 overflow-auto rounded border p-2 text-xs break-all whitespace-pre-wrap'>
          {errorMessage}
        </pre>
      </section>
    </>
  )
}

function BillingSection({ billing }: { billing: ProxyLogBillingDetails }) {
  const { t } = useTranslation()
  if (!billing) return null
  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>
          {t('proxyLogs.detail.sectionBilling')}
        </h3>
        <JsonBlock value={billing} />
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

function DetailField({
  label,
  children,
  full,
}: {
  label: string
  children: ReactNode
  full?: boolean
}) {
  return (
    <div className={cn('flex flex-col', full && 'col-span-2')}>
      <dt className='text-muted-foreground text-[11px]'>{label}</dt>
      <dd className='min-w-0 break-words'>{children}</dd>
    </div>
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
    try {
      if (
        typeof navigator === 'undefined' ||
        !navigator.clipboard?.writeText
      ) {
        throw new Error('clipboard unavailable')
      }
      await navigator.clipboard.writeText(text)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
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
