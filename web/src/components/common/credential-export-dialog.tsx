// Unlocks a downstream key for verification, client imports and config export.
// Exports live for this dialog session; reopening requires confirmation again.

import {
  Check,
  Copy,
  ExternalLink,
  Eye,
  EyeOff,
  Link2,
  Lock,
  Sparkles,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { api } from '@/lib/api'
import type {
  CredentialExportProfile as ExportProfile,
  CredentialExportProfileId as ExportProfileId,
  CredentialExportResponse as ExportResponse,
} from '@/lib/api/credential-export'
import { copyText } from '@/lib/clipboard'
import { isReauthRequired } from '@/lib/http-client'
import { toast } from '@/lib/toast'

import {
  CredentialVerifySection,
  type CredentialVerifyProtocol,
} from './credential-verify-section'

const PROFILE_PROTOCOL: Record<ExportProfileId, CredentialVerifyProtocol> = {
  openai: 'chat',
  cherry: 'chat',
  generic: 'chat',
  'claude-code': 'messages',
  codex: 'responses',
  openwebui: 'chat',
}
const CC_SWITCH_APP: Partial<Record<ExportProfileId, string>> = {
  codex: 'codex',
  'claude-code': 'claude',
}

/** Clipboard write with a unified success toast. */
async function copyTextWithToast(
  text: string,
  successKey: string,
  t: (key: string) => string
) {
  const copied = await copyText(text)
  if (copied) {
    toast.success(t(successKey))
  } else {
    // Clipboard may be unavailable (non-secure context / permissions).
    toast.error(t('connect.copyFailed'))
  }
  return copied
}

function buildCherryStudioLink(
  keyId: number,
  keyName: string,
  apiBase: string,
  apiKey: string
): string {
  const payload = JSON.stringify({
    id: String(keyId),
    name: keyName,
    baseUrl: apiBase,
    apiKey,
  })
  const data = encodeURIComponent(btoa(unescape(encodeURIComponent(payload))))
  return `cherrystudio://providers/api-keys?v=1&data=${data}`
}

function buildCcSwitchLink(
  app: string,
  apiBase: string,
  apiKey: string
): string {
  const params = new URLSearchParams({
    resource: 'provider',
    app,
    endpoint: apiBase,
    apiKey,
  })
  return `ccswitch://v1/import?${params.toString()}`
}

/** Render profile content as clipboard text. */
function stringifyProfileContent(profile: ExportProfile): string {
  if (typeof profile.content === 'string') {
    return profile.content
  }
  return JSON.stringify(profile.content, null, 2)
}

export type CredentialExportTarget = {
  id: number
  name: string
  keyMasked?: string
}

interface CredentialExportDialogProps {
  target: CredentialExportTarget | null
  onOpenChange: (open: boolean) => void
}

export function CredentialExportDialog({
  target,
  onOpenChange,
}: CredentialExportDialogProps) {
  return target ? (
    <OpenCredentialExportDialog
      key={target.id}
      target={target}
      onOpenChange={onOpenChange}
    />
  ) : null
}

function OpenCredentialExportDialog({
  target,
  onOpenChange,
}: {
  target: CredentialExportTarget
  onOpenChange: (open: boolean) => void
}) {
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
  const [pendingDeepLink, setPendingDeepLink] = useState<{
    url: string
    fallback: string
  } | null>(null)

  useEffect(
    () => () => {
      const controller = exportRequest.current
      exportRequest.current = null
      controller?.abort()
      tokenRef.current = ''
      if (copyTimer.current !== null) clearTimeout(copyTimer.current)
    },
    []
  )

  function handleOpenChange(open: boolean) {
    if (!open) {
      const controller = exportRequest.current
      exportRequest.current = null
      controller?.abort()
      tokenRef.current = ''
      setUnlockDraft('')
      setData(null)
      setShowKey(false)
      setPendingDeepLink(null)
      setModel('')
      setExportPhase('locked')
      if (copyTimer.current !== null) clearTimeout(copyTimer.current)
      setCopiedKey(false)
    }
    onOpenChange(open)
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
        target.id,
        'all',
        token,
        model || undefined,
        controller.signal
      )
      if (exportRequest.current !== controller) return
      if (response.keyId !== target.id || (model && response.model !== model))
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

  const locked = exportPhase === 'locked'
  const reauthRejected = exportPhase === 'rejected'
  const apiBase = useMemo(
    () => `${(data?.baseUrl ?? '').replace(/\/+$/, '')}/v1`,
    [data]
  )

  const genericProfile = data?.profiles.find((p) => p.id === 'generic')
  const cherryProfile = data?.profiles.find((p) => p.id === 'cherry')
  const selectedProfile = data?.profiles.find((p) => p.id === client)
  const protocol = selectedProfile?.protocol ?? PROFILE_PROTOCOL[client]
  const clientBase = client === 'claude-code' ? (data?.baseUrl ?? '') : apiBase
  const currentConfiguration =
    exportPhase === 'ready' && !!model && data?.model === model
  const ccApp = CC_SWITCH_APP[client]
  let configurationHint = t('connect.modelRequired')
  if (model)
    configurationHint = currentConfiguration
      ? t('connect.contextHint')
      : t('connect.configStale')
  const keyContent = cherryProfile?.content ?? genericProfile?.content
  const apiKey =
    typeof keyContent === 'object' && typeof keyContent.apiKey === 'string'
      ? keyContent.apiKey
      : ''
  function changeModel(value: string) {
    if (value === model) return
    const controller = exportRequest.current
    exportRequest.current = null
    controller?.abort()
    setExportPhase('ready')
    setPendingDeepLink(null)
    setShowKey(false)
    setModel(value)
  }

  function changeClient(value: ExportProfileId) {
    if (value === client) return
    const controller = exportRequest.current
    exportRequest.current = null
    controller?.abort()
    setExportPhase('ready')
    setPendingDeepLink(null)
    setShowKey(false)
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

  async function handleCopyKey() {
    if (!(await copyTextWithToast(apiKey, 'connect.toast.keyCopied', t))) return
    setCopiedKey(true)
    if (copyTimer.current !== null) clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopiedKey(false), 1500)
  }

  function requestDeepLink(url: string, fallbackText: string) {
    setPendingDeepLink({ url, fallback: fallbackText })
  }

  function confirmDeepLink() {
    if (!pendingDeepLink) return
    const { url } = pendingDeepLink
    setPendingDeepLink(null)
    window.location.href = url
  }

  function renderLocked() {
    return (
      <div className='space-y-3 rounded-lg border border-dashed px-4 py-6'>
        <div className='flex items-center gap-2'>
          <Lock className='text-muted-foreground size-4' />
          <p className='text-sm font-medium'>{t('connect.lockedTitle')}</p>
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('connect.lockedDescription')}
        </p>
        <form
          className='space-y-2'
          onSubmit={(event) => {
            event.preventDefault()
            handleUnlock()
          }}
        >
          <Input
            type='password'
            autoComplete='current-password'
            value={unlockDraft}
            onChange={(event) => setUnlockDraft(event.target.value)}
            placeholder={t('reauth.tokenPlaceholder')}
            aria-label={t('reauth.tokenLabel')}
          />
          {reauthRejected && (
            <p role='alert' className='text-destructive-soft-fg text-xs'>
              {t('reauth.errorInvalid')}
            </p>
          )}
          <Button type='submit' size='sm' disabled={!unlockDraft.trim()}>
            {t('connect.unlock')}
          </Button>
        </form>
      </div>
    )
  }

  function renderBody() {
    if (locked || reauthRejected) {
      return renderLocked()
    }
    if (exportPhase === 'loading' && !data) {
      return (
        <div className='space-y-3'>
          <Skeleton className='h-16 w-full rounded-lg' />
          <Skeleton className='h-24 w-full rounded-lg' />
        </div>
      )
    }
    if (exportPhase === 'error' || !data) {
      return (
        <div className='rounded-lg border border-dashed px-4 py-6 text-center'>
          <p className='text-destructive-soft-fg text-sm'>
            {t('connect.loadFailed')}
          </p>
          <Button
            variant='outline'
            size='sm'
            className='mt-3'
            onClick={() => void loadExport(tokenRef.current)}
          >
            {t('common.retry')}
          </Button>
        </div>
      )
    }
    return (
      <div className='space-y-4'>
        {/* Gateway endpoint */}
        <div className='bg-muted/40 rounded-lg border p-3'>
          <p className='text-muted-foreground mb-1 text-xs font-medium'>
            {t('connect.endpointLabel')}
          </p>
          <div className='flex items-center gap-2'>
            <code className='text-foreground flex-1 truncate font-mono text-sm'>
              {clientBase}
            </code>
            <Button
              variant='outline'
              size='icon-sm'
              aria-label={t('connect.copyEndpoint')}
              onClick={() =>
                void copyTextWithToast(
                  clientBase,
                  'connect.toast.endpointCopied',
                  t
                )
              }
            >
              <Copy className='size-3.5' />
            </Button>
          </div>
          <div className='mt-2 flex items-center gap-2 border-t pt-2'>
            <code className='text-muted-foreground flex-1 truncate font-mono text-xs'>
              {showKey ? apiKey : (target.keyMasked ?? 'sk-…')}
            </code>
            <Button
              variant='outline'
              size='icon-sm'
              aria-label={t(showKey ? 'connect.hideKey' : 'connect.showKey')}
              onClick={() => setShowKey((value) => !value)}
            >
              {showKey ? (
                <EyeOff className='size-3.5' />
              ) : (
                <Eye className='size-3.5' />
              )}
            </Button>
            <Button
              variant='outline'
              size='icon-sm'
              aria-label={t(
                copiedKey ? 'connect.toast.keyCopied' : 'connect.copyKey'
              )}
              onClick={() => void handleCopyKey()}
            >
              {copiedKey ? (
                <Check className='text-success size-3.5' />
              ) : (
                <Copy className='size-3.5' />
              )}
            </Button>
          </div>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='credential-client'>{t('connect.clientLabel')}</Label>
          <Select
            items={data.profiles.map((profile) => ({
              value: profile.id,
              label: profile.label,
            }))}
            value={client}
            onValueChange={(value) => {
              if (value) changeClient(value as ExportProfileId)
            }}
          >
            <SelectTrigger id='credential-client' className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {data.profiles.map((profile) => (
                <SelectItem key={profile.id} value={profile.id}>
                  {profile.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className='text-muted-foreground text-xs break-all'>
            {t('connect.modelLabel')}: {model || t('connect.modelRequired')}
          </p>
        </div>

        <Tabs defaultValue='verify'>
          <TabsList className='w-full'>
            <TabsTrigger value='verify'>{t('connect.verifyTab')}</TabsTrigger>
            <TabsTrigger value='clients'>{t('connect.clientsTab')}</TabsTrigger>
          </TabsList>
          <TabsContent value='verify' keepMounted>
            {apiKey ? (
              <CredentialVerifySection
                keyId={target.id}
                apiBase={apiBase}
                apiKey={apiKey}
                contextId={client}
                model={model}
                protocol={protocol}
                onModelChange={changeModel}
                onProtocolChange={changeProtocol}
              />
            ) : null}
          </TabsContent>
          <TabsContent value='clients' className='space-y-4'>
            <div className='space-y-2'>
              <p className='text-muted-foreground text-xs'>
                {configurationHint}
              </p>
              <Button
                size='sm'
                variant='outline'
                disabled={!model || exportPhase === 'loading'}
                onClick={() => void loadExport(tokenRef.current)}
              >
                {t('connect.updateConfig')}
              </Button>
            </div>
            {/* One-click deep links (explicit confirmation, #1034) */}
            <div className='space-y-2'>
              {model && (ccApp || client === 'cherry') ? (
                <p className='text-muted-foreground text-xs'>
                  {t('connect.providerImportHint', { model })}
                </p>
              ) : null}
              <p className='text-muted-foreground text-xs font-medium'>
                {t('connect.oneClickLabel')}
              </p>
              <div className='flex flex-wrap gap-2'>
                {client === 'cherry' ? (
                  <Button
                    size='sm'
                    disabled={!apiKey || !currentConfiguration}
                    onClick={() =>
                      requestDeepLink(
                        buildCherryStudioLink(
                          target.id,
                          target.name,
                          apiBase,
                          apiKey
                        ),
                        JSON.stringify(
                          {
                            name: target.name,
                            type: 'openai',
                            apiHost: apiBase,
                            apiKey,
                          },
                          null,
                          2
                        )
                      )
                    }
                  >
                    <Sparkles className='size-3.5' />
                    {t('connect.cherryStudio')}
                  </Button>
                ) : null}
                {ccApp ? (
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={!apiKey || !currentConfiguration}
                    onClick={() =>
                      requestDeepLink(
                        buildCcSwitchLink(
                          ccApp,
                          client === 'claude-code' ? data.baseUrl : apiBase,
                          apiKey
                        ),
                        selectedProfile
                          ? stringifyProfileContent(selectedProfile)
                          : `${apiBase} ${apiKey}`
                      )
                    }
                  >
                    <ExternalLink className='size-4' />
                    {t('connect.ccSwitch')}
                  </Button>
                ) : null}
              </div>
              {pendingDeepLink ? (
                <div className='space-y-2 rounded-lg border border-dashed p-3'>
                  <p className='text-sm font-medium'>
                    {t('connect.deepLinkConfirmTitle')}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {t('connect.deepLinkConfirmDescription')}
                  </p>
                  <div className='flex gap-2'>
                    <Button size='sm' onClick={confirmDeepLink}>
                      {t('connect.deepLinkConfirmContinue')}
                    </Button>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() =>
                        void copyTextWithToast(
                          pendingDeepLink.fallback,
                          'connect.toast.jsonCopied',
                          t
                        )
                      }
                    >
                      {t('connect.copyFallback')}
                    </Button>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => setPendingDeepLink(null)}
                    >
                      {t('settings.common.cancel')}
                    </Button>
                  </div>
                </div>
              ) : null}
            </div>

            {currentConfiguration && selectedProfile ? (
              <div className='space-y-2'>
                <ProfileRow
                  title={selectedProfile.label}
                  description={selectedProfile.description ?? ''}
                  content={stringifyProfileContent(selectedProfile)}
                  masked={!showKey}
                  onCopy={(text) =>
                    void copyTextWithToast(text, 'connect.toast.jsonCopied', t)
                  }
                />
                {selectedProfile.setup ? (
                  <ProfileRow
                    title={t('connect.setupStep')}
                    description={t('connect.setupHint')}
                    content={selectedProfile.setup.content}
                    masked={!showKey}
                    onCopy={(text) =>
                      void copyTextWithToast(
                        text,
                        'connect.toast.jsonCopied',
                        t
                      )
                    }
                  />
                ) : null}
              </div>
            ) : null}

            <p className='text-muted-foreground text-xs'>{t('connect.hint')}</p>
          </TabsContent>
        </Tabs>
      </div>
    )
  }

  return (
    <Dialog open onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <Link2 className='text-primary size-4' />
            {t('connect.title', { name: target.name })}
          </DialogTitle>
          <DialogDescription>{t('connect.description')}</DialogDescription>
        </DialogHeader>

        {renderBody()}

        <DialogFooter>
          <Button variant='outline' onClick={() => handleOpenChange(false)}>
            {t('settings.common.cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ProfileRow({
  title,
  description,
  content,
  masked = false,
  onCopy,
}: {
  title: string
  description: string
  content: string
  /** #1034: hide the secret in the preview pane; copy still uses the real text. */
  masked?: boolean
  onCopy: (text: string) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='rounded-lg border'>
      <div className='flex items-center justify-between gap-2 px-3 py-2'>
        <div className='min-w-0'>
          <p className='text-sm font-medium'>{title}</p>
          <p className='text-muted-foreground truncate text-xs'>
            {description}
          </p>
        </div>
        <Button variant='outline' size='sm' onClick={() => onCopy(content)}>
          <Copy className='mr-1 size-3.5' />
          {t('connect.copy')}
        </Button>
      </div>
      {!masked ? (
        <pre className='bg-muted/40 max-h-28 overflow-auto rounded-b-lg border-t px-3 py-2 font-mono text-xs'>
          {content}
        </pre>
      ) : null}
    </div>
  )
}
