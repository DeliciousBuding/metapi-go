import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

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
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'
import type { UpstreamDeletionPreview } from '@/lib/api/upstream-lifecycle'

import type { UpstreamDeletionTarget } from './upstream-deletion'

type UpstreamDeleteDialogProps = {
  target: UpstreamDeletionTarget
  preview?: UpstreamDeletionPreview
  error?: 'previewFailed' | 'deleteFailed' | 'changed'
  review: number
  isPending: boolean
  onCancel: () => void
  onRetry: () => void
  onConfirm: () => void
}

const countLabels = {
  channels: 'common.upstreamDeletion.counts.channels',
  models: 'common.upstreamDeletion.counts.models',
  credentials: 'common.upstreamDeletion.counts.credentials',
  grants: 'common.upstreamDeletion.counts.grants',
  groups: 'common.upstreamDeletion.counts.groups',
  members: 'common.upstreamDeletion.counts.members',
  routes: 'common.upstreamDeletion.counts.routes',
  routeChannels: 'common.upstreamDeletion.counts.routeChannels',
  routeGroupSources: 'common.upstreamDeletion.counts.routeGroupSources',
  downstreamKeys: 'common.upstreamDeletion.counts.downstreamKeys',
  sourceMappings: 'common.upstreamDeletion.counts.sourceMappings',
} as const

export function UpstreamDeleteDialog(props: UpstreamDeleteDialogProps) {
  const { t } = useTranslation()
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.isPending) props.onCancel()
      }}
    >
      <DialogContent showCloseButton={!props.isPending} className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>
            {t('common.upstreamDeletion.title', { name: props.target.name })}
          </DialogTitle>
          <DialogDescription>
            {t('common.upstreamDeletion.description')}
          </DialogDescription>
        </DialogHeader>
        {props.error && (
          <Notice
            tone={props.error === 'changed' ? 'warning' : 'destructive'}
            role='alert'
          >
            {t(`common.upstreamDeletion.${props.error}`)}
          </Notice>
        )}
        {!props.preview && props.isPending && (
          <div
            role='status'
            className='text-muted-foreground flex items-center gap-2 py-4'
          >
            <Spinner className='size-4' />
            {t('common.upstreamDeletion.loading')}
          </div>
        )}
        {props.preview ? (
          <DeletionConfirmation
            key={`${props.target.kind}-${props.target.id}-${props.preview.revision}-${props.review}`}
            preview={props.preview}
            isPending={props.isPending}
            onCancel={props.onCancel}
            onConfirm={props.onConfirm}
          />
        ) : (
          <DialogFooter>
            <Button
              variant='outline'
              onClick={props.onCancel}
              disabled={props.isPending}
            >
              {t('common.cancel')}
            </Button>
            {props.error && (
              <Button onClick={props.onRetry} disabled={props.isPending}>
                {t('common.retry')}
              </Button>
            )}
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}

function DeletionConfirmation(
  props: Pick<
    UpstreamDeleteDialogProps,
    'isPending' | 'onCancel' | 'onConfirm'
  > & { preview: UpstreamDeletionPreview }
) {
  const { t } = useTranslation()
  const inputId = useId()
  const [countdown, setCountdown] = useState(3)
  const [word, setWord] = useState('')
  useEffect(() => {
    if (countdown === 0) return
    const timer = window.setTimeout(
      () => setCountdown((remaining) => remaining - 1),
      1000
    )
    return () => window.clearTimeout(timer)
  }, [countdown])
  const canConfirm =
    countdown === 0 && word.trim() === 'DELETE' && !props.isPending
  let confirmLabel = t('common.upstreamDeletion.confirm')
  if (props.isPending) confirmLabel = t('common.upstreamDeletion.pending')
  else if (countdown > 0) {
    confirmLabel = t('common.upstreamDeletion.countdown', {
      seconds: countdown,
    })
  }
  return (
    <>
      <dl className='grid grid-cols-2 gap-x-6 gap-y-2 rounded-lg border p-3 text-sm'>
        {(Object.keys(countLabels) as Array<keyof typeof countLabels>).map(
          (key) => {
            const count = props.preview.counts[key] ?? 0
            return count > 0 ? (
              <div
                className='col-span-2 flex items-baseline justify-between gap-4'
                key={key}
              >
                <dt>{t(countLabels[key])}</dt>
                <dd className='font-medium tabular-nums'>{count}</dd>
              </div>
            ) : null
          }
        )}
      </dl>
      {props.preview.affectedRouteIds.length > 0 && (
        <p className='text-muted-foreground text-sm break-words'>
          {t('common.upstreamDeletion.affectedRoutes', {
            routes: props.preview.affectedRouteIds
              .map((id) => `#${id}`)
              .join(', '),
          })}
        </p>
      )}
      <div className='grid gap-2'>
        <label htmlFor={inputId} className='text-sm'>
          {t('common.upstreamDeletion.typeHint', { word: 'DELETE' })}
        </label>
        <Input
          id={inputId}
          value={word}
          onChange={(event) => setWord(event.target.value)}
          disabled={props.isPending}
          autoComplete='off'
          spellCheck={false}
          placeholder='DELETE'
        />
      </div>
      <DialogFooter>
        <Button
          variant='outline'
          onClick={props.onCancel}
          disabled={props.isPending}
        >
          {t('common.cancel')}
        </Button>
        <Button
          variant='destructive'
          disabled={!canConfirm}
          onClick={() => {
            if (canConfirm) props.onConfirm()
          }}
        >
          {confirmLabel}
        </Button>
      </DialogFooter>
    </>
  )
}
