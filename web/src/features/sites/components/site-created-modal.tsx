// metapi-go/features/sites — post-create guidance modal.
//
// Continue to the credential flow appropriate for the selected adapter.

import { useNavigate } from '@tanstack/react-router'
import {
  ArrowRight as ArrowRightIcon,
  CheckCircle2 as CheckCircle2Icon,
  KeyRound as KeyRoundIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PlatformBadge } from '@/components/common/platform-badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { getPlatformDefinition } from '@/lib/platform-catalog'

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
  const group = getPlatformDefinition(site?.platform)?.group

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

        {site && (
          <div className='bg-muted/30 flex flex-wrap items-center gap-2 rounded-lg border p-3 text-sm'>
            <PlatformBadge platform={site.platform} />
            <span className='text-muted-foreground min-w-0 break-all'>
              {site.url}
            </span>
          </div>
        )}

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('sites.created.dismiss')}
          </Button>
          {group === 'oauth' ? (
            <Button
              onClick={() => {
                onOpenChange(false)
                navigate({ to: '/oauth' })
              }}
            >
              {t('sites.created.goToOAuth')}
              <ArrowRightIcon className='size-4' />
            </Button>
          ) : (
            <>
              <Button
                variant={group === 'api' ? 'default' : 'outline'}
                onClick={handleGoToAddApiKey}
                disabled={!site}
              >
                <KeyRoundIcon className='size-4' />
                {t('sites.created.addApiKey')}
              </Button>
              {group !== 'api' && (
                <Button onClick={handleGoToAccounts} disabled={!site}>
                  {t('sites.created.goToAccounts')}
                  <ArrowRightIcon className='size-4' />
                </Button>
              )}
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
