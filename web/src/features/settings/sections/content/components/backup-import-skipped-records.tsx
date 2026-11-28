import { useTranslation } from 'react-i18next'

import {
  formatAxonHubResidual,
  formatMagpieResidual,
} from './backup-import-residuals'
import type { BackupImportReview } from './backup-import-review'

export function BackupImportSkippedRecords({
  review,
}: {
  review: BackupImportReview
}) {
  const { t } = useTranslation()
  const {
    magpie,
    skipped,
    skippedGroups,
    residuals,
    unsupportedProtocols,
    otherResiduals,
  } = review
  return (
    <>
      {skipped.length > 0 ? (
        <div className='bg-warning/10 text-warning-soft-fg space-y-2 rounded-lg p-3 text-sm'>
          <p className='font-medium'>
            {t(
              magpie
                ? 'settings.content.importExport.magpieSkippedProvidersTitle'
                : 'settings.content.importExport.axonhubSkippedTitle'
            )}
          </p>
          <ul className='space-y-1.5'>
            {skipped.map((item) => (
              <li key={`${item.type}-${item.sourceId}`}>
                <span className='font-medium'>{item.type}</span>
                {' · '}
                <span className='opacity-80'>
                  {item.reasons
                    .map((reason) =>
                      t(
                        `settings.content.importExport.design.${item.reasonPrefix}.${reason}`,
                        { defaultValue: reason }
                      )
                    )
                    .join(' · ')}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {skippedGroups.length > 0 ? (
        <div className='bg-warning/10 text-warning-soft-fg space-y-2 rounded-lg p-3 text-sm'>
          <p className='font-medium'>
            {t('settings.content.importExport.magpieSkippedGroupsTitle')}
          </p>
          <ul className='space-y-1.5'>
            {skippedGroups.map((group) => (
              <li key={group.sourceId}>
                <span className='font-medium'>{group.sourceId}</span>
                {' · '}
                <span className='opacity-80'>
                  {group.reasons
                    .map((reason) =>
                      t(
                        `settings.content.importExport.design.magpieReasons.${reason}`,
                        { defaultValue: reason }
                      )
                    )
                    .join(' · ')}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {residuals.length > 0 ? (
        <div className='bg-muted/50 space-y-2 rounded-lg p-3 text-sm leading-relaxed'>
          <p className='font-medium'>
            {t(
              magpie
                ? 'settings.content.importExport.magpieResidualsTitle'
                : 'settings.content.importExport.axonhubResidualsTitle'
            )}
          </p>
          <ul className='text-muted-foreground space-y-1'>
            {unsupportedProtocols.length > 0 && (
              <li className='break-words'>
                {t(
                  'settings.content.importExport.axonhubUnsupportedProtocols',
                  {
                    formats: unsupportedProtocols.join(', '),
                  }
                )}
              </li>
            )}
            {otherResiduals.map((residual) => (
              <li key={residual} className='break-words'>
                {magpie
                  ? formatMagpieResidual(residual, t)
                  : formatAxonHubResidual(residual, t)}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </>
  )
}
