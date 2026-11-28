import { useMutation } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  detectExternalBackupSourceText,
  stripBackupBOM,
  type ExternalBackupSource,
} from '@/lib/helpers/external-backup-source'
import { toast } from '@/lib/toast'

import type { BackupImportPreview } from './backup-import-preview-panel'
import { fetchBackupImportPreview } from './parse-backup-import-preview'

const BACKUP_IMPORT_MAX_BYTES = 20 * 1024 * 1024
const MAGPIE_ORIGIN_KEY = 'magpie-backup'

type ImportPreviewSnapshot = {
  raw: string
  originKey?: string
  source?: ExternalBackupSource
  plan: BackupImportPreview
}

export function useBackupImportReview() {
  const { t } = useTranslation()
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
  const previewMutation = useMutation({ mutationFn: fetchBackupImportPreview })
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
  function clearImport() {
    setImportText('')
    setImportFileName('')
    setExternalOriginKey('')
    setMagpiePassphrase('')
    resetImportReview()
  }
  function onText(raw: string) {
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
  }
  function onPassphrase(passphrase: string) {
    resetImportReview()
    setMagpiePassphrase(passphrase)
  }
  function onOrigin(origin: string) {
    resetImportReview()
    setExternalOriginKey(origin)
  }
  return {
    sourceProps: {
      raw: importText,
      fileName: importFileName,
      originKey: externalOriginKey,
      external: externalImportSource !== null,
      magpie: externalImportSource === 'magpie-backup',
      passphrase: magpiePassphrase,
      onFile: handleImportFile,
      onText,
      onOrigin,
      onPassphrase,
    },
    previewDisabled:
      previewMutation.isPending ||
      !importText.trim() ||
      (externalImportSource !== null && !externalOriginKey.trim()) ||
      (externalImportSource === 'magpie-backup' && !magpiePassphrase.trim()),
    previewPending: previewMutation.isPending,
    previewSnapshot,
    importPreview,
    allowChannelsOnlyImport,
    setAllowChannelsOnlyImport,
    confirmImportOpen,
    setConfirmImportOpen,
    clearImport,
    handlePreviewImport,
  }
}
