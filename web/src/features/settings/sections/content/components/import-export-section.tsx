import { useMutation } from '@tanstack/react-query'
import { Download, Database, Users, SlidersHorizontal } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ReauthDialog } from '@/components/common/reauth-dialog'
import { SectionCard } from '@/components/common/section-card'
import { Button } from '@/components/ui/button'
import { api, type BackupWebdavExportType } from '@/lib/api'
import { isReauthRequired } from '@/lib/http-client'
import { toast } from '@/lib/toast'

import { SettingsSubsection } from '../../../components/settings-subsection'
import { LocalBackupImport } from './local-backup-import'
import { WebdavBackupSection } from './webdav-backup-section'

function downloadTextFile(filename: string, text: string) {
  const blob = new Blob([text], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export function ImportExportSection() {
  const { t } = useTranslation()
  // #1034: backup export is a sensitive op — the operator must re-present
  // the master token before the export request is sent.
  const [reauthTarget, setReauthTarget] = useState<
    | { kind: 'download'; type: 'all' | 'accounts' | 'preferences' }
    | { kind: 'webdav' }
    | null
  >(null)
  const [reauthError, setReauthError] = useState<string | null>(null)

  const exportMutation = useMutation({
    mutationFn: async ({
      type,
      confirmToken,
    }: {
      type: BackupWebdavExportType
      confirmToken: string
    }) => {
      const text = await api.exportBackupRaw(type, confirmToken)
      return { type, text }
    },
    onSuccess: ({ type, text }) => {
      setReauthTarget(null)
      setReauthError(null)
      const today = new Date().toISOString().slice(0, 10)
      downloadTextFile(`metapi-${type}-${today}.json`, text)
      toast.success(t('settings.content.importExport.toast.exported'))
    },
    onError: (error: unknown) => {
      if (isReauthRequired(error)) {
        setReauthError(t('reauth.errorInvalid'))
        return
      }
      setReauthTarget(null)
      setReauthError(null)
      toast.error(t('settings.content.importExport.toast.exportFailed'))
    },
  })

  const exportWebdavMutation = useMutation({
    mutationFn: async ({
      type,
      confirmToken,
    }: {
      type?: BackupWebdavExportType
      confirmToken: string
    }) => api.exportBackupToWebdav(type, confirmToken),
    onSuccess: () => {
      setReauthTarget(null)
      setReauthError(null)
      toast.success(t('settings.content.importExport.toast.webdavExported'))
    },
    onError: (error: unknown) => {
      if (isReauthRequired(error)) {
        setReauthError(t('reauth.errorInvalid'))
        return
      }
      setReauthTarget(null)
      setReauthError(null)
      toast.error(t('settings.content.importExport.toast.webdavExportFailed'))
    },
  })

  return (
    <SectionCard
      title={t('settings.content.importExport.title')}
      description={t('settings.content.importExport.description')}
    >
      <div className='space-y-6'>
        <LocalBackupImport />
        <SettingsSubsection
          title={t('settings.content.importExport.exportGroup')}
          description={t('settings.content.importExport.design.exportHint')}
          icon={<Download className='size-5' />}
        >
          <div className='grid gap-3 sm:grid-cols-3'>
            {(
              [
                {
                  type: 'all',
                  label: 'exportAll',
                  detail: 'exportAllHint',
                  icon: Database,
                },
                {
                  type: 'accounts',
                  label: 'exportAccounts',
                  detail: 'exportAccountsHint',
                  icon: Users,
                },
                {
                  type: 'preferences',
                  label: 'exportPreferences',
                  detail: 'exportPreferencesHint',
                  icon: SlidersHorizontal,
                },
              ] as const
            ).map((item) => (
              <Button
                key={item.type}
                variant='outline'
                className='h-auto min-w-0 items-start justify-start gap-3 p-4 text-left whitespace-normal'
                disabled={exportMutation.isPending}
                onClick={() => {
                  setReauthError(null)
                  setReauthTarget({ kind: 'download', type: item.type })
                }}
              >
                <item.icon
                  className='text-primary mt-0.5 size-4 shrink-0'
                  aria-hidden='true'
                />
                <span className='min-w-0 space-y-1'>
                  <span className='block text-sm font-medium'>
                    {t(`settings.content.importExport.${item.label}`)}
                  </span>
                  <span className='text-muted-foreground block text-xs leading-relaxed font-normal'>
                    {t(`settings.content.importExport.design.${item.detail}`)}
                  </span>
                </span>
              </Button>
            ))}
          </div>
        </SettingsSubsection>

        <WebdavBackupSection
          exportPending={exportWebdavMutation.isPending}
          onExport={() => {
            setReauthError(null)
            setReauthTarget({ kind: 'webdav' })
          }}
        />
      </div>
      <ReauthDialog
        open={reauthTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setReauthTarget(null)
            setReauthError(null)
          }
        }}
        description={t('settings.content.importExport.reauthDescription')}
        errorMessage={reauthError}
        isPending={exportMutation.isPending || exportWebdavMutation.isPending}
        onSubmit={(confirmToken) => {
          if (reauthTarget?.kind === 'download') {
            exportMutation.mutate({
              type: reauthTarget.type,
              confirmToken,
            })
          } else if (reauthTarget?.kind === 'webdav') {
            exportWebdavMutation.mutate({ type: undefined, confirmToken })
          }
        }}
      />
    </SectionCard>
  )
}
