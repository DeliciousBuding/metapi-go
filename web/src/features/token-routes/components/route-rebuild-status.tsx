import { CheckCircle2, Info, RefreshCw, TriangleAlert } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Notice, type NoticeTone } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import {
  rebuildHasWarnings,
  type useRebuildRoutes,
  type RebuildRoutesResult,
} from '../api'
import type { useRouteRebuildTask } from '../lib/use-route-rebuild-task'

type Props = {
  rebuild: ReturnType<typeof useRebuildRoutes>
  observation: ReturnType<typeof useRouteRebuildTask>
}

function RebuildResult({ result }: { result: RebuildRoutesResult }) {
  const { t } = useTranslation()
  const refresh = result.modelRefresh
  const metrics = [
    [t('tokenRoutes.rebuild.routesCreated'), result.routesCreated ?? 0],
    [t('tokenRoutes.rebuild.routesConsidered'), result.routesConsidered ?? 0],
    [t('tokenRoutes.rebuild.channelsInserted'), result.channelsInserted ?? 0],
    [t('tokenRoutes.rebuild.channelsRemoved'), result.channelsRemoved ?? 0],
    [t('tokenRoutes.rebuild.channelsKept'), result.channelsKept ?? 0],
    [
      t('tokenRoutes.rebuild.unsafeModelsSkipped'),
      result.unsafeModelsSkipped ?? 0,
    ],
  ] as const
  return (
    <div className='space-y-2'>
      <dl className='grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3'>
        {metrics.map(([label, value]) => (
          <div key={label} className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='text-sm font-medium tabular-nums'>{value}</dd>
          </div>
        ))}
      </dl>
      {refresh && (
        <p className='text-muted-foreground text-sm'>
          {t('tokenRoutes.rebuild.modelRefresh', refresh)}
        </p>
      )}
    </div>
  )
}

export function RouteRebuildStatus(props: Props) {
  const { t } = useTranslation()
  const titleId = useId()
  const { reference, task, queryError, isChecking } = props.observation
  const submitting = props.rebuild.isPending
  const submissionError = props.rebuild.isError
  if (!reference && !submitting && !submissionError) return null

  let title = 'tokenRoutes.rebuild.checking'
  let hint = 'tokenRoutes.rebuild.backgroundHint'
  let tone: NoticeTone = 'info'
  let loading = true
  let terminal = false
  let result: RebuildRoutesResult | undefined
  if (submitting) {
    title = 'tokenRoutes.rebuild.submitting'
  } else if (submissionError) {
    title = 'tokenRoutes.rebuild.launchUnknown'
    hint = 'tokenRoutes.rebuild.launchUnknownHint'
    tone = 'warning'
    loading = false
  } else if (queryError) {
    title = 'tokenRoutes.rebuild.queryUnknown'
    hint = 'tokenRoutes.rebuild.queryUnknownHint'
    tone = 'warning'
    loading = false
  } else if (task?.status === 'pending') {
    title = 'tokenRoutes.rebuild.pending'
  } else if (task?.status === 'running') {
    title = 'tokenRoutes.rebuild.running'
  } else if (task?.status === 'failed') {
    title = 'tokenRoutes.rebuild.failed'
    hint = 'tokenRoutes.rebuild.failedHint'
    tone = 'destructive'
    loading = false
    terminal = true
  } else if (task?.status === 'succeeded') {
    result = task.result ?? undefined
    const partial = result && rebuildHasWarnings(result)
    title = partial
      ? 'tokenRoutes.rebuild.partial'
      : 'tokenRoutes.rebuild.succeeded'
    hint = partial
      ? 'tokenRoutes.rebuild.partialHint'
      : 'tokenRoutes.rebuild.succeededHint'
    tone = partial ? 'warning' : 'success'
    loading = false
    terminal = true
  }

  const retryRebuild = () =>
    props.rebuild.mutate({
      refreshModels: submissionError
        ? (props.rebuild.variables?.refreshModels ?? true)
        : (reference?.refreshModels ?? true),
      wait: false,
    })
  let icon = (
    <TriangleAlert className='mt-0.5 size-4 shrink-0' aria-hidden='true' />
  )
  if (loading) {
    icon = <Spinner className='mt-0.5 size-4 shrink-0' aria-hidden='true' />
  } else if (tone === 'success') {
    icon = (
      <CheckCircle2 className='mt-0.5 size-4 shrink-0' aria-hidden='true' />
    )
  }

  const retrying = submitting || isChecking
  return (
    <section
      aria-labelledby={titleId}
      className='min-w-0 space-y-2 rounded-lg border p-3'
    >
      <Notice
        tone={tone}
        role={tone === 'warning' || tone === 'destructive' ? 'alert' : 'status'}
      >
        {icon}
        <div className='min-w-0 space-y-1'>
          <h2 id={titleId} className='font-medium'>
            {t(title)}
          </h2>
          <p>{t(hint)}</p>
          {terminal && task?.status === 'failed' && task.error && (
            <p className='wrap-anywhere'>{task.error}</p>
          )}
        </div>
      </Notice>
      {reference && (
        <p className='text-muted-foreground flex min-w-0 items-start gap-1 text-xs'>
          <Info className='mt-0.5 size-3 shrink-0' aria-hidden='true' />
          <span className='min-w-0 wrap-anywhere'>
            {t('tokenRoutes.rebuild.taskId', { id: reference.taskId })}
          </span>
        </p>
      )}
      {result && <RebuildResult result={result} />}
      <div className='flex flex-wrap gap-2'>
        {queryError && !submissionError && (
          <Button
            variant='secondary'
            size='sm'
            disabled={retrying}
            onClick={() => void props.observation.retryQuery()}
          >
            <RefreshCw aria-hidden='true' />
            {t('tokenRoutes.rebuild.retryQuery')}
          </Button>
        )}
        {(submissionError || queryError || task?.status === 'failed') && (
          <Button
            variant='outline'
            size='sm'
            disabled={retrying}
            onClick={retryRebuild}
          >
            {t(
              submissionError
                ? 'tokenRoutes.rebuild.retryLaunch'
                : 'tokenRoutes.rebuild.recover'
            )}
          </Button>
        )}
        {terminal && (
          <Button variant='ghost' size='sm' onClick={props.observation.dismiss}>
            {t('tokenRoutes.rebuild.dismiss')}
          </Button>
        )}
      </div>
    </section>
  )
}
