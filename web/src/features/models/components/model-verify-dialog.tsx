// metapi-go/features/models — batch availability probe dialog.
//
// Collapses the models-page "batch probe" behind a single dialog that runs
// POST /api/models/verify-batch (the same lightweight probe the background
// model-probe scheduler uses, with durable per-row history) and renders the
// honest per-target outcome table. A "history" toggle surfaces the recent
// verification rows from GET /api/models/verify-history so past passes are
// not lost when the dialog closes.
//
// Honesty rules: probe-machinery error (503 when the scheduler is not
// running) is rendered as a failure with the message — never as success —
// and inconclusive/skipped rows keep their own badge instead of being
// counted as either success or failure.

import { FlaskConical as FlaskConicalIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import type {
  ModelVerifyItem,
  VerifyBatchResponse,
  VerifyHistoryResponse,
} from '@/lib/api/types'
import { formatLatency } from '@/lib/format'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

const STATUS_BADGE_VARIANT: Record<
  ModelVerifyItem['status'],
  'success' | 'destructive' | 'secondary' | 'outline'
> = {
  success: 'success',
  failure: 'destructive',
  inconclusive: 'secondary',
  skipped: 'outline',
}

function statusLabelKey(status: ModelVerifyItem['status']): string {
  switch (status) {
    case 'success':
      return 'models.verify.statusSuccess'
    case 'failure':
      return 'models.verify.statusFailure'
    case 'inconclusive':
      return 'models.verify.statusInconclusive'
    default:
      return 'models.verify.statusSkipped'
  }
}

function VerifyItemRow({ item }: { item: ModelVerifyItem }) {
  const { t } = useTranslation()
  return (
    <tr className='border-t'>
      <td className='px-3 py-1.5 break-all'>{item.model}</td>
      <td className='text-muted-foreground px-3 py-1.5'>
        {item.siteName || '—'}
      </td>
      <td className='px-3 py-1.5'>
        <Badge variant={STATUS_BADGE_VARIANT[item.status]}>
          {t(statusLabelKey(item.status))}
        </Badge>
      </td>
      <td className='px-3 py-1.5 text-right tabular-nums'>
        {formatLatency(item.latencyMs, { autoSeconds: true, spaced: true })}
      </td>
      <td
        className='text-muted-foreground max-w-52 px-3 py-1.5 break-words'
        title={item.errorText ?? ''}
      >
        {item.errorText || '—'}
      </td>
    </tr>
  )
}

type ModelVerifyDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ModelVerifyDialog({
  open,
  onOpenChange,
}: ModelVerifyDialogProps) {
  const { t } = useTranslation()

  const [modelFilter, setModelFilter] = useState('')
  const [phase, setPhase] = useState<'idle' | 'running' | 'done' | 'error'>(
    'idle'
  )
  const [result, setResult] = useState<VerifyBatchResponse | null>(null)
  const [errorMessage, setErrorMessage] = useState('')
  const [showHistory, setShowHistory] = useState(false)
  const [history, setHistory] = useState<ModelVerifyItem[] | null>(null)
  const [historyLoading, setHistoryLoading] = useState(false)

  // Reset the run state when the dialog reopens so a fresh pass starts
  // from the idle phase instead of showing the previous run.
  useEffect(() => {
    if (open) {
      setPhase('idle')
      setResult(null)
      setErrorMessage('')
      setModelFilter('')
      setShowHistory(false)
      setHistory(null)
    }
  }, [open])

  function parseModels(): string[] {
    return modelFilter
      .split(/[\s,]+/)
      .map((m) => m.trim())
      .filter(Boolean)
  }

  async function runVerify() {
    setPhase('running')
    setResult(null)
    setErrorMessage('')
    try {
      const response = await api.verifyModelsBatch(parseModels(), 0, 50)
      setResult(response)
      setPhase('done')
      setShowHistory(false)
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      setErrorMessage(message)
      setPhase('error')
    }
  }

  async function loadHistory() {
    setHistoryLoading(true)
    try {
      const response: VerifyHistoryResponse =
        await api.getModelVerifyHistory(20)
      setHistory(response.items)
      setShowHistory(true)
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      toast.error(t('models.verify.historyLoadFailed', { message }))
    } finally {
      setHistoryLoading(false)
    }
  }

  const summary = result?.summary
  const summaryKeys: Array<{
    key: string
    value: number
    className: string
  }> = [
    {
      key: 'models.verify.summarySuccess',
      value: summary?.success ?? 0,
      className: 'text-success-soft-fg',
    },
    {
      key: 'models.verify.summaryFailure',
      value: summary?.failure ?? 0,
      className: 'text-destructive-soft-fg',
    },
    {
      key: 'models.verify.summaryInconclusive',
      value: summary?.inconclusive ?? 0,
      className: 'text-muted-foreground',
    },
    {
      key: 'models.verify.summarySkipped',
      value: summary?.skipped ?? 0,
      className: 'text-muted-foreground',
    },
  ]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <FlaskConicalIcon className='size-4' />
            {t('models.verify.title')}
          </DialogTitle>
          <DialogDescription className='text-xs'>
            {t('models.verify.description')}
          </DialogDescription>
        </DialogHeader>

        <div className='flex flex-col gap-3'>
          <div className='flex flex-col gap-1.5'>
            <label
              className='text-sm font-medium'
              htmlFor='model-verify-filter'
            >
              {t('models.verify.modelLabel')}
            </label>
            <Input
              id='model-verify-filter'
              value={modelFilter}
              onChange={(e) => setModelFilter(e.target.value)}
              placeholder={t('models.verify.modelPlaceholder')}
              disabled={phase === 'running'}
            />
            <p className='text-muted-foreground text-xs'>
              {t('models.verify.modelHint')}
            </p>
          </div>

          {phase === 'error' && (
            <div
              role='alert'
              className='border-destructive/40 bg-destructive/10 text-destructive-soft-fg rounded-md border p-2 text-xs'
            >
              {t('models.verify.errorMessage', { message: errorMessage })}
            </div>
          )}

          {phase === 'done' && result && (
            <div className='flex flex-col gap-2'>
              <div
                className='flex flex-wrap items-center gap-2 text-xs'
                aria-live='polite'
              >
                {summaryKeys.map((entry) => (
                  <span
                    key={entry.key}
                    className={cn('tabular-nums', entry.className)}
                  >
                    {t(entry.key)} {entry.value}
                  </span>
                ))}
                <span className='text-muted-foreground tabular-nums'>
                  {t('models.verify.probedCount', { count: result.probed })}
                </span>
              </div>

              {result.items.length === 0 ? (
                <p className='text-muted-foreground rounded-md border border-dashed p-3 text-center text-xs'>
                  {result.note || t('models.verify.noRows')}
                </p>
              ) : (
                <div className='max-h-64 overflow-y-auto rounded-md border'>
                  <table className='w-full text-xs'>
                    <thead className='bg-muted/50 sticky top-0 text-left'>
                      <tr>
                        <th className='px-3 py-2 font-medium'>
                          {t('models.verify.colModel')}
                        </th>
                        <th className='px-3 py-2 font-medium'>
                          {t('models.verify.colSite')}
                        </th>
                        <th className='px-3 py-2 font-medium'>
                          {t('models.verify.colStatus')}
                        </th>
                        <th className='px-3 py-2 text-right font-medium'>
                          {t('models.verify.colLatency')}
                        </th>
                        <th className='px-3 py-2 font-medium'>
                          {t('models.verify.colError')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {result.items.map((item) => (
                        <VerifyItemRow
                          key={`${item.channelId ?? 'c'}-${item.model}`}
                          item={item}
                        />
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              <div className='flex items-center gap-2'>
                <Button
                  variant='ghost'
                  size='sm'
                  onClick={() => void loadHistory()}
                  disabled={historyLoading}
                >
                  {historyLoading ? <Spinner /> : null}
                  {t('models.verify.viewHistory')}
                </Button>
                {showHistory && (
                  <button
                    type='button'
                    className='text-muted-foreground hover:text-foreground text-xs underline'
                    onClick={() => setShowHistory(false)}
                  >
                    {t('models.verify.hideHistory')}
                  </button>
                )}
              </div>

              {showHistory && history && (
                <div className='max-h-48 overflow-y-auto rounded-md border'>
                  <table className='w-full text-xs'>
                    <thead className='bg-muted/50 sticky top-0 text-left'>
                      <tr>
                        <th className='px-3 py-2 font-medium'>
                          {t('models.verify.colModel')}
                        </th>
                        <th className='px-3 py-2 font-medium'>
                          {t('models.verify.colSite')}
                        </th>
                        <th className='px-3 py-2 font-medium'>
                          {t('models.verify.colStatus')}
                        </th>
                        <th className='px-3 py-2 text-right font-medium'>
                          {t('models.verify.colLatency')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {history.map((item) => (
                        <VerifyItemRow
                          key={
                            item.id != null
                              ? `history-${item.id}`
                              : `history-c${item.channelId ?? 'x'}-${item.model}`
                          }
                          item={item}
                        />
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          {phase === 'running' && <Spinner />}
          <Button
            variant='default'
            onClick={() => void runVerify()}
            disabled={phase === 'running'}
          >
            {phase === 'running'
              ? t('models.verify.runningLabel')
              : t('models.verify.runButton')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
