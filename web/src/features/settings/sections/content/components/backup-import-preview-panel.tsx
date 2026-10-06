import { useTranslation } from 'react-i18next'

export type BackupImportTablePlan = Record<
  string,
  { rows: number; toInsert: number; duplicates: number; skippedRows: number }
>

export type OctopusV5Preview = {
  source: string
  originKey: string
  sections: Record<string, number>
  notImported?: Record<string, number>
  adaptations?: string[]
  blocking?: string[]
}

export type BackupImportPreview =
  | { kind: 'tables'; tables: BackupImportTablePlan }
  | { kind: 'octopus'; data: OctopusV5Preview }

type Props = {
  preview: BackupImportPreview | null
  acknowledged: boolean
  onAcknowledge: (checked: boolean) => void
}

// Presentation only: the import workflow owns the reviewed snapshot and mutations.
export function BackupImportPreviewPanel(props: Props) {
  const { t } = useTranslation()
  const importPreview = props.preview
  const planEntries =
    importPreview?.kind === 'tables' ? Object.entries(importPreview.tables) : []
  const hasNotImportedOctopusSections =
    importPreview?.kind === 'octopus' &&
    Object.values(importPreview.data.notImported ?? {}).some(
      (count) => count > 0
    )
  const hasOctopusPolicyAdaptations =
    importPreview?.kind === 'octopus' &&
    (importPreview.data.adaptations?.length ?? 0) > 0
  const hasBlockingSections =
    importPreview?.kind === 'octopus' &&
    (importPreview.data.blocking?.length ?? 0) > 0
  return (
    <>
      {importPreview?.kind === 'octopus' && (
        <div className='mt-4 space-y-3 rounded-lg border p-4'>
          <h3 className='text-sm font-medium'>
            {t('settings.content.importExport.importPreviewTitle')}
          </h3>
          <p className='text-muted-foreground text-xs'>
            {t('settings.content.importExport.octopusPreviewOrigin', {
              source: importPreview.data.source,
              originKey: importPreview.data.originKey,
            })}
          </p>
          <ul className='text-muted-foreground list-inside list-disc space-y-1 text-xs'>
            {Object.entries(importPreview.data.sections).map(
              ([section, count]) => (
                <li key={section}>
                  {t('settings.content.importExport.octopusPreviewSection', {
                    section,
                    count,
                  })}
                </li>
              )
            )}
          </ul>
          {hasNotImportedOctopusSections && (
            <div className='text-destructive space-y-1 text-xs'>
              <p>
                {t('settings.content.importExport.octopusNotImportedTitle')}
              </p>
              <ul className='list-inside list-disc space-y-1'>
                {Object.entries(importPreview.data.notImported ?? {}).map(
                  ([section, count]) => (
                    <li key={section}>
                      {t(
                        'settings.content.importExport.octopusPreviewSection',
                        { section, count }
                      )}
                    </li>
                  )
                )}
              </ul>
            </div>
          )}
          {!hasNotImportedOctopusSections &&
            !hasOctopusPolicyAdaptations &&
            !hasBlockingSections && (
              <p className='text-muted-foreground text-xs'>
                {t('settings.content.importExport.octopusAllSectionsSupported')}
              </p>
            )}
          {hasOctopusPolicyAdaptations ? (
            <div className='space-y-1 text-xs'>
              <p className='font-medium'>
                {t('settings.content.importExport.octopusAdaptationsTitle')}
              </p>
              <ul className='list-inside list-disc space-y-1'>
                {importPreview.data.adaptations?.map((adaptation) => (
                  <li key={adaptation}>
                    {t(
                      `settings.content.importExport.octopusAdaptation.${adaptation}`
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {(importPreview.data.blocking?.length ?? 0) > 0 ? (
            <p className='text-destructive text-xs'>
              {t('settings.content.importExport.octopusBlockingSections', {
                sections: importPreview.data.blocking?.join(', '),
              })}
            </p>
          ) : null}
          {(importPreview.data.blocking?.length ?? 0) === 0 &&
          (Object.values(importPreview.data.notImported ?? {}).some(
            (count) => count > 0
          ) ||
            (importPreview.data.adaptations?.length ?? 0) > 0) ? (
            <label className='border-destructive/40 flex items-start gap-2 rounded-md border p-3 text-sm'>
              <input
                type='checkbox'
                checked={props.acknowledged}
                onChange={(event) => {
                  props.onAcknowledge(event.target.checked)
                }}
              />
              <span>
                {t(
                  'settings.content.importExport.octopusChannelsOnlyAcknowledgement'
                )}
              </span>
            </label>
          ) : null}
        </div>
      )}
      {importPreview?.kind === 'tables' && planEntries.length > 0 && (
        <div className='mt-4 space-y-2 rounded-lg border p-4'>
          <h3 className='text-sm font-medium'>
            {t('settings.content.importExport.importPreviewTitle')}
          </h3>
          <ul className='text-muted-foreground list-inside list-disc space-y-1 text-xs'>
            {planEntries.map(([table, plan]) => (
              <li key={table}>
                {t('settings.content.importExport.importPreviewRow', {
                  table,
                  toInsert: plan.toInsert,
                  duplicates: plan.duplicates,
                  skipped: plan.skippedRows,
                })}
              </li>
            ))}
          </ul>
        </div>
      )}
    </>
  )
}
