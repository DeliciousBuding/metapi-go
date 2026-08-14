// metapi-go/components/common — credential export dialog.
//
// The "distribution" surface for a downstream API key: shows the gateway
// base URL, offers one-click deep-link imports (Cherry Studio / CC Switch)
// and copy-to-clipboard profiles (env vars / generic JSON) fetched from
// GET /api/downstream-keys/{id}/export.
//
// Deep links are opened via location.href; when the protocol handler is not
// installed the navigation is a no-op, so every deep-link action also
// exposes an adjacent copy fallback.

import { useQuery } from '@tanstack/react-query'
import { Check, Copy, ExternalLink, Link2, Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
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
async function copyText(
  text: string,
  successKey: string,
  t: (key: string) => string
) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t(successKey))
  } catch {
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

function buildCcSwitchLink(app: string, apiBase: string, apiKey: string): string {
  const params = new URLSearchParams({
    resource: 'provider',
    app,
    endpoint: apiBase,
    apiKey,
  })
  return `ccswitch://v1/import?${params.toString()}`
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

  const open = target !== null

  const exportQuery = useQuery<ExportResponse>({
    queryKey: ['downstream-key-export', target?.id],
    queryFn: async () =>
      (await api.getDownstreamKeyExport(target?.id ?? 0)) as ExportResponse,
    enabled: open && target?.id != null,
    staleTime: 30 * 1000,
  })

  const data = exportQuery.data
  const apiBase = useMemo(
    () => `${(data?.baseUrl ?? '').replace(/\/+$/, '')}/v1`,
    [data]
  )

  const genericProfile = data?.profiles.find((p) => p.id === 'generic')
  const envProfile = data?.profiles.find((p) => p.id === 'openai')
  const cherryProfile = data?.profiles.find((p) => p.id === 'cherry')
  const apiKey =
    (cherryProfile?.content as { apiKey?: string } | undefined)?.apiKey ??
    (genericProfile?.content as { apiKey?: string } | undefined)?.apiKey ??
    ''
  const genericJson = genericProfile
    ? JSON.stringify(genericProfile.content, null, 2)
    : ''

  async function handleCopyKey() {
    await copyText(apiKey, 'connect.toast.keyCopied', t)
    setCopiedKey(true)
    setTimeout(() => setCopiedKey(false), 1500)
  }

  function openDeepLink(url: string, fallbackText: string) {
    window.location.href = url
    // Deep links are a no-op when the protocol handler is missing; the
    // delayed copy doubles as the manual fallback path.
    setTimeout(() => {
      void copyText(fallbackText, 'connect.toast.jsonCopied', t)
    }, 400)
  }

  function renderBody() {
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
                void copyText(apiBase, 'connect.toast.endpointCopied', t)
              }
            >
              <Copy className='size-3.5' />
            </Button>
          </div>
          <div className='mt-2 flex items-center gap-2 border-t pt-2'>
            <code className='text-muted-foreground flex-1 truncate font-mono text-xs'>
              {target?.keyMasked ?? 'sk-…'}
            </code>
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

        {/* One-click deep links */}
        <div className='space-y-2'>
          <p className='text-muted-foreground text-xs font-medium'>
            {t('connect.oneClickLabel')}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              size='sm'
              disabled={!apiKey}
              onClick={() =>
                openDeepLink(
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
              <Sparkles className='mr-1 size-3.5' />
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
                  openDeepLink(
                    buildCcSwitchLink(ccApp, apiBase, apiKey),
                    `${apiBase} ${apiKey}`
                  )
                }
              >
                <ExternalLink className='mr-1 size-3.5' />
                {t('connect.ccSwitch')}
              </Button>
            </div>
          </div>
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
                onCopy={(text) =>
                  void copyText(text, 'connect.toast.jsonCopied', t)
                }
              />
            ) : null}
            {genericProfile ? (
              <ProfileRow
                title={t('connect.genericJson')}
                description={genericProfile.description ?? ''}
                content={genericJson}
                onCopy={(text) =>
                  void copyText(text, 'connect.toast.jsonCopied', t)
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
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
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
  onCopy,
}: {
  title: string
  description: string
  content: string
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
          'bg-muted/40 max-h-28 overflow-auto rounded-b-lg border-t px-3 py-2 font-mono text-xs'
        )}
      >
        {content}
      </pre>
    </div>
  )
}
