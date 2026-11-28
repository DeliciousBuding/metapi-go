import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import type { BackupWebdavResponse } from '@/lib/api'

export function WebdavSyncActions({
  exportPending,
  onExport,
  importPending,
  onImport,
  state,
}: {
  exportPending: boolean
  onExport: () => void
  importPending: boolean
  onImport: () => void
  state?: BackupWebdavResponse['state']
}) {
  const { t } = useTranslation()
  return (
    <>
      <div className='flex flex-wrap gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={exportPending}
          onClick={onExport}
        >
          {t('settings.content.importExport.exportToWebdav')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={importPending}
          onClick={onImport}
        >
          {t('settings.content.importExport.importFromWebdav')}
        </Button>
      </div>
      {state?.lastSyncAt ? (
        <p className='text-muted-foreground text-xs'>
          {t('settings.content.importExport.lastSync', {
            at: state.lastSyncAt,
          })}
        </p>
      ) : null}
      {state?.lastError ? (
        <p className='text-destructive text-xs'>
          {t('settings.content.importExport.lastError', {
            error: state.lastError,
          })}
        </p>
      ) : null}
    </>
  )
}
