import { ArrowRight, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'

import { SettingsSubsection } from '../../../components/settings-subsection'
import { BackupImportPreviewPanel } from './backup-import-preview-panel'
import { getBackupRemovalCounts } from './backup-import-removals'
import { BackupImportSource } from './backup-import-source'
import { useBackupImportMutation } from './use-backup-import-mutation'
import { useBackupImportReview } from './use-backup-import-review'

export function LocalBackupImport() {
  const { t } = useTranslation()
  const review = useBackupImportReview()
  const {
    previewSnapshot,
    importPreview,
    allowChannelsOnlyImport,
    setAllowChannelsOnlyImport,
    confirmImportOpen,
    setConfirmImportOpen,
  } = review
  const importMutation = useBackupImportMutation(
    review.clearImport,
    review.handlePreviewImport
  )
  const hasNotImportedOctopusSections =
    importPreview?.kind === 'octopus' &&
    Object.values(importPreview.data.notImported ?? {}).some(
      (count) => count > 0
    )
  const hasOctopusPolicyAdaptations =
    importPreview?.kind === 'octopus' &&
    (importPreview.data.adaptations?.length ?? 0) > 0
  const canImport =
    previewSnapshot !== null &&
    !review.previewPending &&
    !(importPreview && importPreview.kind !== 'tables'
      ? (importPreview.data.blocking?.length ?? 0) > 0
      : false) &&
    !(
      previewSnapshot?.source === 'magpie-backup' &&
      !review.sourceProps.passphrase.trim()
    ) &&
    (!(hasNotImportedOctopusSections || hasOctopusPolicyAdaptations) ||
      allowChannelsOnlyImport)
  let confirmImportDescriptionKey =
    'settings.content.importExport.importConfirmDescription'
  if (importPreview?.kind === 'octopus') {
    confirmImportDescriptionKey =
      hasNotImportedOctopusSections || hasOctopusPolicyAdaptations
        ? 'settings.content.importExport.octopusPartialImportConfirmDescription'
        : 'settings.content.importExport.octopusImportConfirmDescription'
  } else if (importPreview?.kind === 'axonhub') {
    confirmImportDescriptionKey =
      'settings.content.importExport.axonhubImportConfirmDescription'
  } else if (importPreview?.kind === 'magpie') {
    confirmImportDescriptionKey =
      'settings.content.importExport.magpieImportConfirmDescription'
  }
  const externalPreview =
    importPreview?.kind === 'tables' ? null : importPreview?.data
  const removalImpact = externalPreview?.removalImpact
  const removalCounts = getBackupRemovalCounts(externalPreview)
  const hasExternalRemovals =
    removalCounts.length > 0 ||
    Object.values(externalPreview?.removals ?? {}).some((count) => count > 0)

  return (
    <>
      <SettingsSubsection
        title={t('settings.content.importExport.importGroup')}
        description={t('settings.content.importExport.design.importHint')}
        icon={<Upload className='size-5' />}
      >
        <BackupImportSource
          {...review.sourceProps}
          disabled={importMutation.isPending}
        />
        <div className='flex items-center justify-end gap-3'>
          <Button
            type='button'
            disabled={review.previewDisabled || importMutation.isPending}
            onClick={() => void review.handlePreviewImport()}
          >
            <ArrowRight className='size-4' aria-hidden='true' />
            {review.previewPending
              ? t('settings.common.saving')
              : t('settings.content.importExport.importPreview')}
          </Button>
        </div>
        <BackupImportPreviewPanel
          preview={importPreview}
          acknowledged={allowChannelsOnlyImport}
          onAcknowledge={setAllowChannelsOnlyImport}
        />
        {importPreview ? (
          <div className='flex justify-end'>
            <Button
              type='button'
              disabled={!canImport || importMutation.isPending}
              onClick={() => setConfirmImportOpen(true)}
            >
              <Upload className='size-4' aria-hidden='true' />
              {t('settings.content.importExport.import')}
            </Button>
          </div>
        ) : null}
      </SettingsSubsection>
      <ConfirmDialog
        key={
          importPreview?.kind === 'octopus' &&
          Object.keys(importPreview.data.notImported ?? {}).length > 0
            ? 'blocked'
            : 'ready'
        }
        open={confirmImportOpen}
        title={t('settings.content.importExport.importConfirmTitle')}
        description={
          t(confirmImportDescriptionKey) +
          (hasExternalRemovals
            ? ` ${t('settings.content.importExport.octopusReplacementDescription')}`
            : '') +
          (removalImpact?.requiresCascade
            ? ` ${t('settings.content.importExport.removalImpactCascade')}`
            : '') +
          (removalCounts.length > 0
            ? ` ${removalCounts
                .map(([section, count]) =>
                  t('settings.content.importExport.octopusPreviewSection', {
                    section: t(
                      `settings.content.importExport.design.sections.${section}`,
                      { defaultValue: section }
                    ),
                    count,
                  })
                )
                .join(' · ')}`
            : '')
        }
        confirmLabel={t('settings.content.importExport.import')}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmImportOpen(false)
          if (!previewSnapshot || !canImport || importMutation.isPending) return
          importMutation.mutate({
            raw: previewSnapshot.raw,
            originKey: previewSnapshot.originKey,
            source: previewSnapshot.source,
            channelsOnly: allowChannelsOnlyImport,
            replaceOrigin: hasExternalRemovals,
            replacementRevision: removalImpact?.revision,
            magpiePassphrase:
              previewSnapshot.source === 'magpie-backup'
                ? review.sourceProps.passphrase
                : undefined,
          })
        }}
        onCancel={() => setConfirmImportOpen(false)}
      />
    </>
  )
}
