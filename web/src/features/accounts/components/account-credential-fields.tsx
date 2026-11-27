import { CheckCircle2, TriangleAlert } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

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

export function AccountCredentialFields({
  form,
  open,
  credentialMode,
  showPasswordMode,
}: {
  form: UseFormReturn<AccountFormValues>
  open: boolean
  credentialMode: CredentialMode
  showPasswordMode: boolean
}) {
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

  return (
    <>
      {/* Credential mode toggle */}
      <FormItem>
        <FormLabel>{t('accounts.form.credentialMode')}</FormLabel>
        <Tabs
          value={credentialMode}
          onValueChange={(value) =>
            form.setValue('credentialMode', value as CredentialMode, {
              shouldDirty: true,
            })
          }
        >
          <TabsList>
            <TabsTrigger value='session'>
              {t('accounts.form.modeSession')}
            </TabsTrigger>
            <TabsTrigger value='apikey'>
              {t('accounts.form.modeApiKey')}
            </TabsTrigger>
            {showPasswordMode && (
              <TabsTrigger value='password'>
                {t('accounts.form.modePassword')}
              </TabsTrigger>
            )}
          </TabsList>
        </Tabs>
      </FormItem>

      {/* Connection name (optional) — hidden in password mode, where
                username is the site login name collected by PasswordFields */}
      {credentialMode !== 'password' && (
        <FormField
          control={form.control}
          name='username'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('accounts.form.connectionName')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('accounts.form.connectionNamePlaceholder')}
                  {...field}
                  value={field.value ?? ''}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}

      {credentialMode === 'session' && (
        <SessionFields
          form={form}
          verification={verification}
          onVerify={handleVerify}
        />
      )}
      {credentialMode === 'apikey' && (
        <ApiKeyFields
          form={form}
          verification={verification}
          onVerify={handleVerify}
        />
      )}
      {credentialMode === 'password' && <PasswordFields form={form} />}
    </>
  )
}

function VerifyCredentialFeedback({
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

interface SessionFieldsProps {
  form: UseFormReturn<AccountFormValues>
  verification: VerificationState
  onVerify: () => void
}

function SessionFields({ form, verification, onVerify }: SessionFieldsProps) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='accessToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.accessToken')}</FormLabel>
            <FormControl>
              <Textarea
                rows={4}
                placeholder={t('accounts.formSession.accessTokenPlaceholder')}
                className='font-mono text-xs'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {t('accounts.formSession.accessTokenHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <VerifyCredentialFeedback
        verification={verification}
        onVerify={onVerify}
      />

      <FormField
        control={form.control}
        name='platformUserId'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.platformUserId')}</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder={t(
                  'accounts.formSession.platformUserIdPlaceholder'
                )}
                value={field.value ?? ''}
                onChange={(event) =>
                  field.onChange(
                    event.target.value === ''
                      ? undefined
                      : Number(event.target.value)
                  )
                }
                onBlur={field.onBlur}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='refreshToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.refreshToken')}</FormLabel>
            <FormControl>
              <Input
                className='font-mono text-xs'
                placeholder={t('accounts.formSession.refreshTokenPlaceholder')}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='tokenExpiresAt'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.tokenExpiresAt')}</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder={t(
                  'accounts.formSession.tokenExpiresAtPlaceholder'
                )}
                value={field.value ?? ''}
                onChange={(event) =>
                  field.onChange(
                    event.target.value === ''
                      ? undefined
                      : Number(event.target.value)
                  )
                }
                onBlur={field.onBlur}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}

// ---------------------------------------------------------------------------
// Password-mode fields — username+password sign-in, bound via the separate
// POST /api/accounts/login endpoint (the backend fetches the session token
// and stores autoRelogin config on the account).
// ---------------------------------------------------------------------------

function PasswordFields({ form }: { form: UseFormReturn<AccountFormValues> }) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='username'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formPassword.username')}</FormLabel>
            <FormControl>
              <Input
                autoComplete='username'
                placeholder={t('accounts.formPassword.usernamePlaceholder')}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='password'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formPassword.password')}</FormLabel>
            <FormControl>
              <Input
                type='password'
                autoComplete='new-password'
                placeholder={t('accounts.formPassword.passwordPlaceholder')}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>{t('accounts.formPassword.hint')}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}

// ---------------------------------------------------------------------------
// API-Key-mode fields
// ---------------------------------------------------------------------------

function ApiKeyFields({ form, verification, onVerify }: SessionFieldsProps) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='apiToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formApiKey.apiKey')}</FormLabel>
            <FormControl>
              <Textarea
                rows={3}
                placeholder={t('accounts.formApiKey.apiKeyPlaceholder')}
                className='font-mono text-xs'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <VerifyCredentialFeedback
        verification={verification}
        onVerify={onVerify}
      />

      <FormField
        control={form.control}
        name='skipModelFetch'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <FormLabel>{t('accounts.formApiKey.skipModelFetch')}</FormLabel>
              <FormDescription>
                {t('accounts.formApiKey.skipModelFetchHint')}
              </FormDescription>
            </div>
            <FormControl>
              <Switch
                checked={field.value}
                onCheckedChange={(checked) => {
                  field.onChange(checked)
                  field.onBlur() // Remember explicit choices even when toggled back to the default.
                }}
              />
            </FormControl>
          </FormItem>
        )}
      />
    </>
  )
}
