// metapi-go/features/channels/components — inline synchronous channel probe card.
//
// Embedded inside ChannelDetailSheet and CooldownReasonDialog so an operator
// can verify a single route channel in place via `POST /api/admin/test-channel`
// without leaving the sheet/dialog. Supports lightweight `models` discovery and
// minimal single-turn `chat` probes, cancellation via AbortController, and
// honest rendering of optional additive fields (`firstByteLatencyMs`,
// `upstreamReportedModel`) without fabricating missing measurements.

import {
  Activity,
  CheckCircle2,
  Play,
  Square,
  TriangleAlert,
} from 'lucide-react'
import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { ExpandableText } from '@/components/common/expandable-text'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { formatLatency } from '@/lib/format'
import { extractApiErrorBody, resolveResponseMessage } from '@/lib/http-client'
import { cn } from '@/lib/utils'

import {
  probeChannel,
  type ChannelProbeMode,
  type ChannelProbeResult,
} from '../api'

type ChannelInlineProbeProps = {
  channelId: number
  /** Pre-filled default model when the channel exposes a concrete model name. */
  defaultModel?: string | null
  className?: string
}

/**
 * Only pre-fill concrete model names; route patterns with wildcards (`gpt-*`)
 * or placeholders (`—`) should stay empty so the backend falls back cleanly.
 */
function initialModelInput(raw: string | null | undefined): string {
  const trimmed = raw?.trim() ?? ''
  if (!trimmed || trimmed === '—' || /[*?[\]{}|^$\\]/.test(trimmed)) {
    return ''
  }
  return trimmed
}

function isAbortError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const record = error as { name?: unknown; code?: unknown }
  return record.name === 'AbortError' || record.code === 'ERR_CANCELED'
}

function formatProbeError(error: unknown, fallback: string): string {
  const apiBody = extractApiErrorBody(error)
  const bodyMessage = resolveResponseMessage(apiBody)
  if (bodyMessage) return bodyMessage
  if (error instanceof Error && error.message.trim()) {
    return error.message
  }
  return fallback
}

export function ChannelInlineProbe({
  channelId,
  defaultModel,
  className,
}: ChannelInlineProbeProps) {
  const { t } = useTranslation()
  const baseId = useId()
  const [mode, setMode] = useState<ChannelProbeMode>('models')
  const [modelInput, setModelInput] = useState(() =>
    initialModelInput(defaultModel)
  )
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<ChannelProbeResult | null>(null)
  const [requestError, setRequestError] = useState<string | null>(null)
  const [stopped, setStopped] = useState(false)

  const abortRef = useRef<AbortController | null>(null)
  const runIdRef = useRef(0)

  // Reset probe state and cancel any in-flight request when switching channels.
  useEffect(() => {
    abortRef.current?.abort()
    abortRef.current = null
    runIdRef.current += 1
    setRunning(false)
    setResult(null)
    setRequestError(null)
    setStopped(false)
    setModelInput(initialModelInput(defaultModel))
  }, [channelId, defaultModel])

  // Abort on unmount so late responses cannot touch unmounted state.
  useEffect(() => {
    return () => {
      abortRef.current?.abort()
      abortRef.current = null
    }
  }, [])

  function handleStop() {
    abortRef.current?.abort()
    abortRef.current = null
    setRunning(false)
    setStopped(true)
  }

  async function handleRun() {
    if (running || channelId <= 0) return
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    const currentRun = ++runIdRef.current

    setRunning(true)
    setResult(null)
    setRequestError(null)
    setStopped(false)

    try {
      const response = await probeChannel(
        {
          channelId,
          mode,
          model: modelInput.trim() || undefined,
        },
        controller.signal
      )
      if (runIdRef.current !== currentRun) return
      setResult(response)
    } catch (error) {
      if (runIdRef.current !== currentRun) return
      if (isAbortError(error)) {
        setStopped(true)
      } else {
        setRequestError(formatProbeError(error, t('common.requestFailed')))
      }
    } finally {
      if (runIdRef.current === currentRun) {
        abortRef.current = null
        setRunning(false)
      }
    }
  }

  function handleInputKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    // Ignore IME conversion confirmations (including Safari keyCode 229).
    if (
      event.nativeEvent.isComposing ||
      event.keyCode === 229 ||
      event.key !== 'Enter'
    ) {
      return
    }
    event.preventDefault()
    void handleRun()
  }

  const effectiveRequestModel =
    result?.model?.trim() || modelInput.trim() || null
  const reportedModel = result?.upstreamReportedModel?.trim() || null
  const reportedModelDiffers =
    reportedModel !== null &&
    effectiveRequestModel !== null &&
    reportedModel.toLowerCase() !== effectiveRequestModel.toLowerCase()
  const hasFirstByte =
    typeof result?.firstByteLatencyMs === 'number' &&
    Number.isFinite(result.firstByteLatencyMs)

  return (
    <section
      aria-labelledby={`${baseId}-heading`}
      className={cn(
        'bg-muted/30 flex min-w-0 flex-col gap-2.5 rounded-lg border p-3',
        className
      )}
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex min-w-0 items-center gap-1.5'>
          <Activity
            aria-hidden='true'
            className='text-muted-foreground size-3.5 shrink-0'
          />
          <h4 id={`${baseId}-heading`} className='text-xs font-semibold'>
            {t('modelTester.page.title')}
          </h4>
        </div>

        <div
          role='group'
          aria-label={t('proxyLogs.columns.mode')}
          className='bg-background inline-flex rounded-md border p-0.5'
        >
          {(['models', 'chat'] as const).map((option) => {
            const active = mode === option
            const label =
              option === 'models'
                ? t('channels.catalog.models')
                : t('channels.capabilities.names.chat')
            return (
              <button
                key={option}
                type='button'
                aria-pressed={active}
                disabled={running}
                onClick={() => setMode(option)}
                className={cn(
                  'focus-visible:ring-focus-ring rounded-sm px-2 py-0.5 text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50',
                  active
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:text-foreground'
                )}
              >
                {label}
              </button>
            )
          })}
        </div>
      </div>

      <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
        <Input
          id={`${baseId}-model`}
          value={modelInput}
          disabled={running}
          onChange={(event) => setModelInput(event.target.value)}
          onKeyDown={handleInputKeyDown}
          aria-label={t('modelTester.form.model')}
          placeholder={t('tokenRoutes.preview.modelPlaceholder')}
          className='bg-background h-8 min-w-0 flex-1 text-xs'
        />
        {running ? (
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={handleStop}
            className='h-8 shrink-0 gap-1.5'
          >
            <Spinner className='size-3.5' />
            <Square aria-hidden='true' className='size-3' />
            {t('sites.probe.stopButton')}
          </Button>
        ) : (
          <Button
            type='button'
            size='sm'
            variant='default'
            onClick={() => void handleRun()}
            disabled={channelId <= 0}
            className='h-8 shrink-0 gap-1.5'
          >
            <Play aria-hidden='true' className='size-3' />
            {t('sites.probe.runButton')}
          </Button>
        )}
      </div>

      <p className='text-muted-foreground text-2xs leading-relaxed'>
        {t('modelTester.page.description')}
      </p>

      {stopped && !running && !result && !requestError && (
        <p role='status' className='text-muted-foreground text-xs'>
          {t('modelTester.error.stopped')}
        </p>
      )}

      {requestError && (
        <div
          role='alert'
          className='border-destructive/40 bg-destructive/10 text-destructive-soft-fg flex items-start gap-2 rounded-md border px-2.5 py-2 text-xs'
        >
          <TriangleAlert
            aria-hidden='true'
            className='mt-0.5 size-3.5 shrink-0'
          />
          <span className='min-w-0 break-words'>{requestError}</span>
        </div>
      )}

      {result && (
        <div
          role='status'
          className='bg-background flex min-w-0 flex-col gap-2 rounded-md border p-2.5 text-xs'
        >
          <div className='flex flex-wrap items-center gap-1.5'>
            <Badge
              variant={result.success ? 'success' : 'destructive'}
              className='gap-1'
            >
              {result.success ? (
                <CheckCircle2 aria-hidden='true' className='size-3' />
              ) : (
                <TriangleAlert aria-hidden='true' className='size-3' />
              )}
              {result.success
                ? t('modelTester.compare.statusSuccess')
                : t('modelTester.compare.statusFailure')}
            </Badge>

            {result.statusCode > 0 && (
              <Badge variant='outline' className='font-mono tabular-nums'>
                {t('observability.overview.slowRequests.colHttp')}{' '}
                {result.statusCode}
              </Badge>
            )}

            {hasFirstByte && (
              <span className='text-muted-foreground inline-flex items-center gap-1 tabular-nums'>
                <span>{t('proxyLogs.timing.firstByte')}</span>
                <span className='text-foreground font-medium'>
                  {formatLatency(result.firstByteLatencyMs, {
                    autoSeconds: true,
                  })}
                </span>
              </span>
            )}

            <span className='text-muted-foreground inline-flex items-center gap-1 tabular-nums'>
              <span>{t('proxyLogs.timing.total')}</span>
              <span className='text-foreground font-medium'>
                {formatLatency(result.latencyMs, { autoSeconds: true })}
              </span>
            </span>
          </div>

          {reportedModel && (
            <div className='flex flex-wrap items-center gap-1.5 text-xs'>
              <span className='text-muted-foreground'>
                {t('proxyLogs.detail.upstreamReportedModel')}:
              </span>
              <code className='bg-muted/60 rounded px-1.5 py-0.5 font-mono text-xs break-all'>
                {reportedModel}
              </code>
              {reportedModelDiffers && effectiveRequestModel && (
                <Badge variant='warning' className='text-2xs font-mono'>
                  {effectiveRequestModel} → {reportedModel}
                </Badge>
              )}
            </div>
          )}

          {result.error && (
            <div className='text-destructive-soft-fg flex flex-col gap-0.5'>
              <span className='text-muted-foreground text-2xs'>
                {t('channels.reason.errorSummary')}
              </span>
              <ExpandableText
                text={result.error}
                className='font-mono text-xs leading-normal break-all'
              />
            </div>
          )}

          {result.truncatedBody && (
            <div className='flex flex-col gap-0.5'>
              <span className='text-muted-foreground text-2xs'>
                {t('proxyLogs.detail.rawResponse')}
              </span>
              <ExpandableText
                text={result.truncatedBody}
                className='bg-muted/40 rounded border p-1.5 font-mono text-xs leading-normal break-all'
              />
            </div>
          )}
        </div>
      )}
    </section>
  )
}
