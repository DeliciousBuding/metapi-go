import { useTranslation } from 'react-i18next'

import type { BackupImportReview } from './backup-import-review'

export function BackupImportSourceCounts({
  review,
}: {
  review: BackupImportReview
}) {
  const { t } = useTranslation()
  const { counts, axonhub, magpie } = review
  return counts.length > 0 ? (
    <dl
      className={
        axonhub || magpie
          ? 'divide-y text-sm'
          : 'grid grid-cols-2 gap-2 sm:grid-cols-3'
      }
    >
      {counts.map(([section, count]) => (
        <div
          key={section}
          className={
            axonhub || magpie
              ? 'flex items-center justify-between gap-3 py-2'
              : 'bg-muted/40 min-w-0 rounded-lg px-3 py-3'
          }
        >
          <dt className='text-muted-foreground text-xs'>
            {t(`settings.content.importExport.design.sections.${section}`, {
              defaultValue: section,
            })}
          </dt>
          <dd
            className={
              axonhub || magpie
                ? 'tabular-nums'
                : 'mt-1 text-xl leading-tight font-semibold tabular-nums'
            }
          >
            {count.toLocaleString()}
          </dd>
        </div>
      ))}
    </dl>
  ) : (
    <p className='text-muted-foreground text-sm'>
      {t('settings.content.importExport.design.emptyPlan')}
    </p>
  )
}
