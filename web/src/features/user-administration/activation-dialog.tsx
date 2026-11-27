import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useReauth } from '@/components/common/reauth-provider'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import { Textarea } from '@/components/ui/textarea'
import { toBcp47 } from '@/i18n/languages'
import { gatewayUsersApi, type GatewayUser } from '@/lib/api/gateway-users'
import { copyText } from '@/lib/clipboard'
import { isReauthRequired } from '@/lib/http-client'
import { useAuthStore } from '@/stores/auth-store'

import { WriteError } from './feedback'

export function ActivationDialog({
  user,
  close,
}: {
  user: GatewayUser
  close: () => void
}) {
  const { t, i18n } = useTranslation()
  const { requestReauth } = useReauth()
  const [secret, setSecret] = useState<{
    code: string
    expiresAt: string
  } | null>(null)
  const [pending, setPending] = useState(false)
  const [failed, setFailed] = useState(false)
  const [copyStatus, setCopyStatus] = useState('')
  const alive = useRef(true)
  useEffect(() => {
    alive.current = true
    const unsubscribe = useAuthStore.subscribe((state, previous) => {
      if (
        previous.auth.sessionExpiresAt !== null &&
        state.auth.sessionExpiresAt === null
      ) {
        alive.current = false
        setSecret(null)
        close()
      }
    })
    return () => {
      alive.current = false
      unsubscribe()
    }
  }, [close])
  // Close marks any pending replay obsolete; no secret survives this surface.
  const dismiss = () => {
    alive.current = false
    setSecret(null)
    close()
  }
  const issue = async (token?: string) => {
    if (!alive.current) return
    setPending(true)
    setFailed(false)
    setSecret(null)
    try {
      const result = await gatewayUsersApi.activate(user.id, token)
      if (alive.current) setSecret(result)
    } catch (error) {
      if (isReauthRequired(error)) {
        if (token) throw error
        requestReauth({
          description: t('userAdmin.activationReauth'),
          replay: (confirmToken) => issue(confirmToken),
        })
      } else if (alive.current) setFailed(true)
    } finally {
      if (alive.current) setPending(false)
    }
  }
  const activationPackage = secret
    ? t('userAdmin.activationPackage.text', {
        entry: new URL('/user', window.location.origin).href,
        username: user.username,
        code: secret.code,
        expires: secret.expiresAt,
      })
    : ''
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) dismiss()
      }}
    >
      <DialogContent className='max-h-[90svh] overflow-y-auto sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle className='wrap-anywhere'>
            {t('userAdmin.activationTitle', {
              user: user.displayName || user.username,
            })}
          </DialogTitle>
          <DialogDescription>{t('userAdmin.activationHint')}</DialogDescription>
        </DialogHeader>
        {secret ? (
          <>
            <Input
              aria-label={t('userAdmin.activationCode')}
              value={secret.code}
              readOnly
              className='font-mono'
            />
            <p className='text-muted-foreground text-xs tabular-nums'>
              {t('userAdmin.expires', {
                time: new Date(secret.expiresAt).toLocaleString(
                  toBcp47(i18n.language)
                ),
              })}
            </p>
            <Textarea
              aria-label={t('userAdmin.activationPackage.label')}
              value={activationPackage}
              readOnly
              className='min-h-48'
            />
            <p className='text-muted-foreground text-xs'>
              {t('userAdmin.activationPackage.private')}
            </p>
            <Button
              onClick={() => {
                void copyText(activationPackage).then((ok) => {
                  if (alive.current)
                    setCopyStatus(
                      t(
                        ok
                          ? 'userAdmin.copied'
                          : 'userAdmin.activationPackage.copyFailed'
                      )
                    )
                })
              }}
            >
              {t('userAdmin.activationPackage.copy')}
            </Button>
            <Button
              variant='outline'
              onClick={() => {
                void copyText(secret.code).then((ok) => {
                  if (!alive.current) return
                  setCopyStatus(
                    t(ok ? 'userAdmin.copied' : 'userAdmin.copyFailed')
                  )
                })
              }}
            >
              {t('userAdmin.copyCode')}
            </Button>
            <p role='status'>{copyStatus}</p>
          </>
        ) : (
          <>
            <WriteError error={failed ? new Error('activation') : null} />
            <Button
              onClick={() => void issue()}
              disabled={pending || !user.enabled}
            >
              {t(pending ? 'userAdmin.saving' : 'userAdmin.issueCode')}
            </Button>
            {!user.enabled && (
              <Notice tone='warning'>
                {t('userAdmin.enableBeforeActivation')}
              </Notice>
            )}
          </>
        )}
        <Button variant='outline' onClick={dismiss} disabled={pending}>
          {t('userAdmin.close')}
        </Button>
      </DialogContent>
    </Dialog>
  )
}
