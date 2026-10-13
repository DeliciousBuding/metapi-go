// metapi-go/features/channels/components — catalog refresh diff dialog.
//
// Route-decision E batch: re-discover a channel's upstream model catalog,
// preview the diff (added / stale / kept / manual-untouched) with the impact
// of each stale row, then apply with the preview's basis token. A stale basis
// (409) means the catalog or the upstream moved since the preview — the
// dialog offers a one-click re-preview instead of applying an outdated plan.

import { RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
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
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import type { UpstreamRefreshPreview } from '@/lib/api/upstream-catalog'
import { toast } from '@/lib/toast'

type UpstreamRefreshDialogProps = {
  channelId: number
  channelName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Called after a successful apply so the parent can refetch the catalog. */
  onApplied: () => void
}

type PreviewState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; preview: UpstreamRefreshPreview }

function errorText(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}

export function UpstreamRefreshDialog({
  channelId,
  channelName,
  open,
  onOpenChange,
  onApplied,
}: UpstreamRefreshDialogProps) {
  const { t } = useTranslation()
  const [state, setState] = useState<PreviewState>({ phase: 'loading' })
  const [applying, setApplying] = useState(false)

  const loadPreview = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const preview = await api.refreshUpstreamCatalogPreview(channelId)
      setState({ phase: 'ready', preview })
    } catch (err) {
      setState({ phase: 'error', message: errorText(err) })
    }
  }, [channelId])

  useEffect(() => {
    if (open) {
      setApplying(false)
      void loadPreview()
    }
  }, [open, loadPreview])

  async function apply(preview: UpstreamRefreshPreview) {
    setApplying(true)
    try {
      const result = await api.applyUpstreamCatalogRefresh(
        channelId,
        preview.basisToken
      )
      toast.success(
        t('channels.catalog.refresh.applyDone', {
          added: result.addedCount,
          stale: result.staleCount,
        })
      )
      onOpenChange(false)
      onApplied()
    } catch (err) {
      // 409 = basis moved since the preview. Offer a fresh preview instead of
      // a dead-end error; other failures keep the current preview visible.
      const status = (err as { status?: number }).status
      if (status === 409) {
        toast.info(t('channels.catalog.refresh.staleBasis'))
        await loadPreview()
      } else {
        toast.error(errorText(err))
      }
    } finally {
      setApplying(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>
            {t('channels.catalog.refresh.title', { name: channelName })}
          </DialogTitle>
          <DialogDescription>
            {t('channels.catalog.refresh.description')}
          </DialogDescription>
        </DialogHeader>

        {state.phase === 'loading' && (
          <div className='text-muted-foreground flex items-center justify-center gap-2 py-8 text-sm'>
            <Spinner className='size-4' />
            {t('channels.catalog.refresh.loading')}
          </div>
        )}

        {state.phase === 'error' && (
          <div className='space-y-3 py-2'>
            <p className='text-destructive-soft-fg text-sm'>{state.message}</p>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void loadPreview()}
            >
              {t('channels.catalog.refresh.retry')}
            </Button>
          </div>
        )}

        {state.phase === 'ready' && (
          <RefreshPreviewBody
            preview={state.preview}
            applying={applying}
            onApply={() => void apply(state.preview)}
            onCancel={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function RefreshPreviewBody({
  preview,
  applying,
  onApply,
  onCancel,
}: {
  preview: UpstreamRefreshPreview
  applying: boolean
  onApply: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const unchanged = preview.added.length === 0 && preview.stale.length === 0
  return (
    <>
      <div className='max-h-80 space-y-4 overflow-y-auto py-1 text-sm'>
        <p className='text-muted-foreground'>
          {t('channels.catalog.refresh.summary', {
            upstream: preview.upstreamCount,
            added: preview.added.length,
            stale: preview.stale.length,
            kept: preview.kept.length,
          })}
        </p>
        {preview.legacyNativeCount > 0 && (
          <p className='text-muted-foreground text-xs'>
            {t('channels.catalog.refresh.legacyNote', {
              count: preview.legacyNativeCount,
            })}
          </p>
        )}

        {unchanged && (
          <p className='text-sm'>{t('channels.catalog.refresh.noChanges')}</p>
        )}

        {preview.added.length > 0 && (
          <RefreshGroup
            title={t('channels.catalog.refresh.addedTitle')}
            badge='success'
            names={preview.added}
          />
        )}

        {preview.stale.length > 0 && (
          <section className='space-y-1.5'>
            <h4 className='flex items-center gap-2 text-xs font-medium'>
              <Badge variant='warning'>{preview.stale.length}</Badge>
              {t('channels.catalog.refresh.staleTitle')}
            </h4>
            <ul className='divide-border divide-y rounded-md border text-xs'>
              {preview.stale.map((model) => (
                <li key={model.id} className='px-2 py-1.5'>
                  <span className='font-mono'>{model.name}</span>
                  <span className='text-muted-foreground ml-2'>
                    {t('channels.catalog.refresh.staleImpact', {
                      grants: model.grantCount,
                      routes: model.routeIds.length,
                    })}
                  </span>
                </li>
              ))}
            </ul>
          </section>
        )}

        {preview.manualOnly.length > 0 && (
          <RefreshGroup
            title={t('channels.catalog.refresh.manualTitle')}
            badge='outline'
            names={preview.manualOnly}
          />
        )}

        {preview.offline.length > 0 && (
          <RefreshGroup
            title={t('channels.catalog.refresh.offlineTitle')}
            badge='secondary'
            names={preview.offline}
          />
        )}
      </div>

      <DialogFooter>
        <Button variant='outline' onClick={onCancel} disabled={applying}>
          {t('channels.catalog.refresh.cancel')}
        </Button>
        <Button onClick={onApply} disabled={applying || unchanged}>
          {applying ? (
            <Spinner className='size-4' />
          ) : (
            <RefreshCw className='size-4' />
          )}
          {t('channels.catalog.refresh.apply')}
        </Button>
      </DialogFooter>
    </>
  )
}

function RefreshGroup({
  title,
  badge,
  names,
}: {
  title: string
  badge: 'success' | 'outline' | 'secondary'
  names: string[]
}) {
  return (
    <section className='space-y-1.5'>
      <h4 className='flex items-center gap-2 text-xs font-medium'>
        <Badge variant={badge}>{names.length}</Badge>
        {title}
      </h4>
      <ul className='divide-border divide-y rounded-md border font-mono text-xs'>
        {names.map((name) => (
          <li key={name} className='px-2 py-1.5'>
            {name}
          </li>
        ))}
      </ul>
    </section>
  )
}
