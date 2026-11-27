import {
  Check as CheckIcon,
  Copy as CopyIcon,
  ExternalLink as ExternalLinkIcon,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button, buttonVariants } from '@/components/ui/button'
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { copyText } from '@/lib/clipboard'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

import type { OAuthSessionPolling } from '../lib/oauth-session-polling'
import type { OAuthStartInstructions } from '../types'
import {
  ManualCallbackForm,
  type useManualCallback,
} from './oauth-manual-callback'

export function useInstructionCopy() {
  const { t } = useTranslation()
  const [copiedField, setCopiedField] = useState<string | null>(null)
  async function handleCopy(field: string, text: string) {
    if (await copyText(text)) {
      setCopiedField(field)
      setTimeout(
        () => setCopiedField((current) => (current === field ? null : current)),
        1500
      )
    } else {
      // Clipboard may be unavailable (non-secure context / permissions).
      toast.error(t('common.copyFailed'))
    }
  }
  return { copiedField, handleCopy }
}

function CopyValue({
  field,
  value,
  copy,
  deviceCode = false,
}: {
  field: string
  value: string
  copy: ReturnType<typeof useInstructionCopy>
  deviceCode?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className='flex items-center gap-2'>
      <code
        className={
          deviceCode
            ? 'bg-muted flex-1 rounded px-3 py-2 text-center text-base font-semibold tracking-widest'
            : 'bg-muted flex-1 overflow-x-auto rounded px-2 py-1.5 text-xs'
        }
      >
        {value}
      </code>
      <Button
        type='button'
        variant='outline'
        size='icon-sm'
        onClick={() => copy.handleCopy(field, value)}
        aria-label={t('common.copy')}
      >
        {copy.copiedField === field ? (
          <CheckIcon className='size-3.5' />
        ) : (
          <CopyIcon className='size-3.5' />
        )}
      </Button>
    </div>
  )
}

export function OAuthSessionPanel({
  pendingState,
  instructions,
  authorizationUrl,
  polling,
  copy,
  callback,
  requestClose,
}: {
  pendingState: string
  instructions: OAuthStartInstructions | null
  authorizationUrl: string | null
  polling: OAuthSessionPolling
  copy: ReturnType<typeof useInstructionCopy>
  callback: ReturnType<typeof useManualCallback>
  requestClose: () => void
}) {
  const { t } = useTranslation()
  const { session, exhausted, maxAttempts } = polling
  const deviceFlow = instructions?.deviceFlow
  const redirectUri = instructions?.redirectUri
  const sshTunnelCommand = instructions?.sshTunnelCommand
  const sshTunnelKeyCommand = instructions?.sshTunnelKeyCommand

  // Pending-panel status row: backend error > honest exhaustion > waiting.
  let statusPanel: React.ReactNode
  if (session?.status === 'error') {
    statusPanel = (
      <div className='text-destructive text-sm'>
        {t('oauth.session.failed', {
          error: session?.error ?? '',
        })}
      </div>
    )
  } else if (exhausted) {
    // Honest timeout: polling paused, the wait is NOT over.
    statusPanel = (
      <div className='text-warning-soft-fg text-sm' role='status'>
        {deviceFlow
          ? t('oauth.session.devicePollingExhausted', {
              attempts: maxAttempts,
            })
          : t('oauth.session.pollingExhausted', {
              attempts: maxAttempts,
            })}
      </div>
    )
  } else {
    statusPanel = (
      <div className='text-muted-foreground flex items-center gap-2 text-sm'>
        <Spinner className='size-3.5' />
        <span>
          {deviceFlow
            ? t('oauth.session.deviceWaiting')
            : t('oauth.session.waiting')}
        </span>
      </div>
    )
  }
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('oauth.session.waiting')}</DialogTitle>
        <DialogDescription>
          {t('oauth.session.waitingDescription')}
        </DialogDescription>
      </DialogHeader>

      <div className='grid gap-4'>
        {statusPanel}

        {authorizationUrl && (
          <div className='grid gap-2'>
            <a
              href={authorizationUrl}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(
                buttonVariants({ variant: 'outline', size: 'sm' }),
                'w-fit'
              )}
            >
              <ExternalLinkIcon className='size-3.5' />
              {t('oauth.session.openAuthorization')}
            </a>
            <p className='text-muted-foreground text-xs'>
              {t('oauth.session.popupBlockedHint')}
            </p>
          </div>
        )}

        {deviceFlow && (
          <div className='grid gap-2'>
            <p className='text-muted-foreground text-sm'>
              {t('oauth.session.deviceCodeHint')}
            </p>
            <CopyValue
              field='deviceUserCode'
              value={deviceFlow.userCode}
              copy={copy}
              deviceCode
            />
          </div>
        )}

        <div className='grid gap-2'>
          <p className='text-muted-foreground text-sm'>
            {t('oauth.session.stateHint')}
          </p>
          <CopyValue field='state' value={pendingState} copy={copy} />
        </div>

        {!deviceFlow && redirectUri && (
          <div className='grid gap-2'>
            <p className='text-muted-foreground text-sm'>
              {t('oauth.session.redirectUriHint')}
            </p>
            <CopyValue field='redirectUri' value={redirectUri} copy={copy} />
          </div>
        )}

        {!deviceFlow && sshTunnelCommand && (
          <div className='grid gap-2'>
            <p className='text-muted-foreground text-sm'>
              {t('oauth.session.sshTunnelHint')}
            </p>
            <CopyValue field='sshTunnel' value={sshTunnelCommand} copy={copy} />
            {sshTunnelKeyCommand && (
              <>
                <p className='text-muted-foreground text-sm'>
                  {t('oauth.session.sshTunnelKeyHint')}
                </p>
                <CopyValue
                  field='sshTunnelKey'
                  value={sshTunnelKeyCommand}
                  copy={copy}
                />
              </>
            )}
          </div>
        )}

        {!deviceFlow && <ManualCallbackForm callback={callback} />}
        <DialogFooter>
          <Button type='button' variant='outline' onClick={requestClose}>
            {t('oauth.form.cancel')}
          </Button>
        </DialogFooter>
      </div>
    </>
  )
}
