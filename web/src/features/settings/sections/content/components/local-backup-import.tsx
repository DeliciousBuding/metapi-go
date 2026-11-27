import { useMutation, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { ArrowRight, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { useReauthReplay } from '@/components/common/reauth-provider'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import {
  detectExternalBackupSourceText,
  stripBackupBOM,
  type ExternalBackupSource,
} from '@/lib/helpers/external-backup-source'
import { toast } from '@/lib/toast'

import { SettingsSubsection } from '../../../components/settings-subsection'
import {
  BackupImportPreviewPanel,
  type BackupImportPreview,
  type BackupImportTablePlan,
  type AxonHubV14Preview,
  type MagpieBackupPreview,
  type OctopusV5Preview,
} from './backup-import-preview-panel'
import { getBackupRemovalCounts } from './backup-import-removals'
import { BackupImportSource } from './backup-import-source'

const BACKUP_IMPORT_MAX_BYTES = 20 * 1024 * 1024
const MAGPIE_ORIGIN_KEY = 'magpie-backup'

type ImportPreviewSnapshot = {
  raw: string
  originKey?: string
  source?: ExternalBackupSource
  plan: BackupImportPreview
}

function isExternalPreview(
  plan: BackupImportTablePlan | OctopusV5Preview | AxonHubV14Preview
): plan is OctopusV5Preview | AxonHubV14Preview {
  const candidate = plan as OctopusV5Preview
  const impact = candidate.removalImpact
  if (
    impact &&
    (impact.kind !== 'source' ||
      impact.id !== 0 ||
      typeof impact.revision !== 'string' ||
      !impact.revision.trim() ||
      typeof impact.counts !== 'object' ||
      impact.counts === null ||
      Array.isArray(impact.counts))
  ) {
    return false
  }
  return (
    typeof candidate.source === 'string' &&
    typeof candidate.originKey === 'string' &&
    typeof candidate.sections === 'object' &&
    candidate.sections !== null &&
    !Array.isArray(candidate.sections) &&
    (candidate.blocking === undefined || Array.isArray(candidate.blocking))
  )
}

export function LocalBackupImport() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const handleReauth = useReauthReplay()
  const [importText, setImportText] = useState('')
  const [importFileName, setImportFileName] = useState('')
  const [externalOriginKey, setExternalOriginKey] = useState('')
  const [magpiePassphrase, setMagpiePassphrase] = useState('')
  const [allowChannelsOnlyImport, setAllowChannelsOnlyImport] = useState(false)
  const [previewSnapshot, setPreviewSnapshot] =
    useState<ImportPreviewSnapshot | null>(null)
  const importPreview = previewSnapshot?.plan ?? null
  const importRevision = useRef(0)
  const [confirmImportOpen, setConfirmImportOpen] = useState(false)

  const previewMutation = useMutation({
    mutationFn: async (input: {
      raw: string
      originKey?: string
      magpiePassphrase?: string
    }) => {
      const { raw, originKey, magpiePassphrase } = input
      const data = JSON.parse(stripBackupBOM(raw)) as unknown
      const external = detectExternalBackupSourceText(raw)
      const result = (await api.previewBackupImport(data, {
        externalOriginKey: external ? originKey : undefined,
        magpiePassphrase,
      })) as {
        success?: boolean
        plan?:
          | BackupImportTablePlan
          | OctopusV5Preview
          | AxonHubV14Preview
          | MagpieBackupPreview
      }
      if (external) {
        const plan = result.plan
        if (!plan || !isExternalPreview(plan)) {
          throw new Error('Invalid external import preview')
        }
        if (plan.originKey !== originKey) {
          throw new Error('Preview origin key does not match the request')
        }
        if (external === 'axonhub-v1.4') {
          const axonhub = plan as AxonHubV14Preview
          if (
            typeof axonhub.routable !== 'object' ||
            axonhub.routable === null ||
            Array.isArray(axonhub.routable) ||
            (axonhub.residuals !== undefined &&
              !Array.isArray(axonhub.residuals)) ||
            (axonhub.skippedChannels !== undefined &&
              !Array.isArray(axonhub.skippedChannels))
          ) {
            throw new Error('Invalid AxonHub v1.4 import preview')
          }
          return {
            kind: 'axonhub' as const,
            data: axonhub,
          }
        }
        if (external === 'magpie-backup') {
          const magpie = plan as MagpieBackupPreview
          if (
            typeof magpie.routable !== 'object' ||
            magpie.routable === null ||
            Array.isArray(magpie.routable) ||
            (magpie.residuals !== undefined &&
              !Array.isArray(magpie.residuals)) ||
            (magpie.skippedProviders !== undefined &&
              !Array.isArray(magpie.skippedProviders)) ||
            (magpie.skippedGroups !== undefined &&
              !Array.isArray(magpie.skippedGroups))
          ) {
            throw new Error('Invalid Magpie backup import preview')
          }
          return {
            kind: 'magpie' as const,
            data: magpie,
          }
        }
        const octopus = plan as OctopusV5Preview
        if (
          octopus.adaptations !== undefined &&
          !Array.isArray(octopus.adaptations)
        ) {
          throw new Error('Invalid Octopus v5 import preview')
        }
        return {
          kind: 'octopus' as const,
          data: octopus,
        }
      }
      if (
        !result.plan ||
        typeof result.plan !== 'object' ||
        Array.isArray(result.plan)
      ) {
        throw new Error('Invalid backup import preview')
      }
      return {
        kind: 'tables' as const,
        tables: result.plan as BackupImportTablePlan,
      }
    },
  })

  const importMutation = useMutation({
    retry: false,
    mutationFn: async (input: {
      raw: string
      originKey?: string
      source?: ExternalBackupSource
      channelsOnly?: boolean
      replaceOrigin?: boolean
      replacementRevision?: string
      magpiePassphrase?: string
      confirmToken?: string
    }) => {
      const { raw, originKey, source, channelsOnly, replaceOrigin } = input
      const data = JSON.parse(stripBackupBOM(raw)) as unknown
      const octopus = source === 'octopus-v5'
      const originKeyArg = source ? originKey : undefined
      const octopusMode =
        octopus && channelsOnly ? ('channels-only' as const) : undefined
      const replaceOctopus = octopus && replaceOrigin
      const replaceAxonHub = source === 'axonhub-v1.4' && replaceOrigin
      const replaceMagpie = source === 'magpie-backup' && replaceOrigin
      return input.confirmToken
        ? api.importBackup(data, {
            externalOriginKey: originKeyArg,
            octopusImportMode: octopusMode,
            replaceOctopusOrigin: replaceOctopus,
            replaceAxonHubOrigin: replaceAxonHub,
            replaceMagpieOrigin: replaceMagpie,
            replacementRevision: input.replacementRevision,
            magpiePassphrase: input.magpiePassphrase,
            confirmToken: input.confirmToken,
          })
        : api.importBackup(data, {
            externalOriginKey: originKeyArg,
            octopusImportMode: octopusMode,
            replaceOctopusOrigin: replaceOctopus,
            replaceAxonHubOrigin: replaceAxonHub,
            replaceMagpieOrigin: replaceMagpie,
            replacementRevision: input.replacementRevision,
            magpiePassphrase: input.magpiePassphrase,
          })
    },
    onSuccess: () => {
      toast.success(t('settings.content.importExport.toast.imported'))
      setImportText('')
      setImportFileName('')
      setExternalOriginKey('')
      setMagpiePassphrase('')
      resetImportReview()
      // A backup restores rows across the schema, including settings and routes.
      void queryClient.invalidateQueries()
    },
    onError: (error: unknown, input) => {
      if (isAxiosError(error) && error.response?.status === 409) {
        toast.error(t('settings.content.importExport.toast.previewChanged'))
        void handlePreviewImport()
        return
      }
      if (
        handleReauth(
          error,
          t('reauth.operations.backupImport'),
          (confirmToken) =>
            importMutation.mutateAsync({ ...input, confirmToken })
        )
      ) {
        return
      }
      toast.error(t('settings.content.importExport.toast.importFailed'))
    },
  })

  function resetImportReview() {
    importRevision.current++
    setPreviewSnapshot(null)
    setConfirmImportOpen(false)
    setAllowChannelsOnlyImport(false)
  }

  async function handleImportFile(file: File) {
    resetImportReview()
    const revision = importRevision.current
    setImportText('')
    setImportFileName('')
    setMagpiePassphrase('')
    if (file.size > BACKUP_IMPORT_MAX_BYTES) {
      toast.error(t('settings.content.importExport.importFileTooLarge'))
      return
    }
    try {
      const raw = stripBackupBOM(await file.text())
      if (revision !== importRevision.current) return
      setImportText(raw)
      setImportFileName(file.name)
      if (
        detectExternalBackupSourceText(raw) === 'magpie-backup' &&
        !externalOriginKey.trim()
      ) {
        setExternalOriginKey(MAGPIE_ORIGIN_KEY)
      }
    } catch {
      if (revision !== importRevision.current) return
      toast.error(t('settings.content.importExport.toast.importFailed'))
    }
  }

  async function handlePreviewImport() {
    resetImportReview()
    const raw = importText
    const originKey = externalOriginKey.trim()
    const source = detectExternalBackupSourceText(raw)
    const revision = importRevision.current
    try {
      if (source && !originKey) {
        toast.error(t('settings.content.importExport.originKeyRequired'))
        return
      }
      if (source === 'magpie-backup' && !magpiePassphrase.trim()) {
        toast.error(t('settings.content.importExport.magpiePassphraseRequired'))
        return
      }
      const plan = await previewMutation.mutateAsync({
        raw,
        originKey,
        magpiePassphrase:
          source === 'magpie-backup' ? magpiePassphrase : undefined,
      })
      if (revision !== importRevision.current) return
      setPreviewSnapshot({
        raw,
        originKey: plan.kind === 'tables' ? undefined : originKey,
        source: plan.kind === 'tables' ? undefined : (source ?? undefined),
        plan,
      })
    } catch {
      if (revision !== importRevision.current) return
      toast.error(t('settings.content.importExport.toast.importFailed'))
    }
  }

  const externalImportSource = detectExternalBackupSourceText(importText)
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
    !previewMutation.isPending &&
    !(importPreview && importPreview.kind !== 'tables'
      ? (importPreview.data.blocking?.length ?? 0) > 0
      : false) &&
    !(
      previewSnapshot?.source === 'magpie-backup' && !magpiePassphrase.trim()
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
          raw={importText}
          fileName={importFileName}
          originKey={externalOriginKey}
          external={externalImportSource !== null}
          magpie={externalImportSource === 'magpie-backup'}
          passphrase={magpiePassphrase}
          disabled={importMutation.isPending}
          onFile={(file) => void handleImportFile(file)}
          onText={(raw) => {
            resetImportReview()
            setImportText(raw)
            setImportFileName('')
            setMagpiePassphrase('')
            if (
              detectExternalBackupSourceText(raw) === 'magpie-backup' &&
              !externalOriginKey.trim()
            ) {
              setExternalOriginKey(MAGPIE_ORIGIN_KEY)
            }
          }}
          onPassphrase={(passphrase) => {
            resetImportReview()
            setMagpiePassphrase(passphrase)
          }}
          onOrigin={(origin) => {
            resetImportReview()
            setExternalOriginKey(origin)
          }}
        />
        <div className='flex items-center justify-end gap-3'>
          <Button
            type='button'
            disabled={
              previewMutation.isPending ||
              importMutation.isPending ||
              !importText.trim() ||
              (externalImportSource !== null && !externalOriginKey.trim()) ||
              (externalImportSource === 'magpie-backup' &&
                !magpiePassphrase.trim())
            }
            onClick={() => void handlePreviewImport()}
          >
            <ArrowRight className='size-4' aria-hidden='true' />
            {previewMutation.isPending
              ? t('settings.common.saving')
              : t('settings.content.importExport.importPreview')}
          </Button>
        </div>
        <BackupImportPreviewPanel
          preview={importPreview}
          acknowledged={allowChannelsOnlyImport}
          onAcknowledge={(checked) => {
            setAllowChannelsOnlyImport(checked)
          }}
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
                ? magpiePassphrase
                : undefined,
          })
        }}
        onCancel={() => setConfirmImportOpen(false)}
      />
    </>
  )
}
