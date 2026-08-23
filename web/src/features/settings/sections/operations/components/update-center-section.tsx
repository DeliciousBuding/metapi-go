// metapi-go/features/settings/sections/operations/components — update
// center section (UC-1). Read-only residual status display + external links
// to the public GitHub repo and GHCR package. Per the rewrite policy there
// is no in-app registry/deploy surface; ops use the CLI.

import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'

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
const GHCR_URL =
  'https://github.com/DeliciousBuding/metapi-go/pkgs/container/metapi-go'

export function UpdateCenterSection() {
  const { t } = useTranslation()

  const statusQuery = useQuery<UpdateCenterStatus>({
    queryKey: updateCenterQueryKeys.all,
    queryFn: async () =>
      (await api.getUpdateCenterStatus()) as UpdateCenterStatus,
    staleTime: 60 * 1000,
  })

  const status = statusQuery.data ?? {}
  const currentVersion =
    status.currentVersion && status.currentVersion !== '0.0.0'
      ? status.currentVersion
      : undefined
  const latestVersion =
    status.latestVersion && status.latestVersion !== '0.0.0'
      ? status.latestVersion
      : undefined
  const hasComparableVersions = Boolean(currentVersion && latestVersion)

  if (statusQuery.isError) {
    return (
      <SettingsSectionError
        title={t('settings.operations.updateCenter.title')}
        onRetry={() => void statusQuery.refetch()}
      />
    )
  }

  return (
    <SettingsSectionCard
      title={t('settings.operations.updateCenter.title')}
      description={t('settings.operations.updateCenter.description')}
    >
      {statusQuery.isLoading ? (
        <SettingsSectionSkeleton />
      ) : (
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center gap-4'>
            <div>
              <div className='text-muted-foreground text-xs'>
                {t('settings.operations.updateCenter.currentVersion')}
              </div>
              <code className='text-sm'>
                {currentVersion ??
                  t('settings.operations.updateCenter.unknown')}
              </code>
            </div>
            {latestVersion ? (
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('settings.operations.updateCenter.latestVersion')}
                </div>
                <code className='text-sm'>{latestVersion}</code>
              </div>
            ) : null}
            {hasComparableVersions && status.updateAvailable !== undefined ? (
              <Badge variant={status.updateAvailable ? 'default' : 'secondary'}>
                {status.updateAvailable
                  ? t('settings.operations.updateCenter.updateAvailable')
                  : t('settings.operations.updateCenter.upToDate')}
              </Badge>
            ) : null}
            {status.mode ? (
              <Badge variant='secondary'>
                {t('settings.operations.updateCenter.mode')}:{' '}
                {status.mode === 'external'
                  ? t('settings.operations.updateCenter.modeExternal')
                  : status.mode}
              </Badge>
            ) : null}
          </div>

          {status.mode === 'external' || status.residual ? (
            <p className='text-muted-foreground text-xs'>
              {status.mode === 'external'
                ? t('settings.operations.updateCenter.externalResidual')
                : status.residual}
            </p>
          ) : null}

          <div className='flex flex-wrap gap-2'>
            <a
              href={REPO_URL}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
            >
              {t('settings.operations.updateCenter.repoLink')}
            </a>
            <a
              href={RELEASES_URL}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
            >
              {t('settings.operations.updateCenter.releasesLink')}
            </a>
            <a
              href={GHCR_URL}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
            >
              {t('settings.operations.updateCenter.ghcrLink')}
            </a>
          </div>

          <p className='text-muted-foreground text-xs'>
            {t('settings.operations.updateCenter.opsNote')}
          </p>
        </div>
      )}
    </SettingsSectionCard>
  )
}
