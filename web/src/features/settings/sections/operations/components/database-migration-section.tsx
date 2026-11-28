// metapi-go/features/settings/sections/operations/components — database
// migration section. Queues a copy of the live runtime database onto an
// external sqlite/postgres target as an admin background task and polls
// /api/tasks/{id} for progress, replacing the retired CLI-only migration note.

import { TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
import { SectionSkeleton } from '@/components/common/section-skeleton'
import { Notice } from '@/components/ui/notice'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { DatabaseMigrationForm } from './database-migration-form'
import { DatabaseMigrationStatus } from './database-migration-status'
import { useDatabaseMigration } from './use-database-migration'
import { useDatabaseMigrationForm } from './use-database-migration-form'

export function DatabaseMigrationSection() {
  const { t } = useTranslation()
  const { form, configQuery } = useDatabaseMigrationForm()
  const migration = useDatabaseMigration(form)
  const activeConfig = configQuery.data?.active

  if (configQuery.isLoading) {
    return <SectionSkeleton />
  }
  if (configQuery.isError || !configQuery.data) {
    return (
      <SectionError
        title={t('settings.operations.database.migration.title')}
        messageKey='settings.common.loadFailed'
        onRetry={() => void configQuery.refetch()}
      />
    )
  }

  return (
    <SectionCard
      title={t('settings.operations.database.migration.title')}
      description={t('settings.operations.database.migration.description')}
    >
      <div className='space-y-5'>
        {/* Wipe/restart warning surfaced inline (it used to ride in the card
            description, which is now page-header-only for headerless cards). */}
        <Notice tone='warning'>
          <TriangleAlert className='text-warning mt-0.5 size-4 shrink-0' />
          <p className='text-muted-foreground text-xs'>
            {t('settings.operations.database.migration.warning')}
          </p>
        </Notice>

        {activeConfig ? (
          <div className='bg-muted/25 rounded-lg border p-3'>
            <p className='text-muted-foreground text-xs font-medium'>
              {t('settings.operations.database.migration.sourceLabel')}
            </p>
            <code className='text-foreground mt-1 block text-xs break-all'>
              {activeConfig.dialect} · {activeConfig.connection}
              {activeConfig.ssl ? ' · SSL' : ''}
            </code>
          </div>
        ) : null}

        <DatabaseMigrationForm
          form={form}
          busy={migration.busy}
          testing={migration.testing}
          migrating={migration.migrating}
          onTestConnection={migration.handleTestConnection}
          onStartMigration={migration.handleStartMigration}
        />
        {migration.task && <DatabaseMigrationStatus task={migration.task} />}
      </div>

      <ConfirmDialog
        open={migration.confirmOpen}
        title={t('settings.operations.database.migration.confirm.title')}
        description={t(
          'settings.operations.database.migration.confirm.description'
        )}
        confirmLabel={t(
          'settings.operations.database.migration.confirm.confirmLabel'
        )}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onCancel={() => migration.setConfirmOpen(false)}
        onConfirm={migration.confirmMigration}
      />

      <FormNavigationGuard enabled={form.formState.isDirty} />
    </SectionCard>
  )
}
