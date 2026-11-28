import { Link } from '@tanstack/react-router'
import { CheckCircle2, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { VerificationState } from './use-credential-verification'

export function CredentialLogsLink({ keyId }: { keyId: number }) {
  const { t } = useTranslation()
  return (
    <Button
      variant='outline'
      size='sm'
      render={<Link to='/proxy-logs' search={{ downstreamKeyId: keyId }} />}
    >
      {t('connect.verify.viewLogs')}
    </Button>
  )
}

export function CredentialVerifyResult({
  verificationState,
  keyId,
}: {
  verificationState: VerificationState
  keyId: number
}) {
  const { t } = useTranslation()
  return (
    <>
      {verificationState.phase === 'ok' ? (
        <div className='flex flex-wrap items-center gap-2'>
          <span className='text-success-soft-fg flex items-center gap-1 text-xs'>
            <CheckCircle2 className='size-3.5' />
            {t('connect.verify.testOk', {
              protocol: t(
                `connect.verify.protocol.${verificationState.protocol}`
              ),
              ms: verificationState.latencyMs,
            })}
          </span>
          <CredentialLogsLink keyId={keyId} />
        </div>
      ) : null}
      {verificationState.phase === 'fail' ? (
        <div className='space-y-2'>
          <p className='text-destructive-soft-fg flex items-start gap-1 text-xs break-all'>
            <XCircle className='mt-0.5 size-3.5 shrink-0' />
            <span>
              {verificationState.status !== null
                ? `HTTP ${verificationState.status}: ${verificationState.message}`
                : verificationState.message}
            </span>
          </p>
          <CredentialLogsLink keyId={keyId} />
        </div>
      ) : null}
    </>
  )
}
