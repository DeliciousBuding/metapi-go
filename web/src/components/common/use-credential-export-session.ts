import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'
import type {
  CredentialExportProfileId as ExportProfileId,
  CredentialExportResponse as ExportResponse,
} from '@/lib/api/credential-export'
import { copyText } from '@/lib/clipboard'
import { isReauthRequired } from '@/lib/http-client'
import { toast } from '@/lib/toast'

import type { ClientImport } from './credential-export-profile'
import type { CredentialVerifyProtocol } from './credential-verify-section'

export function useCredentialExportSession(keyId: number) {
  const { t } = useTranslation()
  const [client, setClient] = useState<ExportProfileId>('codex')
  const [model, setModel] = useState('')
  const [copiedKey, setCopiedKey] = useState(false)
  const [unlockDraft, setUnlockDraft] = useState('')
  // Re-confirmation and plaintext exports must never enter Query/Mutation caches.
  const tokenRef = useRef('')
  const exportRequest = useRef<AbortController | null>(null)
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [data, setData] = useState<ExportResponse | null>(null)
  const [exportPhase, setExportPhase] = useState<
    'locked' | 'loading' | 'ready' | 'error' | 'rejected'
  >('locked')
  // #1034: the key renders masked until explicitly revealed.
  const [showKey, setShowKey] = useState(false)
  // #1034: deep links hand the plain key to an external app — they wait for
  // an explicit confirmation step before navigating.
  const [pendingDeepLink, setPendingDeepLink] = useState<ClientImport | null>(
    null
  )

  function abortExport() {
    const controller = exportRequest.current
    exportRequest.current = null
    controller?.abort()
  }

  function dispose() {
    abortExport()
    tokenRef.current = ''
    if (copyTimer.current !== null) clearTimeout(copyTimer.current)
  }

  useEffect(() => dispose, [])

  function close() {
    dispose()
    setUnlockDraft('')
    setData(null)
    setShowKey(false)
    setPendingDeepLink(null)
    setModel('')
    setExportPhase('locked')
    setCopiedKey(false)
  }

  async function loadExport(token: string) {
    if (exportRequest.current) return
    const controller = new AbortController()
    exportRequest.current = controller
    tokenRef.current = token
    setExportPhase('loading')
    setPendingDeepLink(null)
    try {
      const response = await api.getDownstreamKeyExport(
        keyId,
        'all',
        token,
        model || undefined,
        controller.signal
      )
      if (exportRequest.current !== controller) return
      if (response.keyId !== keyId || (model && response.model !== model))
        throw new Error('Invalid export target')
      setData(response)
      if (!response.profiles.some((profile) => profile.id === client))
        setClient(response.profiles[0].id)
      setUnlockDraft('')
      setExportPhase('ready')
    } catch (error) {
      if (exportRequest.current !== controller) return
      if (isReauthRequired(error)) {
        tokenRef.current = ''
        setUnlockDraft('')
        setData(null)
        setShowKey(false)
        setExportPhase('rejected')
      } else {
        setExportPhase('error')
      }
    } finally {
      if (exportRequest.current === controller) exportRequest.current = null
    }
  }

  function resetContext() {
    abortExport()
    setExportPhase('ready')
    setPendingDeepLink(null)
    setShowKey(false)
  }

  function changeModel(value: string) {
    if (value === model) return
    resetContext()
    setModel(value)
  }

  function changeClient(value: ExportProfileId) {
    if (value === client) return
    resetContext()
    setClient(value)
  }

  function changeProtocol(value: CredentialVerifyProtocol) {
    if (value === 'responses') changeClient('codex')
    else if (value === 'messages') changeClient('claude-code')
    else changeClient('openai')
  }

  function handleUnlock() {
    const token = unlockDraft.trim()
    if (!token) return
    void loadExport(token)
  }

  async function handleCopyKey(apiKey: string) {
    if (!(await copy(apiKey, 'connect.toast.keyCopied'))) return
    setCopiedKey(true)
    if (copyTimer.current !== null) clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopiedKey(false), 1500)
  }

  async function copy(text: string, successKey: string) {
    const copied = await copyText(text)
    if (copied) {
      toast.success(t(successKey))
    } else {
      // Clipboard may be unavailable (non-secure context / permissions).
      toast.error(t('connect.copyFailed'))
    }
    return copied
  }

  return {
    client,
    model,
    data,
    phase: exportPhase,
    unlockDraft,
    setUnlockDraft,
    unlock: handleUnlock,
    close,
    refresh: () => void loadExport(tokenRef.current),
    changeModel,
    changeClient,
    changeProtocol,
    showKey,
    toggleKey: () => setShowKey((value) => !value),
    copiedKey,
    copyKey: handleCopyKey,
    copyEndpoint: (endpoint: string) =>
      void copy(endpoint, 'connect.toast.endpointCopied'),
    copyConfig: (text: string) => void copy(text, 'connect.toast.jsonCopied'),
    pendingDeepLink,
    setPendingDeepLink,
  }
}
