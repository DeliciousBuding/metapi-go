// metapi-go/features/sites — site detail Sheet (side panel).
//
// Opens from the row "view details" action. Shows the site's static config
// (url / platform / status / probe / endpoints / tags / timestamps) and
// surfaces the two follow-on CTAs of the guided config chain:
//   - 管理账号 → /accounts?siteId=…&create=1 (step 2)
//   - 管理路由 → /token-routes?siteId=… (step 3)

import { useNavigate } from '@tanstack/react-router'
import {
  ArrowRight as ArrowRightIcon,
  CheckCircle2 as CheckCircle2Icon,
  PauseCircle as PauseCircleIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PlatformBadge } from '@/components/common/platform-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { toBcp47 } from '@/i18n/languages'

import type { Site, SiteStatus } from '../types'
import { SiteBalanceSection } from './site-detail-balance'
import { SiteDetailConfiguration } from './site-detail-configuration'
import { SiteDetailEndpoints } from './site-detail-endpoints'
import { SiteProbePanel } from './site-probe-panel'

type SiteDetailSheetProps = {
  site: Site | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onEdit?: (site: Site) => void
}

const STATUS_LABEL_KEY: Record<SiteStatus, string> = {
  active: 'sites.status.active',
  disabled: 'sites.status.disabled',
}

export function SiteDetailSheet({
  site,
  open,
  onOpenChange,
  onEdit,
}: SiteDetailSheetProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const locale = toBcp47(i18n.language || 'en')

  if (!site) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' />
      </Sheet>
    )
  }

  const status: SiteStatus = site.status === 'disabled' ? 'disabled' : 'active'
  const endpoints = site.apiEndpoints ?? []
  const tags = site.tags ?? []
  const siteId = site.id

  function goToAccounts() {
    onOpenChange(false)
    navigate({
      to: '/accounts',
      search: { siteId, create: true },
      replace: true,
    })
  }

  function goToRoutes() {
    onOpenChange(false)
    navigate({
      to: '/token-routes',
      search: { siteId },
      replace: true,
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='sm:max-w-md'>
        <SheetHeader>
          <SheetTitle className='pr-6'>{site.name}</SheetTitle>
          <SheetDescription className='truncate' title={site.url}>
            {site.url}
          </SheetDescription>
        </SheetHeader>

        <div className='flex flex-col gap-4 overflow-y-auto px-4 pb-4'>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge variant={status === 'active' ? 'success' : 'secondary'}>
              {status === 'active' ? (
                <CheckCircle2Icon className='size-3' aria-hidden='true' />
              ) : (
                <PauseCircleIcon className='size-3' aria-hidden='true' />
              )}
              {t(STATUS_LABEL_KEY[status])}
            </Badge>
            {site.isPinned && (
              <Badge variant='outline'>{t('sites.detail.pinned')}</Badge>
            )}
            {site.platform && <PlatformBadge platform={site.platform} />}
          </div>

          <Separator />

          <SiteBalanceSection site={site} locale={locale} />

          <SiteProbePanel key={site.id} siteId={site.id} />

          <Separator />

          <SiteDetailConfiguration site={site} />
          <SiteDetailEndpoints endpoints={endpoints} locale={locale} />

          {tags.length > 0 && (
            <>
              <Separator />
              <section>
                <h3 className='text-sm font-medium'>
                  {t('sites.detail.tags')}
                </h3>
                <div className='mt-2 flex flex-wrap gap-1.5'>
                  {tags.map((tag) => (
                    <Badge key={tag} variant='outline'>
                      {tag}
                    </Badge>
                  ))}
                </div>
              </section>
            </>
          )}

          <Separator />

          <section className='flex flex-col gap-2'>
            <Button variant='outline' onClick={goToAccounts}>
              {t('sites.detail.manageAccounts')}
              <ArrowRightIcon className='size-4' />
            </Button>
            <Button variant='outline' onClick={goToRoutes}>
              {t('sites.detail.manageRoutes')}
              <ArrowRightIcon className='size-4' />
            </Button>
            {onEdit && (
              <Button variant='ghost' onClick={() => onEdit(site)}>
                {t('sites.detail.editSite')}
              </Button>
            )}
          </section>
        </div>
      </SheetContent>
    </Sheet>
  )
}
