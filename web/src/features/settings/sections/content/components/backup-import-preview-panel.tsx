import { CircleCheck, TriangleAlert, Layers3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'

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

export function BackupImportPreviewPanel(props: Props) {
  const { t } = useTranslation()
  const preview = props.preview
  if (!preview) return null
  const external = preview.kind === 'octopus' ? preview.data : null
  const blocked = (external?.blocking?.length ?? 0) > 0
  const omitted = Object.entries(external?.notImported ?? {}).filter(
    ([, count]) => count > 0
  )
  const adapted = (external?.adaptations?.length ?? 0) > 0
  const needsAcknowledgement = omitted.length > 0 || adapted
  let status = 'ready'
  let variant: 'success' | 'warning' | 'destructive' = 'success'
  if (blocked) {
    status = 'blocked'
    variant = 'destructive'
  } else if (needsAcknowledgement) {
    status = 'review'
    variant = 'warning'
  }
  const counts = external
    ? Object.entries(external.sections).filter(([, count]) => count > 0)
    : []
  return (
    <section
      className='overflow-hidden rounded-xl border'
      aria-label={t('settings.content.importExport.importPreviewTitle')}
    >
      <div className='bg-muted/40 flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3'>
        <h3 className='flex items-center gap-2 text-sm font-semibold'>
          <Layers3 className='text-primary size-4' aria-hidden='true' />
          {t('settings.content.importExport.importPreviewTitle')}
        </h3>
        <Badge variant={variant}>
          {status === 'ready' ? (
            <CircleCheck aria-hidden='true' />
          ) : (
            <TriangleAlert aria-hidden='true' />
          )}
          {t(`settings.content.importExport.design.${status}`)}
        </Badge>
      </div>
      <div className='space-y-4 p-4'>
        {external ? (
          <>
            <p className='text-muted-foreground text-xs leading-relaxed break-words'>
              {t('settings.content.importExport.octopusPreviewOrigin', {
                source: external.source,
                originKey: external.originKey,
              })}
            </p>
            {blocked ? (
              <div
                role='alert'
                className='bg-destructive/10 text-destructive-soft-fg flex items-start gap-2.5 rounded-lg p-3 text-sm leading-relaxed'
              >
                <TriangleAlert
                  className='mt-0.5 size-4 shrink-0'
                  aria-hidden='true'
                />
                <p>
                  {t('settings.content.importExport.octopusBlockingSections', {
                    sections: external.blocking
                      ?.map((key) =>
                        t(
                          `settings.content.importExport.design.blockers.${key}`,
                          { defaultValue: key }
                        )
                      )
                      .join(' · '),
                  })}
                </p>
              </div>
            ) : null}
            {counts.length > 0 ? (
              <dl className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
                {counts.map(([section, count]) => (
                  <div
                    key={section}
                    className='bg-muted/40 min-w-0 rounded-lg px-3 py-3'
                  >
                    <dt className='text-muted-foreground text-xs'>
                      {t(
                        `settings.content.importExport.design.sections.${section}`,
                        { defaultValue: section }
                      )}
                    </dt>
                    <dd className='mt-1 text-xl leading-tight font-semibold tabular-nums'>
                      {count.toLocaleString()}
                    </dd>
                  </div>
                ))}
              </dl>
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('settings.content.importExport.design.emptyPlan')}
              </p>
            )}
            {omitted.length > 0 ? (
              <div className='bg-warning/10 text-warning-soft-fg space-y-2 rounded-lg p-3 text-sm'>
                <p className='font-medium'>
                  {t('settings.content.importExport.octopusNotImportedTitle')}
                </p>
                <div className='flex flex-wrap gap-2'>
                  {omitted.map(([section, count]) => (
                    <Badge key={section} variant='outline'>
                      {t(
                        `settings.content.importExport.design.sections.${section}`,
                        { defaultValue: section }
                      )}{' '}
                      · {count}
                    </Badge>
                  ))}
                </div>
              </div>
            ) : null}
            {adapted ? (
              <div className='bg-warning/10 text-warning-soft-fg space-y-2 rounded-lg p-3 text-sm leading-relaxed'>
                <p className='font-medium'>
                  {t('settings.content.importExport.octopusAdaptationsTitle')}
                </p>
                {external.adaptations?.map((key) => (
                  <p key={key}>
                    {t(
                      `settings.content.importExport.octopusAdaptation.${key}`
                    )}
                  </p>
                ))}
              </div>
            ) : null}
            {!blocked && !needsAcknowledgement ? (
              <p className='text-muted-foreground flex items-center gap-2 text-sm'>
                <CircleCheck
                  className='text-success-soft-fg size-4 shrink-0'
                  aria-hidden='true'
                />
                {t('settings.content.importExport.octopusAllSectionsSupported')}
              </p>
            ) : null}
            {!blocked && needsAcknowledgement ? (
              <label className='flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm leading-relaxed'>
                <Checkbox
                  className='mt-1'
                  checked={props.acknowledged}
                  onCheckedChange={props.onAcknowledge}
                />
                <span>
                  {t(
                    'settings.content.importExport.octopusChannelsOnlyAcknowledgement'
                  )}
                </span>
              </label>
            ) : null}
          </>
        ) : (
          <ul className='divide-y text-sm'>
            {preview.kind === 'tables' &&
              Object.entries(preview.tables).map(([table, plan]) => (
                <li key={table} className='py-2 first:pt-0 last:pb-0'>
                  {t('settings.content.importExport.importPreviewRow', {
                    table,
                    toInsert: plan.toInsert,
                    duplicates: plan.duplicates,
                    skipped: plan.skippedRows,
                  })}
                </li>
              ))}
          </ul>
        )}
      </div>
    </section>
  )
}
