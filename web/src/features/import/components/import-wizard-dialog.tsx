// metapi-go/features/import — unified five-step import wizard dialog.
//
// source → identify → connect → models/routes → done. The wizard collects
// pasted URLs, auto-detects each platform (unknown stays manually specifiable),
// flags duplicates with skip/merge, attaches optional accounts, sets routing
// weight, then commits one idempotent POST /api/sites/import and reports the
// imported/skipped/failed breakdown.

import { useNavigate } from '@tanstack/react-router'
import {
  Check as CheckIcon,
  Minus as MinusIcon,
  TriangleAlert as AlertIcon,
  Upload as UploadIcon,
  UserPlus as UserPlusIcon,
  X as XIcon,
  Zap as ZapIcon,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useSites } from '@/features/sites/api'
import type { Site } from '@/features/sites/types'
import { useRebuildRoutes } from '@/features/token-routes/api'

import { useDetectSite, useImportSites } from '../api'
import { canonicalizeUrl, parseUrlLines } from '../lib/utils'
import type {
  ImportCandidate,
  ImportDuplicateStrategy,
  ImportSiteItem,
  ImportSiteResultStatus,
  ImportSitesResult,
  ImportStepId,
} from '../types'
import { ImportStepper, type ImportStep } from './stepper'

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

function makeCandidate(url: string, index: number): ImportCandidate {
  return {
    id: `candidate-${index}-${url.slice(0, 32)}`,
    url,
    name: defaultSiteName(url),
    platform: '',
    confidence: null,
    detected: false,
    detecting: false,
    duplicateStrategy: 'skip',
    includeAccount: false,
    username: '',
    accessToken: '',
    apiToken: '',
    weight: 1,
  }
}

function defaultSiteName(url: string): string {
  try {
    const withScheme = url.includes('://') ? url : `https://${url}`
    const parsed = new URL(withScheme)
    return parsed.hostname
  } catch {
    return url
  }
}

function statusBadge(status: ImportSiteResultStatus) {
  if (status === 'imported') {
    return {
      variant: 'outline' as const,
      icon: CheckIcon,
      label: 'import.result.imported',
    }
  }
  if (status === 'merged') {
    return {
      variant: 'outline' as const,
      icon: CheckIcon,
      label: 'import.result.merged',
    }
  }
  if (status === 'failed') {
    return {
      variant: 'destructive' as const,
      icon: XIcon,
      label: 'import.result.failed',
    }
  }
  return {
    variant: 'secondary' as const,
    icon: MinusIcon,
    label: 'import.result.skipped',
  }
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

  // Fields flagged invalid by a failed validation pass. Cleared per-field
  // the moment the user edits that field, so an error never outlives the
  // input that caused it. The single source of truth for aria-invalid on
  // wizard inputs — no new error visuals, the design system already wires
  // aria-invalid styling into Input/Textarea.
  const [invalidFields, setInvalidFields] = useState<Set<string>>(
    () => new Set()
  )

  // Live registry of focusable field elements keyed by a stable field id
  // ("source", `${candidate.id}-platform`, `${candidate.id}-weight`). Ref
  // callbacks keep this current on mount/unmount so the focus target for
  // "first invalid field" is always the committed DOM node. Callbacks are
  // cached per key so React does not tear down and reattach on each render.
  const fieldRefs = useRef<Map<string, HTMLElement>>(new Map())
  const fieldRefCallbacks = useRef<
    Map<string, (element: HTMLElement | null) => void>
  >(new Map())
  function registerFieldRef(key: string) {
    let callback = fieldRefCallbacks.current.get(key)
    if (callback === undefined) {
      callback = (element) => {
        if (element) {
          fieldRefs.current.set(key, element)
        } else {
          fieldRefs.current.delete(key)
        }
      }
      fieldRefCallbacks.current.set(key, callback)
    }
    return callback
  }

  // Mark the given field keys invalid and move focus to the first one, so
  // the user lands directly on the error they need to fix. Returns false
  // (no-op) when no focus target is registered yet — e.g. the field has
  // not mounted, which never happens for the step being validated.
  function markInvalidAndFocusFirst(keys: string[]): boolean {
    setInvalidFields(new Set(keys))
    const firstKey = keys[0]
    const target = firstKey ? fieldRefs.current.get(firstKey) : undefined
    if (target) {
      target.focus()
      return true
    }
    return false
  }

  function clearFieldInvalid(key: string) {
    setInvalidFields((prev) => {
      if (!prev.has(key)) return prev
      const next = new Set(prev)
      next.delete(key)
      return next
    })
  }

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
    setInvalidFields(new Set())
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
    setInvalidFields(new Set())
    const initial = urls.map((url, index) => makeCandidate(url, index))
    setCandidates(initial)
    setStep('identify')
    void detectAll(initial)
  }

  function isDuplicate(candidate: ImportCandidate): boolean {
    return existingUrls.has(canonicalizeUrl(candidate.url))
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
    setInvalidFields(new Set())
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
    setInvalidFields(new Set())

    const items: ImportSiteItem[] = candidates.map((candidate) => {
      const accessToken = candidate.accessToken.trim()
      const apiToken = candidate.apiToken.trim()
      const hasAccount =
        candidate.includeAccount && (accessToken !== '' || apiToken !== '')
      return {
        name: candidate.name.trim() || candidate.url,
        url: candidate.url,
        platform: candidate.platform || undefined,
        globalWeight: candidate.weight,
        duplicateStrategy: candidate.duplicateStrategy,
        accounts: hasAccount
          ? [
              {
                username: candidate.username.trim() || undefined,
                accessToken,
                apiToken,
              },
            ]
          : [],
      }
    })

    try {
      const importResult = await importSites.mutateAsync({
        items,
        duplicateStrategy: 'skip',
      })
      setResult(importResult)
      setStep('done')
    } catch {
      // http-client toasted
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

  function handleGoToAccounts() {
    reset()
    onOpenChange(false)
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
                  ref={registerFieldRef('source')}
                  aria-invalid={invalidFields.has('source') ? true : undefined}
                  rows={8}
                  value={sourceText}
                  onChange={(event) => {
                    setSourceText(event.target.value)
                    if (invalidFields.has('source')) clearFieldInvalid('source')
                  }}
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
              <div className='grid gap-3'>
                {candidates.map((candidate) => {
                  const duplicate = isDuplicate(candidate)
                  return (
                    <div
                      key={candidate.id}
                      className='grid gap-3 rounded-lg border p-3'
                    >
                      <div className='grid gap-2'>
                        <Label htmlFor={`${candidate.id}-name`}>
                          {t('import.identify.name')}
                        </Label>
                        <Input
                          id={`${candidate.id}-name`}
                          value={candidate.name}
                          onChange={(event) =>
                            updateCandidate(candidate.id, {
                              name: event.target.value,
                            })
                          }
                        />
                      </div>
                      <div className='text-muted-foreground truncate font-mono text-xs'>
                        {candidate.url}
                      </div>
                      <div className='grid gap-2'>
                        <Label htmlFor={`${candidate.id}-platform`}>
                          {t('import.identify.platform')}
                        </Label>
                        <div className='flex items-center gap-2'>
                          <Input
                            id={`${candidate.id}-platform`}
                            ref={registerFieldRef(`${candidate.id}-platform`)}
                            aria-invalid={
                              invalidFields.has(`${candidate.id}-platform`)
                                ? true
                                : undefined
                            }
                            value={candidate.platform}
                            onChange={(event) => {
                              updateCandidate(candidate.id, {
                                platform: event.target.value,
                                detected: false,
                                confidence: null,
                              })
                              if (
                                invalidFields.has(`${candidate.id}-platform`)
                              ) {
                                clearFieldInvalid(`${candidate.id}-platform`)
                              }
                            }}
                            className='flex-1'
                          />
                          {candidate.detecting && <Spinner />}
                          {!candidate.detecting && candidate.detected && (
                            <Badge variant='secondary'>
                              <CheckIcon className='size-3' />
                              {t('import.identify.detected', {
                                confidence: Math.round(
                                  (candidate.confidence ?? 0) * 100
                                ),
                              })}
                            </Badge>
                          )}
                        </div>
                      </div>

                      {duplicate && (
                        <div className='border-warning/40 bg-warning/10 rounded-lg border p-3'>
                          <div className='mb-2 flex items-center gap-2 text-sm'>
                            <AlertIcon className='text-warning size-4' />
                            <span>{t('import.identify.duplicateWarning')}</span>
                          </div>
                          <RadioGroup
                            value={candidate.duplicateStrategy}
                            onValueChange={(value) =>
                              updateCandidate(candidate.id, {
                                duplicateStrategy:
                                  value as ImportDuplicateStrategy,
                              })
                            }
                          >
                            <div className='flex items-center gap-2'>
                              <RadioGroupItem
                                id={`${candidate.id}-skip`}
                                value='skip'
                              />
                              <Label htmlFor={`${candidate.id}-skip`}>
                                {t('import.identify.skip')}
                              </Label>
                            </div>
                            <div className='flex items-center gap-2'>
                              <RadioGroupItem
                                id={`${candidate.id}-merge`}
                                value='merge'
                              />
                              <Label htmlFor={`${candidate.id}-merge`}>
                                {t('import.identify.merge')}
                              </Label>
                            </div>
                          </RadioGroup>
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            )}

            {step === 'connect' && (
              <div className='grid gap-3'>
                {candidates.map((candidate) => (
                  <div
                    key={candidate.id}
                    className='grid gap-3 rounded-lg border p-3'
                  >
                    <div className='flex items-center justify-between gap-3'>
                      <div className='min-w-0'>
                        <div className='truncate text-sm font-medium'>
                          {candidate.name}
                        </div>
                        <div className='text-muted-foreground truncate text-xs'>
                          {candidate.url}
                        </div>
                      </div>
                      <Switch
                        checked={candidate.includeAccount}
                        onCheckedChange={(checked) =>
                          updateCandidate(candidate.id, {
                            includeAccount: checked,
                          })
                        }
                        aria-label={t('import.connect.includeAccount', {
                          name: candidate.name,
                        })}
                      />
                    </div>
                    {candidate.includeAccount && (
                      <div className='grid gap-2 sm:grid-cols-3'>
                        <div className='grid gap-1.5'>
                          <Label htmlFor={`${candidate.id}-username`}>
                            {t('import.connect.username')}
                          </Label>
                          <Input
                            id={`${candidate.id}-username`}
                            value={candidate.username}
                            onChange={(event) =>
                              updateCandidate(candidate.id, {
                                username: event.target.value,
                              })
                            }
                          />
                        </div>
                        <div className='grid gap-1.5'>
                          <Label htmlFor={`${candidate.id}-accessToken`}>
                            {t('import.connect.accessToken')}
                          </Label>
                          <Input
                            id={`${candidate.id}-accessToken`}
                            type='password'
                            value={candidate.accessToken}
                            onChange={(event) =>
                              updateCandidate(candidate.id, {
                                accessToken: event.target.value,
                              })
                            }
                          />
                        </div>
                        <div className='grid gap-1.5'>
                          <Label htmlFor={`${candidate.id}-apiToken`}>
                            {t('import.connect.apiToken')}
                          </Label>
                          <Input
                            id={`${candidate.id}-apiToken`}
                            value={candidate.apiToken}
                            onChange={(event) =>
                              updateCandidate(candidate.id, {
                                apiToken: event.target.value,
                              })
                            }
                          />
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}

            {step === 'routes' && (
              <div className='grid gap-3'>
                <p className='text-muted-foreground text-xs'>
                  {t('import.routes.hint')}
                </p>
                {candidates.map((candidate) => (
                  <div
                    key={candidate.id}
                    className='flex items-center justify-between gap-3 rounded-lg border p-3'
                  >
                    <div className='min-w-0'>
                      <div className='truncate text-sm font-medium'>
                        {candidate.name}
                      </div>
                      <div className='text-muted-foreground truncate text-xs'>
                        {candidate.platform}
                      </div>
                    </div>
                    <div className='flex items-center gap-2'>
                      <Label htmlFor={`${candidate.id}-weight`}>
                        {t('import.routes.weight')}
                      </Label>
                      <Input
                        id={`${candidate.id}-weight`}
                        ref={registerFieldRef(`${candidate.id}-weight`)}
                        aria-invalid={
                          invalidFields.has(`${candidate.id}-weight`)
                            ? true
                            : undefined
                        }
                        type='number'
                        min={0}
                        step={0.1}
                        value={candidate.weight}
                        onChange={(event) => {
                          updateCandidate(candidate.id, {
                            weight: Number(event.target.value),
                          })
                          if (invalidFields.has(`${candidate.id}-weight`)) {
                            clearFieldInvalid(`${candidate.id}-weight`)
                          }
                        }}
                        className='w-24'
                      />
                    </div>
                  </div>
                ))}
              </div>
            )}

            {step === 'done' && result && (
              <div className='grid gap-4'>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge variant='outline'>
                    <CheckIcon className='size-3' />
                    {t('import.done.imported', { count: result.imported })}
                  </Badge>
                  <Badge variant='secondary'>
                    <MinusIcon className='size-3' />
                    {t('import.done.skipped', { count: result.skipped })}
                  </Badge>
                  <Badge variant='destructive'>
                    <XIcon className='size-3' />
                    {t('import.done.failed', { count: result.failed })}
                  </Badge>
                </div>
                <p className='text-muted-foreground text-xs'>
                  {t('import.done.rebuildHint')}
                </p>
                <div className='grid gap-2'>
                  {result.results.map((item) => {
                    const badge = statusBadge(item.status)
                    const Icon = badge.icon
                    const showReason =
                      item.reason != null &&
                      item.reason !== '' &&
                      (item.status === 'failed' || item.status === 'skipped')
                    return (
                      <div
                        key={`${item.url}-${item.status}`}
                        className='flex items-start justify-between gap-3 rounded-lg border p-2.5'
                      >
                        <div className='min-w-0'>
                          <div className='truncate text-sm'>{item.name}</div>
                          <div className='text-muted-foreground truncate text-xs'>
                            {item.url}
                          </div>
                        </div>
                        <div className='flex flex-col items-end gap-1'>
                          <Badge variant={badge.variant}>
                            <Icon className='size-3' />
                            {t(badge.label)}
                          </Badge>
                          {showReason && (
                            <span
                              className={
                                item.status === 'failed'
                                  ? 'text-destructive text-xs'
                                  : 'text-muted-foreground text-xs'
                              }
                            >
                              {t('import.result.reason', {
                                reason: item.reason,
                              })}
                            </span>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}
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
