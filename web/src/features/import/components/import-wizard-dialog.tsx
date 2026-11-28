import {
  Check as CheckIcon,
  Upload as UploadIcon,
  UserPlus as UserPlusIcon,
  Zap as ZapIcon,
} from 'lucide-react'
import { useState } from 'react'
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
import { Spinner } from '@/components/ui/spinner'

import { ImportWizardSteps } from './import-wizard-steps'
import { useImportAnnouncement } from './use-import-announcement'
import { useImportDraft } from './use-import-draft'
import { useImportExecution } from './use-import-execution'

type ImportWizardDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ImportWizardDialog({
  open,
  onOpenChange,
}: ImportWizardDialogProps) {
  const { t } = useTranslation()
  const draft = useImportDraft()
  const { step } = draft
  const execution = useImportExecution(
    () => draft.setStep('done'),
    () => {
      draft.reset()
      onOpenChange(false)
    }
  )
  const liveMessage = useImportAnnouncement(
    step,
    draft.candidates.length,
    draft.isDetecting,
    execution.result
  )
  const [discardConfirmOpen, setDiscardConfirmOpen] = useState(false)
  function handleOpenChange(next: boolean) {
    if (!next) {
      // Guard against silently dropping a partially filled wizard: any
      // source text, detected candidates, or a finished result counts as
      // input worth confirming.
      const hasInput =
        draft.sourceText.trim().length > 0 ||
        draft.candidates.length > 0 ||
        execution.result !== null
      if (hasInput) {
        setDiscardConfirmOpen(true)
        return
      }
      execution.finish()
      return
    }
    onOpenChange(next)
  }
  function confirmDiscard() {
    setDiscardConfirmOpen(false)
    execution.finish()
  }
  function handleSubmit() {
    const items = draft.validatedItems()
    if (items) void execution.submit(items)
  }
  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className='sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{t('import.title')}</DialogTitle>
            <DialogDescription>{t('import.description')}</DialogDescription>
          </DialogHeader>

          <ImportWizardSteps
            draft={draft}
            result={execution.result}
            isSubmitting={execution.isSubmitting}
          />
          <div aria-live='polite' className='sr-only'>
            {liveMessage}
          </div>
          <DialogFooter showCloseButton={false}>
            {step !== 'source' && step !== 'done' && (
              <Button type='button' variant='outline' onClick={draft.goBack}>
                {t('import.back')}
              </Button>
            )}
            {step === 'source' && (
              <Button type='button' onClick={draft.handleSourceNext}>
                <UploadIcon className='size-4' />
                {t('import.next')}
              </Button>
            )}
            {step === 'identify' && (
              <Button type='button' onClick={draft.handleIdentifyNext}>
                {t('import.next')}
              </Button>
            )}
            {step === 'connect' && (
              <Button type='button' onClick={() => draft.setStep('routes')}>
                {t('import.next')}
              </Button>
            )}
            {step === 'routes' && (
              <Button
                type='button'
                onClick={handleSubmit}
                disabled={execution.isSubmitting}
              >
                {execution.isSubmitting && <Spinner />}
                {t('import.submit')}
              </Button>
            )}
            {step === 'done' && (
              <>
                <Button
                  type='button'
                  variant='ghost'
                  onClick={execution.finish}
                >
                  <CheckIcon className='size-4' />
                  {t('import.done.close')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  onClick={execution.handleGoToAccounts}
                >
                  <UserPlusIcon className='size-4' />
                  {t('import.done.addAccount')}
                </Button>
                <Button
                  type='button'
                  onClick={execution.handleRebuildRoutes}
                  disabled={execution.isRebuilding}
                >
                  {execution.isRebuilding ? (
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
