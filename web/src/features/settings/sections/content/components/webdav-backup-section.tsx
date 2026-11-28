import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Cloud } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { useReauthReplay } from '@/components/common/reauth-provider'
import { SectionError } from '@/components/common/section-error'
import { Form } from '@/components/ui/form'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { SettingsSubsection } from '../../../components/settings-subsection'
import { useWebdavConfig } from './use-webdav-config'
import { WebdavConfigFields } from './webdav-config-fields'
import { WebdavSyncActions } from './webdav-sync-actions'

const WEBDAV_FORM_ID = 'settings-content-import-export-webdav-form'

export function WebdavBackupSection({
  onExport,
  exportPending,
}: {
  onExport: () => void
  exportPending: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const handleReauth = useReauthReplay()
  const [confirmWebdavImportOpen, setConfirmWebdavImportOpen] = useState(false)
  const { form, config, webdavQuery, onWebdavSubmit, reset, savePending } =
    useWebdavConfig(handleReauth)
  const importWebdavMutation = useMutation({
    mutationFn: async (input: { confirmToken?: string }) =>
      input.confirmToken
        ? api.importBackupFromWebdav(input.confirmToken)
        : api.importBackupFromWebdav(),
    onSuccess: () => {
      toast.success(t('settings.content.importExport.toast.webdavImported'))
      void queryClient.invalidateQueries()
    },
    onError: (error, input) => {
      if (
        handleReauth(
          error,
          t('reauth.operations.backupWebdavImport'),
          (confirmToken) =>
            importWebdavMutation.mutateAsync({ ...input, confirmToken })
        )
      ) {
        return
      }
      toast.error(t('settings.content.importExport.toast.webdavImportFailed'))
    },
  })

  const isWebdavDirty = form.formState.isDirty
  return (
    <>
      {webdavQuery.isLoading ? (
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      ) : null}
      {!webdavQuery.isLoading && (webdavQuery.isError || !config) ? (
        <SectionError
          title={t('settings.content.importExport.webdavGroup')}
          messageKey='settings.common.loadFailed'
          onRetry={() => void webdavQuery.refetch()}
        />
      ) : null}
      {!webdavQuery.isLoading && !webdavQuery.isError && config ? (
        <Form {...form}>
          <form
            id={WEBDAV_FORM_ID}
            onSubmit={form.handleSubmit(onWebdavSubmit)}
            className='bg-muted/20 space-y-4 rounded-xl border p-5'
          >
            <SettingsSubsection
              title={t('settings.content.importExport.webdavGroup')}
              description={t('settings.content.importExport.design.webdavHint')}
              icon={<Cloud className='size-5' />}
            >
              <WebdavConfigFields />
              <SettingsFormActions
                formId={WEBDAV_FORM_ID}
                isDirty={isWebdavDirty}
                isPending={savePending}
                onReset={reset}
                saveLabel={t('settings.content.importExport.saveWebdav')}
              />
              <WebdavSyncActions
                exportPending={exportPending}
                onExport={onExport}
                importPending={importWebdavMutation.isPending}
                onImport={() => setConfirmWebdavImportOpen(true)}
                state={webdavQuery.data?.state}
              />
            </SettingsSubsection>
          </form>
        </Form>
      ) : null}
      <FormNavigationGuard enabled={isWebdavDirty} />
      <ConfirmDialog
        open={confirmWebdavImportOpen}
        title={t('settings.content.importExport.webdavImportConfirmTitle')}
        description={t(
          'settings.content.importExport.webdavImportConfirmDescription'
        )}
        confirmLabel={t('settings.content.importExport.importFromWebdav')}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmWebdavImportOpen(false)
          importWebdavMutation.mutate({})
        }}
        onCancel={() => setConfirmWebdavImportOpen(false)}
      />
    </>
  )
}
