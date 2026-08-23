// metapi-go/features/sites — post-create guidance modal.
//
// This is the FIRST step of the guided "site → account → route" config
// chain (research §4.2). On successful `addSite`, the form dialog hands the
// created site to this modal. Two primary CTAs mirror the TS original's
// three-branch guidance (minus the codex OAuth branch, which the go version
// has no handler for): 「添加账号」 opens the account form in session mode
// with the site preselected, 「添加 API Key」 does the same but defaults the
// credential mode to `apikey` via the `segment=apikey` deep-link param.
// "稍后配置" is a ghost secondary so experts can skip — every page stays
// independently deep-linkable.

import { useNavigate } from '@tanstack/react-router'
import {
  ArrowRight as ArrowRightIcon,
  CheckCircle2 as CheckCircle2Icon,
  KeyRound as KeyRoundIcon,
} from 'lucide-react'
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

import type { Site } from '../types'

type SiteCreatedModalProps = {
  site: Site | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SiteCreatedModal({
  site,
  open,
  onOpenChange,
}: SiteCreatedModalProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  function handleGoToAccounts() {
    if (!site) return
    onOpenChange(false)
    navigate({
      to: '/accounts',
      search: { siteId: site.id, create: true },
      replace: true,
    })
  }

  function handleGoToAddApiKey() {
    if (!site) return
    onOpenChange(false)
    navigate({
      to: '/accounts',
      search: { siteId: site.id, create: true, segment: 'apikey' },
      replace: true,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <div className='text-primary bg-primary/10 mb-2 flex size-10 items-center justify-center rounded-full'>
            <CheckCircle2Icon className='size-5' />
          </div>
          <DialogTitle>{t('sites.created.title')}</DialogTitle>
          <DialogDescription>
            {site
              ? t('sites.created.description', { name: site.name })
              : t('sites.created.descriptionFallback')}
          </DialogDescription>
        </DialogHeader>

        <div className='bg-muted/50 rounded-lg border p-3 text-sm'>
          <p className='text-muted-foreground mb-1'>
            {t('sites.created.nextStepLabel')}
          </p>
          <p className='font-medium'>{t('sites.created.nextStepBody')}</p>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('sites.created.dismiss')}
          </Button>
          <Button
            variant='outline'
            onClick={handleGoToAddApiKey}
            disabled={!site}
          >
            <KeyRoundIcon className='size-4' />
            {t('sites.created.addApiKey')}
          </Button>
          <Button onClick={handleGoToAccounts} disabled={!site}>
            {t('sites.created.goToAccounts')}
            <ArrowRightIcon className='size-4' />
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
