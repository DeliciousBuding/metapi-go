// metapi-go/features/sites — site probe report panel (detail Sheet).
//
// Drives GET /api/sites/{id}/probe-stream (SSE): the stream runs a probe
// pass and pushes one `probe-result` event per model as it completes, then
// a `complete` event with honest totals (including truncated + reason when
// the server cut the pass short). The panel renders the live incremental
// state, and the last completed report is cached in-module so reopening the
// sheet shows the most recent result instead of a blank panel.
//
// Honesty rule: `failure` rows are rendered as failures (red), the probe
// machinery's `error` status is rendered as probe error (warning) with the
// error text, and a truncated pass shows the truncation banner — a broken
// site is never presented as healthy.

import { Search as SearchIcon } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'
import { toBcp47 } from '@/i18n/languages'
import { sitesApi, type SiteProbeResult } from '@/lib/api/sites'
import { formatLatency, formatRelativeTime } from '@/lib/format'

type ProbeSummary = {
  totalModels: number
  available: number
  unavailable: number
  truncated?: boolean
  reason?: string
}

type CompletedReport = {
  rows: SiteProbeResult[]
  summary: ProbeSummary | null
  finishedAt: string
}

type RunPhase = 'idle' | 'running' | 'stopped' | 'error'

/** Last completed report per siteId, kept for sheet reopen (session only). */
const reportCache = new Map<number, CompletedReport>()

function statusBadgeFor(status: SiteProbeResult['status']) {
  switch (status) {
    case 'success':
      return { variant: 'success' as const, key: 'sites.probe.statusSuccess' }
    case 'failure':
      return {
        variant: 'destructive' as const,
        key: 'sites.probe.statusFailure',
      }
    default:
      return { variant: 'warning' as const, key: 'sites.probe.statusError' }
  }
}

export function SiteProbePanel({ siteId }: { siteId: number }) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')

  // Seed from the session cache so a reopened sheet keeps the last report.
  const [cached] = useState<CompletedReport | null>(
    () => reportCache.get(siteId) ?? null
  )
  const [phase, setPhase] = useState<RunPhase>('idle')
  const [rows, setRows] = useState<SiteProbeResult[]>(cached?.rows ?? [])
  const [summary, setSummary] = useState<ProbeSummary | null>(
    cached?.summary ?? null
  )
  const [finishedAt, setFinishedAt] = useState<string | null>(
    cached?.finishedAt ?? null
  )
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  const abortRef = useRef<AbortController | null>(null)
  // Guards against a stale sheet (siteId change) streaming into the new site.
  const siteRef = useRef(siteId)
  siteRef.current = siteId
  const rowsRef = useRef(rows)
  rowsRef.current = rows

  useEffect(() => {
    return () => abortRef.current?.abort()
  }, [])

  const startProbe = useCallback(() => {
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    setPhase('running')
    setRows([])
    setSummary(null)
    setFinishedAt(null)
    setErrorMessage(null)

    void sitesApi
      .streamSiteProbe(siteRef.current, {
        signal: controller.signal,
        onResult: (result) => {
          setRows((prev) => [...prev, result])
        },
        onComplete: (payload) => {
          const completed = {
            rows: rowsRef.current,
            summary: {
              totalModels: payload.totalModels,
              available: payload.available,
              unavailable: payload.unavailable,
              truncated: payload.truncated,
              reason: payload.reason,
            },
            finishedAt: new Date().toISOString(),
          }
          reportCache.set(siteRef.current, completed)
          setSummary(completed.summary)
          setFinishedAt(completed.finishedAt)
          setPhase('idle')
        },
      })
      .catch((error: unknown) => {
        const err = error as Error | null
        if (err?.name === 'AbortError' || controller.signal.aborted) {
          // Aborted by the operator: keep what streamed so far, honestly
          // marked as stopped rather than pretending the run completed.
          setPhase('stopped')
          return
        }
        setErrorMessage(err?.message ?? String(error))
        setPhase('error')
      })
  }, [])

  const stopProbe = useCallback(() => {
    abortRef.current?.abort()
    // phase settles in the catch handler
  }, [])

  const hasRows = rows.length > 0

  return (
    <section className='flex flex-col gap-2'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div>
          <h3 className='text-sm font-medium'>{t('sites.probe.title')}</h3>
          <p className='text-muted-foreground text-xs'>
            {t('sites.probe.description')}
          </p>
        </div>
        <div className='flex items-center gap-2'>
          {phase === 'running' ? (
            <Button variant='outline' size='sm' onClick={stopProbe}>
              <Spinner />
              {t('sites.probe.stopButton')}
            </Button>
          ) : (
            <Button variant='outline' size='sm' onClick={startProbe}>
              <SearchIcon className='size-3.5' />
              {t('sites.probe.runButton')}
            </Button>
          )}
        </div>
      </div>

      {phase === 'running' && !hasRows && (
        <p className='text-muted-foreground text-xs' aria-live='polite'>
          {t('sites.probe.awaitingResults')}
        </p>
      )}

      {phase === 'stopped' && (
        <p className='text-muted-foreground text-xs' aria-live='polite'>
          {t('sites.probe.stoppedNotice')}
        </p>
      )}

      {phase === 'error' && (
        <Notice tone='destructive' size='compact' role='alert'>
          <p>
            {t('sites.probe.errorMessage', {
              message: errorMessage ?? '',
            })}
          </p>
        </Notice>
      )}

      {summary && (
        <p className='text-muted-foreground text-xs' aria-live='polite'>
          {t('sites.probe.summary', {
            total: summary.totalModels,
            available: summary.available,
            unavailable: summary.unavailable,
          })}
          {finishedAt
            ? ` · ${t('sites.probe.finishedAt', {
                time: formatRelativeTime(finishedAt, locale),
              })}`
            : ''}
        </p>
      )}

      {summary?.truncated && (
        <p
          className='border-warning/40 bg-warning/10 rounded-md border p-2 text-xs'
          role='alert'
        >
          {t('sites.probe.truncatedWarning', {
            reason: summary.reason ?? t('sites.probe.truncatedNoReason'),
          })}
        </p>
      )}

      {phase !== 'running' && !hasRows && (
        <p
          className='text-muted-foreground rounded-md border border-dashed p-3 text-center text-xs'
          aria-live='polite'
        >
          {t('sites.probe.empty')}
        </p>
      )}

      {hasRows && (
        <div className='overflow-x-auto rounded-md border'>
          <table className='w-full text-xs'>
            <thead className='bg-muted/50 text-left'>
              <tr>
                <th className='px-2 py-1.5 font-medium'>
                  {t('sites.probe.colModel')}
                </th>
                <th className='px-2 py-1.5 font-medium'>
                  {t('sites.probe.colStatus')}
                </th>
                <th className='px-2 py-1.5 text-right font-medium'>
                  {t('sites.probe.colLatency')}
                </th>
                <th className='px-2 py-1.5 font-medium'>
                  {t('sites.probe.colError')}
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((result) => {
                const badge = statusBadgeFor(result.status)
                return (
                  <tr
                    key={`${result.channelId}-${result.model}`}
                    className='border-t'
                  >
                    <td className='px-2 py-1.5 break-all'>{result.model}</td>
                    <td className='px-2 py-1.5'>
                      <Badge variant={badge.variant}>{t(badge.key)}</Badge>
                    </td>
                    <td className='px-2 py-1.5 text-right tabular-nums'>
                      {formatLatency(result.latencyMs, {
                        autoSeconds: true,
                        spaced: true,
                      })}
                    </td>
                    <td
                      className='text-muted-foreground max-w-40 px-2 py-1.5 break-words'
                      title={result.error}
                    >
                      {result.error || '—'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
