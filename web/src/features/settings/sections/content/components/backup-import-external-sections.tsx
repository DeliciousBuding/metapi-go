import { ChevronDown, CircleCheck, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import { BackupImportImpact } from './backup-import-impact'
import type { BackupImportPreview } from './backup-import-preview-panel'
import type { BackupImportReview } from './backup-import-review'
import { BackupImportSkippedRecords } from './backup-import-skipped-records'
import { BackupImportSourceCounts } from './backup-import-source-counts'

export function BackupImportExternalSections({
  preview,
  review,
  acknowledged,
  onAcknowledge,
}: {
  preview: BackupImportPreview
  review: BackupImportReview
  acknowledged: boolean
  onAcknowledge: (checked: boolean) => void
}) {
  const { t } = useTranslation()
  const {
    external,
    axonhub,
    magpie,
    counts,
    routable,
    blocked,
    hasExternalReview,
    needsAcknowledgement,
  } = review
  if (!external) return null
  const sourceCounts = <BackupImportSourceCounts review={review} />
  return (
    <>
      <p className='text-muted-foreground text-xs leading-relaxed break-words'>
        {t('settings.content.importExport.octopusPreviewOrigin', {
          source: external.source,
          originKey: external.originKey,
        })}
      </p>
      {routable.length > 0 ? (
        <div className='space-y-2'>
          <p className='text-sm font-medium'>
            {t('settings.content.importExport.axonhubRoutableTitle')}
          </p>
          <dl className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
            {routable.map(([section, count]) => (
              <div
                key={section}
                className='bg-primary/5 min-w-0 rounded-lg px-3 py-3'
              >
                <dt className='text-muted-foreground text-xs'>
                  {t(
                    `settings.content.importExport.design.routable.${section}`,
                    { defaultValue: section }
                  )}
                </dt>
                <dd className='mt-1 text-xl leading-tight font-semibold tabular-nums'>
                  {count.toLocaleString()}
                </dd>
              </div>
            ))}
          </dl>
        </div>
      ) : null}
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
                  t(`settings.content.importExport.design.blockers.${key}`, {
                    defaultValue: key,
                  })
                )
                .join(' · '),
            })}
          </p>
        </div>
      ) : null}
      {!axonhub && sourceCounts}
      <BackupImportImpact preview={preview} review={review} />
      <BackupImportSkippedRecords review={review} />
      {(axonhub || magpie) && counts.length > 0 ? (
        <Collapsible className='rounded-lg border'>
          <CollapsibleTrigger
            render={<Button variant='ghost' />}
            className='group w-full justify-between px-3'
          >
            {t('settings.content.importExport.sourceRecordCounts')}
            <ChevronDown
              className='size-4 transition-transform group-data-panel-open:rotate-180'
              aria-hidden='true'
            />
          </CollapsibleTrigger>
          <CollapsibleContent className='px-3 pb-1'>
            {sourceCounts}
          </CollapsibleContent>
        </Collapsible>
      ) : null}
      {(axonhub || magpie) && counts.length === 0 && sourceCounts}
      {!blocked && !hasExternalReview ? (
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
            checked={acknowledged}
            onCheckedChange={onAcknowledge}
          />
          <span>
            {t(
              'settings.content.importExport.octopusChannelsOnlyAcknowledgement'
            )}
          </span>
        </label>
      ) : null}
    </>
  )
}
