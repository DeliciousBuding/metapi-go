import { CheckCircle2, TriangleAlert } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

import { useVerifyAccountToken } from '../api'
import {
  buildAccountVerifyPayload,
  type AccountFormValues,
} from '../lib/accounts-schema'
import type { CredentialMode } from '../types'

// Inline verification state for the session/apikey credential fields. Resets
// to idle whenever the operator changes site, mode, or credential so a stale
// "verified" result can never be reattached to a different credential.
type VerificationState =
  | { status: 'idle' }
  | { status: 'pending' }
  | { status: 'verified'; tokenType: string; modelCount: number }
  | { status: 'failed'; message: string }

function resolveAxiosErrorMessage(error: unknown): string {
  if (error && typeof error === 'object') {
    const response = (error as { response?: { data?: unknown } }).response
    const data = response?.data
    if (data && typeof data === 'object') {
      const message = (data as { error?: unknown }).error
      if (typeof message === 'string' && message.trim()) return message
    }
  }
  return ''
}

export function useCredentialVerification(
  form: UseFormReturn<AccountFormValues>,
  open: boolean,
  credentialMode: CredentialMode
) {
  const { t } = useTranslation()
  const verifyMutation = useVerifyAccountToken()
  // Inline credential verification (session / apikey only). Password mode
  // binds through the real login submit path, so it never verifies inline.
  const [verification, setVerification] = useState<VerificationState>({
    status: 'idle',
  })
  const verificationRequestId = useRef(0)
  const watchedSiteId = form.watch('siteId')
  const watchedAccessToken = form.watch('accessToken')
  const watchedApiToken = form.watch('apiToken')
  const watchedPlatformUserId = form.watch('platformUserId')
  const watchedProxyUrl = form.watch('proxyUrl')
  useEffect(() => {
    verificationRequestId.current += 1
    setVerification({ status: 'idle' })
  }, [
    open,
    watchedSiteId,
    credentialMode,
    watchedAccessToken,
    watchedApiToken,
    watchedPlatformUserId,
    watchedProxyUrl,
  ])

  const handleVerify = async () => {
    const resolved = buildAccountVerifyPayload(form.getValues())
    if (!resolved.ok) {
      setVerification({
        status: 'failed',
        message: t(
          resolved.error === 'site'
            ? 'accounts.verify.siteRequired'
            : 'accounts.verify.tokenRequired'
        ),
      })
      return
    }
    const currentRequestId = verificationRequestId.current + 1
    verificationRequestId.current = currentRequestId
    setVerification({ status: 'pending' })
    try {
      const result = await verifyMutation.mutateAsync(resolved.payload)
      if (verificationRequestId.current !== currentRequestId) return
      setVerification({
        status: 'verified',
        tokenType: result.tokenType ?? 'unknown',
        modelCount: result.modelCount ?? 0,
      })
    } catch (error) {
      if (verificationRequestId.current !== currentRequestId) return
      const backendMessage = resolveAxiosErrorMessage(error)
      setVerification({
        status: 'failed',
        message: backendMessage || t('accounts.verify.failed'),
      })
    }
  }
  return { verification, onVerify: handleVerify }
}

export function VerifyCredentialFeedback({
  verification,
  onVerify,
}: {
  verification: VerificationState
  onVerify: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-1.5'>
      <Button
        type='button'
        variant='outline'
        size='sm'
        onClick={onVerify}
        disabled={verification.status === 'pending'}
      >
        {verification.status === 'pending' ? (
          <Spinner className='size-3.5' />
        ) : null}
        {t('accounts.verify.button')}
      </Button>
      {verification.status === 'verified' && (
        <p className='text-success flex items-center gap-1.5 text-xs'>
          <CheckCircle2 className='size-3.5' />
          {t('accounts.verify.verified', {
            tokenType: verification.tokenType,
            modelCount: verification.modelCount,
          })}
        </p>
      )}
      {verification.status === 'failed' && (
        <div className='text-destructive space-y-0.5 text-xs'>
          <p className='flex items-center gap-1.5'>
            <TriangleAlert className='size-3.5' />
            {t('accounts.verify.failed')}
          </p>
          {verification.message && (
            <p className='text-destructive/80'>{verification.message}</p>
          )}
        </div>
      )}
    </div>
  )
}

export interface VerifiedCredentialFieldsProps {
  form: UseFormReturn<AccountFormValues>
  verification: VerificationState
  onVerify: () => void
}
