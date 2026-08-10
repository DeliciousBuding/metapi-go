// metapi-go/features/proxy-logs/components — proxy log detail Sheet.
//
// Opens from the row "view details" action. Fetches the full detail payload
// (billing / route / channel / http status) via `useProxyLog` and layers it
// over the list row. The conversation path metadata (client family / session
// id / downstream & upstream paths / usage source) is parsed from the log's
// `errorMessage` via the shared `parseProxyLogPathMeta` helper so the same
// brick that decodes the legacy message format powers the conversation-file
// link display. Raw request/response bodies and headers are rendered as
// pretty-printed JSON blocks when the backend surfaces them (forward-compat
// optional fields on `ProxyLogDetail`).

import {
  Copy as CopyIcon,
  Check as CheckIcon,
} from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'

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
import { parseProxyLogPathMeta } from '@/lib/helpers/proxyLogPathMeta'
import { cn } from '@/lib/utils'

import { useProxyLog } from '../api'
import type { ProxyLog, ProxyLogBillingDetails, ProxyLogDetail } from '../types'
import { LatencyBadge } from './latency-badge'
import { StatusBadge } from './status-badge'

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
  // Fetch the full detail when the sheet opens for a concrete log id. The
  // hook is enabled=false when `log` is null so opening the sheet with no
  // selection doesn't fire a request.
  const detailQuery = useProxyLog(log?.id ?? null)

  const detail: ProxyLogDetail | null =
    detailQuery.data ??
    (log ? ({ ...log } as unknown as ProxyLogDetail) : null)

  const pathMeta = useMemo(
    () => parseProxyLogPathMeta(detail?.errorMessage ?? undefined),
    [detail?.errorMessage],
  )

  if (!log) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' className='sm:max-w-lg' />
      </Sheet>
    )
  }

  const isDetailLoading =
    detailQuery.isLoading && detailQuery.isFetching

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='flex w-full flex-col gap-0 sm:max-w-lg'>
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2 pr-6'>
            <span className='truncate'>
              {log.modelRequested || log.modelActual || `#${log.id}`}
            </span>
            <StatusBadge status={log.status} httpStatus={detail?.httpStatus ?? null} />
          </SheetTitle>
          <SheetDescription className='truncate'>
            {`日志 #${log.id} · ${log.createdAt}`}
          </SheetDescription>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          {isDetailLoading && (
            <div className='flex items-center justify-center gap-2 py-6 text-sm text-muted-foreground'>
              <Spinner className='size-4' />
              加载详情…
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
                title='请求体'
                body={detail.requestBody}
              />
              <RawBodySection
                title='响应体'
                body={detail.responseBody}
              />
              <RawBodySection
                title='请求 Headers'
                body={detail.requestHeaders}
              />
              <RawBodySection
                title='响应 Headers'
                body={detail.responseHeaders}
              />
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

// ---------------------------------------------------------------------------
// Sections
// ---------------------------------------------------------------------------

function DetailOverview({ detail }: { detail: ProxyLogDetail }) {
  return (
    <section>
      <h3 className='mb-2 text-sm font-medium'>概览</h3>
      <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
        <DetailField label='时间'>{detail.createdAt}</DetailField>
        <DetailField label='HTTP 状态'>
          <StatusBadge
            status={detail.status}
            httpStatus={detail.httpStatus ?? null}
          />
        </DetailField>
        <DetailField label='账号'>
          {detail.username || (detail.accountId ? `#${detail.accountId}` : '—')}
        </DetailField>
        <DetailField label='站点'>
          {detail.siteName || (detail.siteId ? `#${detail.siteId}` : '—')}
        </DetailField>
        <DetailField label='请求模型'>{detail.modelRequested || '—'}</DetailField>
        <DetailField label='实际模型'>{detail.modelActual || '—'}</DetailField>
        <DetailField label='延迟'>
          <LatencyBadge
            latencyMs={detail.latencyMs}
            firstByteLatencyMs={detail.firstByteLatencyMs}
            showDot
          />
        </DetailField>
        <DetailField label='流式'>
          {detail.isStream ? '是' : '否'}
        </DetailField>
        <DetailField label='令牌'>
          {detail.downstreamKeyName ||
            (detail.downstreamKeyId ? `#${detail.downstreamKeyId}` : '—')}
        </DetailField>
        <DetailField label='重试'>
          {detail.retryCount ? `×${detail.retryCount}` : '0'}
        </DetailField>
        <DetailField label='路由'>
          {detail.routeId ? `#${detail.routeId}` : '—'}
        </DetailField>
        <DetailField label='渠道'>
          {detail.channelId ? `#${detail.channelId}` : '—'}
        </DetailField>
        <DetailField label='Tokens'>
          {formatTokens(detail.totalTokens, detail.promptTokens, detail.completionTokens)}
        </DetailField>
        <DetailField label='预估成本'>
          {detail.estimatedCost !== null && detail.estimatedCost !== undefined
            ? `$${detail.estimatedCost.toFixed(4)}`
            : '—'}
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
  if (!downstreamPath && !upstreamPath && !clientFamily && !sessionId) {
    return null
  }

  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>会话路径</h3>
        <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
          <DetailField label='客户端'>
            {clientFamily || '—'}
          </DetailField>
          <DetailField label='会话 ID'>
            {sessionId || '—'}
          </DetailField>
          <DetailField label='下行路径' full>
            {downstreamPath ? (
              <code className='block break-all rounded bg-muted px-1.5 py-0.5 text-xs'>
                {downstreamPath}
              </code>
            ) : (
              '—'
            )}
          </DetailField>
          <DetailField label='上行路径' full>
            {upstreamPath ? (
              <code className='block break-all rounded bg-muted px-1.5 py-0.5 text-xs'>
                {upstreamPath}
              </code>
            ) : (
              '—'
            )}
          </DetailField>
          <DetailField label='用量来源'>
            {usageSource || '—'}
          </DetailField>
        </dl>
      </section>
    </>
  )
}

function ErrorSection({ errorMessage }: { errorMessage: string }) {
  if (!errorMessage) return null

  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium text-destructive'>错误信息</h3>
        <pre className='max-h-60 overflow-auto whitespace-pre-wrap break-all rounded border border-destructive/30 bg-destructive/5 p-2 text-xs text-destructive'>
          {errorMessage}
        </pre>
      </section>
    </>
  )
}

function BillingSection({ billing }: { billing: ProxyLogBillingDetails }) {
  if (!billing) return null

  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>计费详情</h3>
        <JsonBlock value={billing} />
      </section>
    </>
  )
}

function RawBodySection({ title, body }: { title: string; body?: string | null }) {
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
      <dt className='text-[11px] text-muted-foreground'>{label}</dt>
      <dd className='min-w-0 break-words'>{children}</dd>
    </div>
  )
}

function formatTokens(
  total: number | null | undefined,
  prompt: number | null | undefined,
  completion: number | null | undefined,
): string {
  if (total === null || total === undefined) return '—'
  if (total === 0 && !prompt && !completion) return '0'
  const parts = [String(total)]
  if (prompt !== null && prompt !== undefined) parts.push(`↑${prompt}`)
  if (completion !== null && completion !== undefined) parts.push(`↓${completion}`)
  return parts.join(' ')
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
  const [copied, setCopied] = useState(false)
  const text = useMemo(() => prettyPrintJson(value), [value])

  function handleCopy() {
    if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) return
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    })
  }

  return (
    <div className='relative'>
      <Button
        variant='ghost'
        size='icon-sm'
        onClick={handleCopy}
        className='absolute right-1 top-1'
        aria-label='复制'
      >
        {copied ? <CheckIcon className='size-3.5' /> : <CopyIcon className='size-3.5' />}
      </Button>
      <pre className='max-h-80 overflow-auto whitespace-pre-wrap break-all rounded bg-muted p-2 pr-9 text-xs leading-relaxed'>
        {text}
      </pre>
    </div>
  )
}
