import { CheckCircle2, Pencil, Route, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useReauth } from '@/components/common/reauth-provider'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SecretField } from '@/components/ui/secret-field'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import {
  extractApiErrorBody,
  isReauthRequired,
  resolveResponseMessage,
} from '@/lib/http-client'
import { toast } from '@/lib/toast'

import type { AccountToken } from '../../types'

interface TokenRowProps {
  token: AccountToken
  onConfigureRoutes?: () => void
  onEdit: () => void
  onDelete: () => void
  onSetDefault: () => void
  onToggleEnabled: (enabled: boolean) => void
  isToggling: boolean
}

export function AccountTokenRow({
  token,
  onConfigureRoutes,
  onEdit,
  onDelete,
  onSetDefault,
  onToggleEnabled,
  isToggling,
}: TokenRowProps) {
  const { t } = useTranslation()
  const { requestReauth } = useReauth()
  const isMaskedPending = token.valueStatus === 'masked_pending'
  // The list response only carries the masked form. Plaintext is fetched
  // lazily on an explicit eye/copy click, behind master-token confirmation,
  // and kept only for this row's lifetime (never persisted or cached).
  const [revealedValue, setRevealedValue] = useState<string | null>(null)
  const [revealPending, setRevealPending] = useState(false)
  const canReveal = !isMaskedPending

  function copyValue(value: string) {
    void copyText(value).then((ok) => {
      if (!ok) {
        toast.error(t('common.copyFailed'))
      }
    })
  }

  function requestValue(options: { onReady?: (value: string) => void } = {}) {
    requestReauth({
      description: t('reauth.operations.accountTokenReveal'),
      replay: async (confirmToken) => {
        setRevealPending(true)
        try {
          const data = await api.getAccountTokenValue(token.id, confirmToken)
          const value = typeof data?.token === 'string' ? data.token : ''
          if (!value) {
            throw new Error('Empty token value')
          }
          setRevealedValue(value)
          options.onReady?.(value)
        } catch (error) {
          // A rejected master token keeps the dialog open with its inline
          // error; every other failure is surfaced here and closes it.
          if (!isReauthRequired(error)) {
            toast.error(
              resolveResponseMessage(extractApiErrorBody(error)) ??
                t('accounts.tokens.toast.revealFailed')
            )
          }
          throw error
        } finally {
          setRevealPending(false)
        }
      },
    })
  }

  return (
    <li className='flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2'>
      <div className='flex min-w-0 flex-1 flex-col gap-0.5'>
        <div className='flex flex-wrap items-center gap-1.5'>
          <span
            className='line-clamp-2 text-sm font-medium break-words'
            title={token.name || t('accounts.tokens.unnamed')}
          >
            {token.name || t('accounts.tokens.unnamed')}
          </span>
          {token.isDefault && (
            <Badge variant='default' className='text-3xs'>
              {t('accounts.tokens.default')}
            </Badge>
          )}
          {isMaskedPending && (
            // "Pending" alone reads as "wait", which is what an operator did
            // while the token stayed unusable: Metapi only ever received the
            // upstream's masked display value, and the fix is the sync action
            // that already sits on this account, not time.
            <Badge
              variant='warning'
              className='text-3xs'
              title={t('accounts.tokens.pendingCompleteHint')}
            >
              {t('accounts.tokens.pendingComplete')}
            </Badge>
          )}
        </div>
        <SecretField
          value={revealedValue}
          masked={token.tokenMasked}
          busy={revealPending}
          onRevealRequest={
            canReveal && !revealedValue ? () => requestValue() : undefined
          }
          onCopyRequest={
            canReveal && !revealedValue
              ? () => requestValue({ onReady: copyValue })
              : undefined
          }
        />
      </div>

      <div className='ml-auto flex items-center gap-1'>
        {onConfigureRoutes && (
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={onConfigureRoutes}
            disabled={!token.enabled || isMaskedPending}
            title={t('accounts.tokens.configureRoutes')}
            aria-label={t('accounts.tokens.configureRoutes')}
          >
            <Route />
          </Button>
        )}
        {!token.isDefault && (
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={onSetDefault}
            title={t('accounts.tokens.setAsDefault')}
            aria-label={t('accounts.tokens.setAsDefault')}
          >
            <CheckCircle2 />
          </Button>
        )}
        <Switch
          checked={token.enabled ?? false}
          onCheckedChange={onToggleEnabled}
          disabled={isToggling}
          aria-label={t('accounts.tokens.toggleEnabled')}
        />
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={onEdit}
          title={t('accounts.tokens.edit')}
          aria-label={t('accounts.tokens.edit')}
        >
          <Pencil />
        </Button>
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={onDelete}
          className='text-muted-foreground hover:text-destructive'
          title={t('accounts.tokens.delete')}
          aria-label={t('accounts.tokens.delete')}
        >
          <Trash2 />
        </Button>
      </div>
    </li>
  )
}
