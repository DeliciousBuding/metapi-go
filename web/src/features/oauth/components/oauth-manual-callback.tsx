import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { toast } from '@/lib/toast'

import { useSubmitOAuthManualCallback } from '../api'

export function useManualCallback(
  pendingState: string | null,
  kick: () => void
) {
  const { t } = useTranslation()
  const [callbackUrl, setCallbackUrl] = useState('')
  // Inline validation error for the manual-callback input (not a valid
  // http(s) URL). Cleared as soon as the user edits the field.
  const [callbackInvalid, setCallbackInvalid] = useState(false)
  const submitManualCallback = useSubmitOAuthManualCallback()
  function isValidCallbackUrl(value: string): boolean {
    try {
      const parsed = new URL(value)
      return parsed.protocol === 'http:' || parsed.protocol === 'https:'
    } catch {
      return false
    }
  }

  async function handleManualCallbackSubmit(
    event: React.FormEvent<HTMLFormElement>
  ) {
    event.preventDefault()
    if (!pendingState) return
    const url = callbackUrl.trim()
    if (!url) return
    if (!isValidCallbackUrl(url)) {
      setCallbackInvalid(true)
      return
    }
    setCallbackInvalid(false)
    try {
      await submitManualCallback.mutateAsync({
        state: pendingState,
        callbackUrl: url,
      })
      setCallbackUrl('')
      toast.success(t('oauth.session.callbackSubmitted'))
      // The backend settles (or advances) the session on manual callback —
      // re-check immediately with a fresh budget instead of waiting out the
      // interval.
      kick()
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      toast.error(message || t('common.requestFailed'))
    }
  }
  const reset = useCallback(() => {
    setCallbackUrl('')
    setCallbackInvalid(false)
  }, [])
  return {
    callbackUrl,
    callbackInvalid,
    reset,
    clearError: () => setCallbackInvalid(false),
    change: (url: string) => {
      setCallbackUrl(url)
      setCallbackInvalid(false)
    },
    handleManualCallbackSubmit,
    isSubmittingCallback: submitManualCallback.isPending,
  }
}
export function ManualCallbackForm({
  callback,
}: {
  callback: ReturnType<typeof useManualCallback>
}) {
  const { t } = useTranslation()
  const {
    callbackUrl,
    callbackInvalid,
    isSubmittingCallback,
    handleManualCallbackSubmit,
  } = callback
  return (
    <form onSubmit={handleManualCallbackSubmit} className='grid gap-2'>
      <label
        className='text-sm font-medium'
        htmlFor='oauth-manual-callback-url'
      >
        {t('oauth.session.callbackLabel')}
      </label>
      <Input
        id='oauth-manual-callback-url'
        value={callbackUrl}
        onChange={(event) => callback.change(event.target.value)}
        placeholder={t('oauth.session.callbackPlaceholder')}
        // Not a user credential: a pasted OAuth callback URL.
        // 'off' is conforming here (WHATWG only forbids it on
        // login/password fields) and keeps managers from
        // corrupting the pasted value (#1029 batch A evaluation).
        autoComplete='off'
        spellCheck={false}
        aria-invalid={callbackInvalid || undefined}
      />
      {callbackInvalid && (
        <p className='text-destructive text-xs'>
          {t('oauth.session.callbackInvalidUrl')}
        </p>
      )}
      <Button
        type='submit'
        disabled={!callbackUrl.trim() || isSubmittingCallback}
      >
        {isSubmittingCallback ? <Spinner /> : null}
        {t('oauth.session.callbackSubmit')}
      </Button>
    </form>
  )
}
