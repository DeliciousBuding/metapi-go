import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import type { BackupImportPreview } from './backup-import-preview-panel'
import type { BackupImportReview } from './backup-import-review'

export function BackupImportImpact({
  preview,
  review,
}: {
  preview: BackupImportPreview
  review: BackupImportReview
}) {
  const { t } = useTranslation()
  const { removals, removalImpact, omitted, adapted } = review
  return (
    <>
      {removals.length > 0 ? (
        <div className='bg-warning/10 text-warning-soft-fg space-y-2 rounded-lg p-3 text-sm'>
          <p className='font-medium'>
            {removalImpact
              ? t('settings.content.importExport.removalImpactTitle')
              : t('settings.content.importExport.octopusReplacementTitle')}
          </p>
          <p className='leading-relaxed'>
            {t('settings.content.importExport.octopusReplacementDescription')}
          </p>
          {removalImpact?.requiresCascade ? (
            <p className='leading-relaxed'>
              {t('settings.content.importExport.removalImpactCascade')}
            </p>
          ) : null}
          <div className='flex flex-wrap gap-2'>
            {removals.map(([section, count]) => (
              <Badge key={section} variant='outline'>
                {t(`settings.content.importExport.design.sections.${section}`, {
                  defaultValue: section,
                })}{' '}
                · {count}
              </Badge>
            ))}
          </div>
        </div>
      ) : null}
      {omitted.length > 0 ? (
        <div className='bg-warning/10 text-warning-soft-fg space-y-2 rounded-lg p-3 text-sm'>
          <p className='font-medium'>
            {t('settings.content.importExport.octopusNotImportedTitle')}
          </p>
          <div className='flex flex-wrap gap-2'>
            {omitted.map(([section, count]) => (
              <Badge key={section} variant='outline'>
                {t(`settings.content.importExport.design.sections.${section}`, {
                  defaultValue: section,
                })}{' '}
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
          {(preview.kind === 'octopus'
            ? (preview.data.adaptations ?? [])
            : []
          ).map((key) => (
            <p key={key}>
              {t(`settings.content.importExport.octopusAdaptation.${key}`)}
            </p>
          ))}
        </div>
      ) : null}
    </>
  )
}
