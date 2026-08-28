// metapi-go/components/common — master-token re-confirmation dialog (#1034).
//
// Sensitive operations (backup export, downstream key export, token
// rotation) require the operator to re-present the master token, even with
// a live session cookie. The caller opens this dialog, and on submit replays
// the request with X-Admin-Confirm-Token. The entered token is passed up
// once and never stored.

import { useEffect, useState } from 'react'
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

interface ReauthDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Called with the entered master token; caller replays the request. */
  onSubmit: (confirmToken: string) => void
  /** Disable the submit button while the replayed request is in flight. */
  isPending?: boolean
  /** Context sentence shown above the input (what is about to happen). */
  description?: string
  /** Shown when the previous attempt was rejected (wrong token). */
  errorMessage?: string | null
}

export function ReauthDialog({
  open,
  onOpenChange,
  onSubmit,
  isPending = false,
  description,
  errorMessage = null,
}: ReauthDialogProps) {
  const { t } = useTranslation()
  const [token, setToken] = useState('')

  // Fresh prompt every time the dialog opens.
  useEffect(() => {
    if (open) setToken('')
  }, [open])

  function submit() {
    const trimmed = token.trim()
    if (!trimmed || isPending) return
    onSubmit(trimmed)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('reauth.title')}</DialogTitle>
          <DialogDescription>
            {description ?? t('reauth.description')}
          </DialogDescription>
        </DialogHeader>
        <form
          className='space-y-3'
          onSubmit={(event) => {
            event.preventDefault()
            submit()
          }}
        >
          <label className='text-sm font-medium' htmlFor='reauth-token'>
            {t('reauth.tokenLabel')}
          </label>
          <Input
            id='reauth-token'
            type='password'
            autoComplete='current-password'
            autoFocus
            value={token}
            disabled={isPending}
            onChange={(event) => setToken(event.target.value)}
            placeholder={t('reauth.tokenPlaceholder')}
          />
          {errorMessage ? (
            <p className='text-destructive text-sm'>{errorMessage}</p>
          ) : null}
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={isPending}
              onClick={() => onOpenChange(false)}
            >
              {t('reauth.cancel')}
            </Button>
            <Button type='submit' disabled={isPending || !token.trim()}>
              {isPending ? t('common.loading') : t('reauth.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
