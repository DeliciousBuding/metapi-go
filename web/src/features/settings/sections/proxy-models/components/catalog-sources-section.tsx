import { Plus as PlusIcon, RefreshCw as RefreshCwIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { SectionCard } from '@/components/common/section-card'
import { SectionSkeleton } from '@/components/common/section-skeleton'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { toBcp47 } from '@/i18n/languages'
import type { CatalogSyncStatus } from '@/lib/api'
import { formatDateTime } from '@/lib/format'

import { SourceDialog, type SourceDialogState } from './catalog-source-dialog'
import { CatalogSourceTable } from './catalog-source-table'
import { useCatalogSources } from './use-catalog-sources'

export function CatalogSourcesSection() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const [dialog, setDialog] = useState<SourceDialogState | null>(null)
  const {
    statusQuery,
    syncingId,
    sync,
    autoSyncMutation,
    sourceActions,
    reloadSources,
  } = useCatalogSources()
  if (statusQuery.isLoading) {
    return <SectionSkeleton />
  }
  if (statusQuery.error) {
    return (
      <SectionCard
        title={t('settings.proxyModels.catalogSources.title')}
        description={t('settings.proxyModels.catalogSources.description')}
      >
        <QueryErrorBanner
          error={statusQuery.error}
          messageKey='settings.proxyModels.catalogSources.loadError'
          onRetry={() => void statusQuery.refetch()}
          isRetrying={statusQuery.isFetching}
        />
      </SectionCard>
    )
  }

  const status = statusQuery.data as CatalogSyncStatus
  const sources = status.sources ?? []
  const runtimeEnabled = status.enabled !== false
  const busy = syncingId !== null
  return (
    <div className='flex flex-col gap-4'>
      <SectionCard
        title={t('settings.proxyModels.catalogSources.title')}
        description={t('settings.proxyModels.catalogSources.description')}
        hideHeaderCopy
        actions={
          <>
            <Button
              size='sm'
              variant='outline'
              onClick={() => setDialog({ mode: 'new' })}
            >
              <PlusIcon className='size-3.5' />
              {t('settings.proxyModels.catalogSources.add')}
            </Button>
            <Button
              size='sm'
              onClick={() => sync('all')}
              disabled={busy || !runtimeEnabled}
            >
              {syncingId === 'all' ? (
                <Spinner />
              ) : (
                <RefreshCwIcon className='size-3.5' />
              )}
              {syncingId === 'all'
                ? t('settings.proxyModels.catalogSources.syncAllPending')
                : t('settings.proxyModels.catalogSources.syncAll')}
            </Button>
          </>
        }
      >
        <div className='flex flex-col gap-4'>
          {!runtimeEnabled && (
            <Notice tone='info' className='block'>
              <p className='font-medium'>
                {t('settings.proxyModels.catalogSources.runtimeDisabled')}
              </p>
              <p className='mt-1'>
                {t('settings.proxyModels.catalogSources.runtimeDisabledHint')}{' '}
                <code className='[overflow-wrap:anywhere]'>
                  PRICING_CATALOG_ENABLED=true
                </code>
              </p>
            </Notice>
          )}
          <div className='flex items-center justify-between gap-4 rounded-md border p-3'>
            <div className='flex flex-col gap-0.5'>
              <Label htmlFor='catalog-auto-sync'>
                {t('settings.proxyModels.catalogSources.autoSync')}
              </Label>
              <p className='text-muted-foreground text-xs'>
                {t('settings.proxyModels.catalogSources.autoSyncHint')}
                {status.intervalMin > 0 ? ` (${status.intervalMin} min)` : ''}
              </p>
            </div>
            <Switch
              id='catalog-auto-sync'
              checked={status.autoSync}
              onCheckedChange={(checked) => autoSyncMutation.mutate(checked)}
              disabled={autoSyncMutation.isPending || !runtimeEnabled}
              aria-label={t('settings.proxyModels.catalogSources.autoSync')}
            />
          </div>

          {runtimeEnabled && (
            <div className='rounded-md border p-3'>
              <h3 className='text-sm font-medium'>
                {t('settings.proxyModels.catalogSources.snapshot.title')}
              </h3>
              <dl className='mt-2 grid grid-cols-1 gap-2 text-sm sm:grid-cols-3'>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('settings.proxyModels.catalogSources.snapshot.source')}
                  </dt>
                  <dd className='truncate' title={status.snapshot.source}>
                    {status.snapshot.source || '—'}
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t(
                      'settings.proxyModels.catalogSources.snapshot.fetchedAt'
                    )}
                  </dt>
                  <dd>
                    {status.snapshot.fetchedAt
                      ? formatDateTime(status.snapshot.fetchedAt, locale)
                      : t('settings.proxyModels.catalogSources.snapshot.never')}
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('settings.proxyModels.catalogSources.snapshot.models')}
                  </dt>
                  <dd className='tabular-nums'>{status.snapshot.models}</dd>
                </div>
              </dl>
            </div>
          )}

          <CatalogSourceTable
            sources={sources}
            syncingId={syncingId}
            runtimeEnabled={runtimeEnabled}
            actions={sourceActions}
            onSync={sync}
            onEdit={(source) => setDialog({ mode: 'edit', source })}
          />
        </div>
      </SectionCard>

      {dialog ? (
        <SourceDialog
          state={dialog}
          onOpenChange={(open) => {
            if (!open) setDialog(null)
          }}
          onSaved={() => {
            setDialog(null)
            void reloadSources()
          }}
        />
      ) : null}
    </div>
  )
}
