import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import { type PersonalKey, userPortalApi } from '../api'
import { CreatedKey } from './portal-created-key'

export function RotateKeyDialog({
  item,
  userId,
  onClose,
}: {
  item: PersonalKey
  userId: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [secret, setSecret] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const [failed, setFailed] = useState(false)
  const submitting = useRef(false)
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  async function rotate() {
    if (submitting.current || secret !== null) return
    submitting.current = true
    setPending(true)
    setFailed(false)
    try {
      // Keep the one-time secret in this dialog only, never in a Query or
      // mutation cache. Unmounting on navigation/logout discards the result.
      const result = await userPortalApi.rotateKey(item.id)
      if (!mounted.current) return
      setSecret(result.key)
      void client.invalidateQueries({
        queryKey: ['userPortal', userId, 'keys'],
      })
    } catch {
      if (mounted.current) setFailed(true)
    } finally {
      submitting.current = false
      if (mounted.current) setPending(false)
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !submitting.current) onClose()
      }}
    >
      <DialogContent showCloseButton={!pending}>
        <DialogHeader>
          <DialogTitle>
            {t(
              secret ? 'userPortal.rotation.ready' : 'userPortal.rotation.title'
            )}
          </DialogTitle>
          <DialogDescription>
            {t('userPortal.rotation.description', { name: item.name })}
          </DialogDescription>
        </DialogHeader>
        {secret ? (
          <>
            <Notice tone='success'>{t('userPortal.rotation.success')}</Notice>
            <CreatedKey secret={secret} onDismiss={onClose} />
          </>
        ) : (
          <>
            <Notice tone='warning'>{t('userPortal.rotation.warning')}</Notice>
            <p className='text-muted-foreground text-sm'>
              {t('userPortal.rotation.preserves')}
            </p>
            {failed && (
              <Notice tone='destructive' role='alert'>
                {t('userPortal.rotation.failed')}
              </Notice>
            )}
            <DialogFooter>
              <Button variant='outline' disabled={pending} onClick={onClose}>
                {t('common.cancel')}
              </Button>
              <Button
                variant='destructive'
                disabled={pending}
                onClick={() => void rotate()}
              >
                {pending && <Spinner aria-hidden='true' />}
                {t(
                  pending
                    ? 'userPortal.rotation.pending'
                    : 'userPortal.rotation.confirm'
                )}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
