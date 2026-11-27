import { Copy, ExternalLink, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type {
  CredentialExportProfile as ExportProfile,
  CredentialExportProfileId as ExportProfileId,
} from '@/lib/api/credential-export'
import { cn } from '@/lib/utils'

import {
  buildClientImport,
  CLIENT_CONFIG_META,
  stringifyProfileContent,
  type ClientContext,
  type ClientImport,
  type CredentialExportTarget,
} from './credential-export-profile'

interface ClientSetupProps {
  target: CredentialExportTarget
  context: ClientContext
  profiles: ExportProfile[]
  model: string
  currentConfiguration: boolean
  loading: boolean
  showKey: boolean
  pendingDeepLink: ClientImport | null
  setPendingDeepLink: (value: ClientImport | null) => void
  changeClient: (client: ExportProfileId) => void
  onRefresh: () => void
  onCopy: (text: string) => void
}

export function CredentialClientSetup({
  target,
  context,
  profiles,
  model,
  currentConfiguration,
  loading,
  showKey,
  pendingDeepLink,
  setPendingDeepLink,
  changeClient,
  onRefresh,
  onCopy,
}: ClientSetupProps) {
  const { t } = useTranslation()
  const { client, selectedProfile, apiKey, ccApp } = context
  let configurationHint = t('connect.modelRequired')
  if (model)
    configurationHint = currentConfiguration
      ? t('connect.contextHint')
      : t('connect.configStale')
  function confirmDeepLink() {
    if (!pendingDeepLink) return
    const { url } = pendingDeepLink
    setPendingDeepLink(null)
    window.location.href = url
  }

  return (
    <>
      <div
        role='group'
        aria-label={t('connect.clientCardsLabel')}
        className='grid grid-cols-2 gap-2 sm:grid-cols-3'
      >
        {profiles.map((profile) => {
          const selected = profile.id === client
          const meta = CLIENT_CONFIG_META[profile.id]
          return (
            <button
              key={profile.id}
              type='button'
              aria-pressed={selected}
              onClick={() => changeClient(profile.id)}
              className={cn(
                'focus-visible:ring-ring flex flex-col items-start gap-1 rounded-lg border p-2.5 text-left text-xs transition-colors outline-none focus-visible:ring-2',
                selected
                  ? 'border-primary bg-accent/60 text-foreground font-medium'
                  : 'bg-background hover:bg-muted/50 text-muted-foreground'
              )}
            >
              <span className='text-foreground w-full truncate font-medium'>
                {profile.label}
              </span>
              <Badge
                variant={meta.requiresV1 ? 'outline' : 'secondary'}
                className='text-2xs h-4 px-1.5 font-mono'
              >
                {meta.requiresV1 ? '/v1' : 'root'}
              </Badge>
            </button>
          )
        })}
      </div>

      <div className='bg-muted/30 space-y-1 rounded-lg border px-3 py-2 text-xs'>
        <p className='text-foreground font-medium'>
          {t(
            CLIENT_CONFIG_META[client].requiresV1
              ? 'connect.endpointWithV1'
              : 'connect.endpointWithoutV1'
          )}
        </p>
        <p className='text-muted-foreground break-words'>
          {t('connect.configFilePath')}:{' '}
          <code className='font-mono'>
            {t(CLIENT_CONFIG_META[client].pathKey)}
          </code>
        </p>
      </div>

      <div className='space-y-2'>
        <p className='text-muted-foreground text-xs'>{configurationHint}</p>
        <Button
          size='sm'
          variant='outline'
          disabled={!model || loading}
          onClick={onRefresh}
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
          {client === 'cherry' || ccApp ? (
            <Button
              size='sm'
              variant={client === 'cherry' ? 'default' : 'outline'}
              disabled={!apiKey || !currentConfiguration}
              onClick={() =>
                setPendingDeepLink(buildClientImport(target, context))
              }
            >
              {client === 'cherry' ? (
                <Sparkles className='size-3.5' />
              ) : (
                <ExternalLink className='size-4' />
              )}
              {t(
                client === 'cherry'
                  ? 'connect.cherryStudio'
                  : 'connect.ccSwitch'
              )}
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
                onClick={() => onCopy(pendingDeepLink.fallback)}
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
            onCopy={onCopy}
          />
          {selectedProfile.setup ? (
            <ProfileRow
              title={t('connect.setupStep')}
              description={t('connect.setupHint')}
              content={selectedProfile.setup.content}
              masked={!showKey}
              onCopy={onCopy}
            />
          ) : null}
        </div>
      ) : null}

      <p className='text-muted-foreground text-xs'>{t('connect.hint')}</p>
    </>
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
      ) : (
        <div
          data-testid='masked-preview-placeholder'
          className='bg-muted/30 text-muted-foreground rounded-b-lg border-t px-3 py-2 font-mono text-xs'
        >
          •••••••••••• · {t('connect.maskedPreviewPlaceholder')}
        </div>
      )}
    </div>
  )
}
