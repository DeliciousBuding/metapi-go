// metapi-go/features/sites — post-create guidance modal.
//
// This is the FIRST step of the guided "site → account → route" config
// chain (research §4.2). On successful `addSite`, the form dialog hands the
// created site to this modal. The primary CTA deep-links into the accounts
// page with `siteId` so the account form can pre-fill the site binding (and
// later carry the init preset). "稍后配置" is a ghost secondary so experts
// can skip — every page stays independently deep-linkable.

import { useNavigate } from '@tanstack/react-router'
import {
  ArrowRight as ArrowRightIcon,
  CheckCircle2 as CheckCircle2Icon,
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

/**
 * Build the accounts-page deep link. Uses the untyped `href` overload so it
 * works whether or not the `/accounts` route is registered in the typed
 * route tree yet (the accounts feature may land in a later phase).
 */
function buildAccountsHref(siteId: number): string {
  const params = new URLSearchParams({
    siteId: String(siteId),
    create: '1',
  })
  return `/accounts?${params.toString()}`
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
    navigate({ href: buildAccountsHref(site.id), replace: true })
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
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            {t('sites.created.dismiss')}
          </Button>
          <Button onClick={handleGoToAccounts} disabled={!site}>
            {t('sites.created.goToAccounts')}
            <ArrowRightIcon className='ml-1 size-4' />
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
