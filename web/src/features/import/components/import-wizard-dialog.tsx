// metapi-go/features/import — unified five-step import wizard dialog.
//
// source → identify → connect → models/routes → done. The wizard collects
// pasted URLs, auto-detects each platform (unknown stays manually specifiable),
// flags duplicates with skip/merge, attaches optional accounts, sets routing
// weight, then commits one idempotent POST /api/sites/import and reports the
// imported/skipped/failed breakdown.

import { useNavigate } from '@tanstack/react-router'
import axios from 'axios'
import {
  Check as CheckIcon,
  Upload as UploadIcon,
  UserPlus as UserPlusIcon,
  Zap as ZapIcon,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { useSites, type Site } from '@/features/sites'
import { useRebuildRoutes } from '@/features/token-routes'

import { useDetectSite, useImportSites } from '../api'
import { canonicalizeUrl, parseUrlLines } from '../lib/utils'
import type { ImportCandidate, ImportSitesResult, ImportStepId } from '../types'
import { makeCandidate, toImportSiteItem } from './import-candidate'
import { ImportConnectStep } from './import-connect-step'
import { ImportIdentifyStep } from './import-identify-step'
import { ImportResultStep } from './import-result-step'
import { ImportRoutesStep } from './import-routes-step'
import { ImportStepper, type ImportStep } from './stepper'
import { useImportFields } from './use-import-fields'

const STEP_ORDER: ImportStepId[] = [
  'source',
  'identify',
  'connect',
  'routes',
  'done',
]

type ImportWizardDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ImportWizardDialog({
  open,
  onOpenChange,
}: ImportWizardDialogProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const sitesQuery = useSites()
  const detectSite = useDetectSite()
  const importSites = useImportSites()
  const rebuildRoutes = useRebuildRoutes()

  const [step, setStep] = useState<ImportStepId>('source')
  const [sourceText, setSourceText] = useState('')
  const [candidates, setCandidates] = useState<ImportCandidate[]>([])
  const [result, setResult] = useState<ImportSitesResult | null>(null)
  const [discardConfirmOpen, setDiscardConfirmOpen] = useState(false)

  const { bindField, clearInvalid, markInvalidAndFocusFirst } =
    useImportFields()

  const steps: ImportStep[] = useMemo(
    () => [
      { id: 'source', label: t('import.steps.source') },
      { id: 'identify', label: t('import.steps.identify') },
      { id: 'connect', label: t('import.steps.connect') },
      { id: 'routes', label: t('import.steps.routes') },
      { id: 'done', label: t('import.steps.done') },
    ],
    [t]
  )

  const existingUrls = useMemo(() => {
    const urls = new Set<string>()
    for (const site of (sitesQuery.data ?? []) as Site[]) {
      const canonical = canonicalizeUrl(site.url)
      if (canonical) urls.add(canonical)
    }
    return urls
  }, [sitesQuery.data])

  const currentIndex = STEP_ORDER.indexOf(step)

  // Derived busy flags for the aria-busy region. isSubmitting covers the
  // final POST; isDetecting covers the parallel detect fan-out.
  const isSubmitting = importSites.isPending
  const isDetecting = candidates.some((candidate) => candidate.detecting)

  // Polite live region: announce detection completion (only on the true→false
  // transition so the initial pre-detect render never fires a false "done")
  // and the final imported/skipped/failed summary once results land.
  const [liveMessage, setLiveMessage] = useState('')
  const wasDetectingRef = useRef(false)
  useEffect(() => {
    if (step === 'done' && result) {
      setLiveMessage(
        t('import.done.announce', {
          imported: result.imported,
          skipped: result.skipped,
          failed: result.failed,
        })
      )
      return
    }
    if (step === 'identify' && wasDetectingRef.current && !isDetecting) {
      setLiveMessage(
        t('import.identify.detectionComplete', { count: candidates.length })
      )
    }
    wasDetectingRef.current = isDetecting
  }, [step, result, candidates.length, isDetecting, t])

  function reset() {
    setStep('source')
    setSourceText('')
    setCandidates([])
    setResult(null)
    clearInvalid()
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      // Guard against silently dropping a partially filled wizard: any
      // source text, detected candidates, or a finished result counts as
      // input worth confirming.
      const hasInput =
        sourceText.trim().length > 0 || candidates.length > 0 || result !== null
      if (hasInput) {
        setDiscardConfirmOpen(true)
        return
      }
      reset()
    }
    onOpenChange(next)
  }

  function confirmDiscard() {
    setDiscardConfirmOpen(false)
    reset()
    onOpenChange(false)
  }

  function updateCandidate(id: string, patch: Partial<ImportCandidate>) {
    setCandidates((prev) =>
      prev.map((candidate) =>
        candidate.id === id ? { ...candidate, ...patch } : candidate
      )
    )
  }

  async function detectAll(items: ImportCandidate[]) {
    await Promise.all(
      items.map(async (item) => {
        if (item.platform !== '' || item.detecting) return
        updateCandidate(item.id, { detecting: true })
        try {
          const detected = await detectSite.mutateAsync(item.url)
          updateCandidate(item.id, {
            platform: detected.platform ?? '',
            confidence: detected.confidence ?? null,
            detected: detected.platform != null,
            detecting: false,
          })
        } catch {
          updateCandidate(item.id, { detecting: false })
        }
      })
    )
  }

  function handleSourceNext() {
    const urls = parseUrlLines(sourceText)
    if (urls.length === 0) {
      toast.error(t('import.source.empty'))
      markInvalidAndFocusFirst(['source'])
      return
    }
    clearInvalid()
    const initial = urls.map((url, index) => makeCandidate(url, index))
    setCandidates(initial)
    setStep('identify')
    void detectAll(initial)
  }

  function handleIdentifyNext() {
    const missing = candidates.filter(
      (candidate) => candidate.platform.trim() === ''
    )
    if (missing.length > 0) {
      toast.error(t('import.identify.missingPlatform'))
      markInvalidAndFocusFirst(
        missing.map((candidate) => `${candidate.id}-platform`)
      )
      return
    }
    clearInvalid()
    setStep('connect')
  }

  function handleConnectNext() {
    setStep('routes')
  }

  async function handleSubmit() {
    const invalidWeightKeys: string[] = []
    for (const candidate of candidates) {
      if (!Number.isFinite(candidate.weight) || candidate.weight < 0) {
        invalidWeightKeys.push(`${candidate.id}-weight`)
      }
    }
    if (invalidWeightKeys.length > 0) {
      toast.error(t('import.routes.invalidWeight'))
      markInvalidAndFocusFirst(invalidWeightKeys)
      return
    }
    clearInvalid()

    const items = candidates.map(toImportSiteItem)

    try {
      const importResult = await importSites.mutateAsync({
        items,
        duplicateStrategy: 'skip',
      })
      setResult(importResult)
      setStep('done')
    } catch (error) {
      // Transport failures (non-2xx) are toasted by the http-client error
      // interceptor; a plain error thrown by the mutation (e.g. an empty
      // response body) is not, so surface it instead of swallowing.
      if (!axios.isAxiosError(error)) {
        toast.error(t('import.submitFailed'))
      }
    }
  }

  function handleDone() {
    reset()
    onOpenChange(false)
  }

  // Imported sites are not routable until a route rebuild picks them up.
  // Reuse the shared rebuild mutation (it already toasts queued/complete
  // outcomes); close the wizard once the rebuild request lands. On failure
  // the wizard stays open so the operator can retry.
  function handleRebuildRoutes() {
    rebuildRoutes.mutate(undefined, {
      onSuccess: () => {
        reset()
        onOpenChange(false)
      },
    })
  }

  // Mirror the manual site-created modal's exit (site-created-modal.tsx):
  // land the accounts page with the one-shot `siteId`/`create` deep link so
  // the create dialog opens with the imported site preselected. Exactly one
  // imported/merged row with a siteId carries it; any other successful
  // batch at least opens the create flow; an all-skipped/failed batch keeps
  // the plain navigation (nothing to preselect).
  function handleGoToAccounts() {
    const results = result?.results ?? []
    const successful = results.filter(
      (item) => item.status === 'imported' || item.status === 'merged'
    )
    const withSiteId = successful.filter(
      (item) => typeof item.siteId === 'number' && item.siteId > 0
    )
    reset()
    onOpenChange(false)
    if (successful.length === 1 && withSiteId.length === 1) {
      navigate({
        to: '/accounts',
        search: { siteId: withSiteId[0].siteId, create: true },
        replace: true,
      })
      return
    }
    if (successful.length > 0) {
      navigate({
        to: '/accounts',
        search: { create: true },
        replace: true,
      })
      return
    }
    navigate({ to: '/accounts' })
  }

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className='sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{t('import.title')}</DialogTitle>
            <DialogDescription>{t('import.description')}</DialogDescription>
          </DialogHeader>

          <ImportStepper steps={steps} currentIndex={currentIndex} />

          <div
            className='mt-2 min-h-64'
            aria-busy={isSubmitting || isDetecting}
          >
            {step === 'source' && (
              <div className='grid gap-3'>
                <Label htmlFor='import-source-urls'>
                  {t('import.source.label')}
                </Label>
                <Textarea
                  id='import-source-urls'
                  {...bindField('source', setSourceText)}
                  rows={8}
                  value={sourceText}
                  placeholder={t('import.source.placeholder')}
                  className='font-mono text-xs'
                  autoFocus
                />
                <p className='text-muted-foreground text-xs'>
                  {t('import.source.hint')}
                </p>
              </div>
            )}

            {step === 'identify' && (
              <ImportIdentifyStep
                candidates={candidates}
                updateCandidate={updateCandidate}
                existingUrls={existingUrls}
                bindField={bindField}
              />
            )}
            {step === 'connect' && (
              <ImportConnectStep
                candidates={candidates}
                updateCandidate={updateCandidate}
              />
            )}
            {step === 'routes' && (
              <ImportRoutesStep
                candidates={candidates}
                updateCandidate={updateCandidate}
                bindField={bindField}
              />
            )}
            {step === 'done' && result && <ImportResultStep result={result} />}
          </div>

          <div aria-live='polite' className='sr-only'>
            {liveMessage}
          </div>

          <DialogFooter showCloseButton={false}>
            {step !== 'source' && step !== 'done' && (
              <Button
                type='button'
                variant='outline'
                onClick={() =>
                  setStep(
                    STEP_ORDER[Math.max(0, currentIndex - 1)] as ImportStepId
                  )
                }
              >
                {t('import.back')}
              </Button>
            )}
            {step === 'source' && (
              <Button type='button' onClick={handleSourceNext}>
                <UploadIcon className='size-4' />
                {t('import.next')}
              </Button>
            )}
            {step === 'identify' && (
              <Button type='button' onClick={handleIdentifyNext}>
                {t('import.next')}
              </Button>
            )}
            {step === 'connect' && (
              <Button type='button' onClick={handleConnectNext}>
                {t('import.next')}
              </Button>
            )}
            {step === 'routes' && (
              <Button
                type='button'
                onClick={handleSubmit}
                disabled={importSites.isPending}
              >
                {importSites.isPending && <Spinner />}
                {t('import.submit')}
              </Button>
            )}
            {step === 'done' && (
              <>
                <Button type='button' variant='ghost' onClick={handleDone}>
                  <CheckIcon className='size-4' />
                  {t('import.done.close')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  onClick={handleGoToAccounts}
                >
                  <UserPlusIcon className='size-4' />
                  {t('import.done.addAccount')}
                </Button>
                <Button
                  type='button'
                  onClick={handleRebuildRoutes}
                  disabled={rebuildRoutes.isPending}
                >
                  {rebuildRoutes.isPending ? (
                    <Spinner />
                  ) : (
                    <ZapIcon className='size-4' />
                  )}
                  {t('import.done.rebuildRoutes')}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={discardConfirmOpen}
        onOpenChange={(next) => {
          if (!next) setDiscardConfirmOpen(false)
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('import.discard.title')}</DialogTitle>
            <DialogDescription>
              {t('import.discard.description')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDiscardConfirmOpen(false)}
            >
              {t('import.discard.keepEditing')}
            </Button>
            <Button variant='destructive' onClick={confirmDiscard}>
              {t('import.discard.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
