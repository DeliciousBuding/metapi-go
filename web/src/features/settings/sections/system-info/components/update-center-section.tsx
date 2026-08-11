// metapi-go/features/settings/sections/system-info/components — update
// center section (UC-1). Read-only residual status display + external links
// to the public GitHub repo and GHCR package. Per the rewrite policy there
// is no in-app registry/deploy surface; ops use the CLI.

import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'

import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { cn } from '@/lib/utils'

type UpdateCenterStatus = {
  currentVersion?: string
  latestVersion?: string
  updateAvailable?: boolean
  residual?: string
  mode?: string
}

const updateCenterQueryKeys = {
  all: ['update-center-status'] as const,
}

const REPO_URL = 'https://github.com/DeliciousBuding/metapi-go'
const RELEASES_URL = `${REPO_URL}/releases`
const GHCR_URL = 'https://github.com/DeliciousBuding/metapi-go/pkgs/container/metapi-go'

export function UpdateCenterSection() {
  const { t } = useTranslation()

  const statusQuery = useQuery<UpdateCenterStatus>({
    queryKey: updateCenterQueryKeys.all,
    queryFn: async () => (await api.getUpdateCenterStatus()) as UpdateCenterStatus,
    staleTime: 60 * 1000,
  })

  const status = statusQuery.data ?? {}

  return (
    <SettingsSectionCard
      title={t('settings.systemInfo.updateCenter.title')}
      description={t('settings.systemInfo.updateCenter.description')}
    >
      {statusQuery.isLoading ? (
        <SettingsSectionSkeleton />
      ) : (
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center gap-4'>
            <div>
              <div className='text-xs text-muted-foreground'>
                {t('settings.systemInfo.updateCenter.currentVersion')}
              </div>
              <code className='text-sm'>
                {status.currentVersion ?? t('settings.systemInfo.updateCenter.unknown')}
              </code>
            </div>
            {status.latestVersion ? (
              <div>
                <div className='text-xs text-muted-foreground'>
                  {t('settings.systemInfo.updateCenter.latestVersion')}
                </div>
                <code className='text-sm'>{status.latestVersion}</code>
              </div>
            ) : null}
            {status.updateAvailable !== undefined ? (
              <Badge variant={status.updateAvailable ? 'default' : 'secondary'}>
                {status.updateAvailable
                  ? t('settings.systemInfo.updateCenter.updateAvailable')
                  : t('settings.systemInfo.updateCenter.upToDate')}
              </Badge>
            ) : null}
            {status.mode ? (
              <Badge variant='secondary'>
                {t('settings.systemInfo.updateCenter.mode')}: {status.mode}
              </Badge>
            ) : null}
          </div>

          {status.residual ? (
            <p className='text-xs text-muted-foreground'>{status.residual}</p>
          ) : null}

          <div className='flex flex-wrap gap-2'>
            <a
              href={REPO_URL}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
            >
              {t('settings.systemInfo.updateCenter.repoLink')}
            </a>
            <a
              href={RELEASES_URL}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
            >
              {t('settings.systemInfo.updateCenter.releasesLink')}
            </a>
            <a
              href={GHCR_URL}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
            >
              {t('settings.systemInfo.updateCenter.ghcrLink')}
            </a>
          </div>

          <p className='text-xs text-muted-foreground'>
            {t('settings.systemInfo.updateCenter.opsNote')}
          </p>
        </div>
      )}
    </SettingsSectionCard>
  )
}
