// metapi-go/components/common — credential export dialog.
//
// The "distribution" surface for a downstream API key: shows the gateway
// base URL, offers one-click deep-link imports (Cherry Studio / CC Switch)
// and copy-to-clipboard profiles (env vars / generic JSON) fetched from
// GET /api/downstream-keys/{id}/export.
//
// #1034 hardening: the dialog opens LOCKED. The export endpoint is a
// sensitive op, so the operator must re-enter the master token before any
// credential material is fetched (X-Admin-Confirm-Token), the key renders
// masked until explicitly revealed, and deep links — which hand the plain
// key to an external app — require an explicit confirmation step.
//
// Deep links are opened via location.href; when the protocol handler is not
// installed the navigation is a no-op, so every deep-link action also
// exposes an adjacent copy fallback.

import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
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
import { useEffect, useMemo, useState } from 'react'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { isReauthRequired } from '@/lib/http-client'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

type ExportProfile = {
  id: string
  label: string
  description?: string
  contentType?: string
  content: unknown
}

type ExportResponse = {
  success: boolean
  formatVersion: string
  keyId: number
  keyName: string
  baseUrl: string
  profiles: ExportProfile[]
}

const CC_SWITCH_APPS = ['codex', 'claude', 'gemini', 'cursor'] as const

/** Clipboard write with a unified success toast. */
async function copyTextWithToast(
  text: string,
  successKey: string,
  t: (key: string) => string
) {
  if (await copyText(text)) {
    toast.success(t(successKey))
  } else {
    // Clipboard may be unavailable (non-secure context / permissions).
    toast.error(t('connect.copyFailed'))
  }
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
  const { t } = useTranslation()
  const [ccApp, setCcApp] = useState<string>('codex')
  const [copiedKey, setCopiedKey] = useState(false)
  // #1034: locked until the master token is re-entered; the token is held
  // in component state only (never persisted) and reset on close.
  const [confirmToken, setConfirmToken] = useState<string | null>(null)
  const [unlockDraft, setUnlockDraft] = useState('')
  const [unlockError, setUnlockError] = useState<string | null>(null)
  // #1034: the key renders masked until explicitly revealed.
  const [showKey, setShowKey] = useState(false)
  // #1034: deep links hand the plain key to an external app — they wait for
  // an explicit confirmation step before navigating.
  const [pendingDeepLink, setPendingDeepLink] = useState<{
    url: string
    fallback: string
  } | null>(null)

  const open = target !== null

  // Fresh lock state every time the dialog opens.
  useEffect(() => {
    if (!open) {
      setConfirmToken(null)
      setUnlockDraft('')
      setUnlockError(null)
      setShowKey(false)
      setPendingDeepLink(null)
    }
  }, [open])

  const exportQuery = useQuery<ExportResponse>({
    queryKey: ['downstream-key-export', target?.id, confirmToken !== null],
    queryFn: async () =>
      (await api.getDownstreamKeyExport(
        target?.id ?? 0,
        'all',
        confirmToken ?? undefined
      )) as ExportResponse,
    // Locked dialog never fetches: no credential material is even requested
    // until the master token is presented.
    enabled: open && target?.id != null && confirmToken !== null,
    staleTime: 30 * 1000,
    retry: false,
  })

  const locked = confirmToken === null
  const reauthRejected =
    exportQuery.isError && isReauthRequired(exportQuery.error)

  const data = exportQuery.data
  const apiBase = useMemo(
    () => `${(data?.baseUrl ?? '').replace(/\/+$/, '')}/v1`,
    [data]
  )

  const genericProfile = data?.profiles.find((p) => p.id === 'generic')
  const envProfile = data?.profiles.find((p) => p.id === 'openai')
  const cherryProfile = data?.profiles.find((p) => p.id === 'cherry')
  const claudeCodeProfile = data?.profiles.find((p) => p.id === 'claude-code')
  const codexProfile = data?.profiles.find((p) => p.id === 'codex')
  const openWebUiProfile = data?.profiles.find((p) => p.id === 'openwebui')
  const apiKey =
    (cherryProfile?.content as { apiKey?: string } | undefined)?.apiKey ??
    (genericProfile?.content as { apiKey?: string } | undefined)?.apiKey ??
    ''
  const genericJson = genericProfile
    ? JSON.stringify(genericProfile.content, null, 2)
    : ''

  function handleUnlock() {
    const token = unlockDraft.trim()
    if (!token) return
    setUnlockError(null)
    setConfirmToken(token)
  }

  async function handleCopyKey() {
    await copyTextWithToast(apiKey, 'connect.toast.keyCopied', t)
    setCopiedKey(true)
    setTimeout(() => setCopiedKey(false), 1500)
  }

  function requestDeepLink(url: string, fallbackText: string) {
    setPendingDeepLink({ url, fallback: fallbackText })
  }

  function confirmDeepLink() {
    if (!pendingDeepLink) return
    const { url, fallback } = pendingDeepLink
    setPendingDeepLink(null)
    window.location.href = url
    // Deep links are a no-op when the protocol handler is missing; the
    // delayed copy doubles as the manual fallback path.
    setTimeout(() => {
      void copyTextWithToast(fallback, 'connect.toast.jsonCopied', t)
    }, 400)
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
          {(unlockError || reauthRejected) && (
            <p className='text-destructive text-xs'>
              {unlockError ?? t('reauth.errorInvalid')}
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
    if (exportQuery.isLoading) {
      return (
        <div className='space-y-3'>
          <Skeleton className='h-16 w-full rounded-lg' />
          <Skeleton className='h-24 w-full rounded-lg' />
        </div>
      )
    }
    if (exportQuery.isError || !data) {
      return (
        <div className='rounded-lg border border-dashed px-4 py-6 text-center'>
          <p className='text-destructive text-sm'>{t('connect.loadFailed')}</p>
          <Button
            variant='outline'
            size='sm'
            className='mt-3'
            onClick={() => void exportQuery.refetch()}
          >
            {t('settings.common.retry')}
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
              {apiBase}
            </code>
            <Button
              variant='outline'
              size='icon-sm'
              aria-label={t('connect.copyEndpoint')}
              onClick={() =>
                void copyTextWithToast(
                  apiBase,
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
              {showKey ? apiKey : (target?.keyMasked ?? 'sk-…')}
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
              aria-label={t('connect.copyKey')}
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

        {/* One-click deep links (explicit confirmation, #1034) */}
        <div className='space-y-2'>
          <p className='text-muted-foreground text-xs font-medium'>
            {t('connect.oneClickLabel')}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              size='sm'
              disabled={!apiKey}
              onClick={() =>
                requestDeepLink(
                  buildCherryStudioLink(
                    target?.id ?? 0,
                    target?.name ?? '',
                    apiBase,
                    apiKey
                  ),
                  JSON.stringify(
                    {
                      name: target?.name ?? '',
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
            <div className='flex gap-2'>
              <Select
                value={ccApp}
                onValueChange={(value) => setCcApp(value ?? 'codex')}
              >
                <SelectTrigger
                  className='h-8 w-28 text-xs'
                  aria-label={t('connect.ccAppLabel')}
                >
                  <SelectValue>{ccApp}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {CC_SWITCH_APPS.map((app) => (
                    <SelectItem key={app} value={app}>
                      {app}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                size='sm'
                variant='outline'
                disabled={!apiKey}
                onClick={() =>
                  requestDeepLink(
                    buildCcSwitchLink(ccApp, apiBase, apiKey),
                    `${apiBase} ${apiKey}`
                  )
                }
              >
                <ExternalLink className='size-4' />
                {t('connect.ccSwitch')}
              </Button>
            </div>
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
                  onClick={() => setPendingDeepLink(null)}
                >
                  {t('settings.common.cancel')}
                </Button>
              </div>
            </div>
          ) : null}
        </div>

        {/* Copy profiles */}
        <div className='space-y-2'>
          <p className='text-muted-foreground text-xs font-medium'>
            {t('connect.copyLabel')}
          </p>
          <div className='space-y-2'>
            {envProfile ? (
              <ProfileRow
                title={t('connect.envVars')}
                description={envProfile.description ?? ''}
                content={String(envProfile.content)}
                masked={!showKey}
                onCopy={(text) =>
                  void copyTextWithToast(text, 'connect.toast.jsonCopied', t)
                }
              />
            ) : null}
            {genericProfile ? (
              <ProfileRow
                title={t('connect.genericJson')}
                description={genericProfile.description ?? ''}
                content={genericJson}
                masked={!showKey}
                onCopy={(text) =>
                  void copyTextWithToast(text, 'connect.toast.jsonCopied', t)
                }
              />
            ) : null}
            {claudeCodeProfile ? (
              <ProfileRow
                title={claudeCodeProfile.label}
                description={claudeCodeProfile.description ?? ''}
                content={stringifyProfileContent(claudeCodeProfile)}
                masked={!showKey}
                onCopy={(text) =>
                  void copyTextWithToast(text, 'connect.toast.jsonCopied', t)
                }
              />
            ) : null}
            {codexProfile ? (
              <ProfileRow
                title={codexProfile.label}
                description={codexProfile.description ?? ''}
                content={stringifyProfileContent(codexProfile)}
                masked={!showKey}
                onCopy={(text) =>
                  void copyTextWithToast(text, 'connect.toast.jsonCopied', t)
                }
              />
            ) : null}
            {openWebUiProfile ? (
              <ProfileRow
                title={openWebUiProfile.label}
                description={openWebUiProfile.description ?? ''}
                content={stringifyProfileContent(openWebUiProfile)}
                masked={!showKey}
                onCopy={(text) =>
                  void copyTextWithToast(text, 'connect.toast.jsonCopied', t)
                }
              />
            ) : null}
          </div>
        </div>

        <p className='text-muted-foreground text-xs'>{t('connect.hint')}</p>
      </div>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <Link2 className='text-primary size-4' />
            {t('connect.title', { name: target?.name ?? '' })}
          </DialogTitle>
          <DialogDescription>{t('connect.description')}</DialogDescription>
        </DialogHeader>

        {renderBody()}

        <DialogFooter>
          <Button
            variant='secondary'
            render={<Link to='/model-tester' />}
            onClick={() => onOpenChange(false)}
          >
            {t('connect.testRequest')}
          </Button>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
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
      <pre
        className={cn(
          'bg-muted/40 max-h-28 overflow-auto rounded-b-lg border-t px-3 py-2 font-mono text-xs',
          masked && 'select-none'
        )}
        aria-hidden={masked || undefined}
      >
        {/* Masked profile must not keep the plaintext in the DOM/accessibility
            tree — blur is visual-only and screen readers would read the key
            (W19-T2 flow). Render a placeholder instead, matching the top-level
            key row's real string mask; the copy button still uses the value. */}
        {masked ? '••••••••' : content}
      </pre>
    </div>
  )
}
