// metapi-go features/accounts/components — inline row disclosure panel.
// Lazily fetches the expanded account's tokens + renders check-in/balance
// snapshot alongside one-click copy (via master-token reauth), route config,
// and full-sheet inspection so operators resolve routine checks without
// repeatedly opening the side drawer.

import { useNavigate } from '@tanstack/react-router'
import { ExternalLink, RefreshCw, Route } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useReauth } from '@/components/common/reauth-provider'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SecretField } from '@/components/ui/secret-field'
import { Spinner } from '@/components/ui/spinner'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import {
  EM_DASH,
  formatAbsoluteDateTime,
  formatCurrency,
  formatRelativeTime,
} from '@/lib/format'
import {
  extractApiErrorBody,
  isReauthRequired,
  resolveResponseMessage,
} from '@/lib/http-client'
import { toast } from '@/lib/toast'

import { useAccountTokens } from '../tokens/api'
import type { Account, AccountToken } from '../types'

interface AccountInlineDetailsProps {
  account: Account
  onViewDetail: (account: Account) => void
  onRefresh: (account: Account) => void
}

function InlineTokenItem({
  token,
  onConfigureRoutes,
}: {
  token: AccountToken
  onConfigureRoutes?: () => void
}) {
  const { t } = useTranslation()
  const { requestReauth } = useReauth()
  const isMaskedPending = token.valueStatus === 'masked_pending'
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
    <li className='flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2'>
      <div className='flex min-w-0 flex-1 flex-col gap-0.5'>
        <div className='flex flex-wrap items-center gap-1.5'>
          <span className='truncate text-sm font-medium'>
            {token.name || t('accounts.tokens.unnamed')}
          </span>
          {token.isDefault && (
            <Badge variant='default' className='text-3xs'>
              {t('accounts.tokens.default')}
            </Badge>
          )}
          {isMaskedPending && (
            <Badge
              variant='warning'
              className='text-3xs'
              title={t('accounts.tokens.pendingCompleteHint')}
            >
              {t('accounts.tokens.pendingComplete')}
            </Badge>
          )}
          {token.enabled === false && (
            <Badge variant='secondary' className='text-3xs'>
              {t('accounts.columns.healthDisabled')}
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
      {onConfigureRoutes && (
        <Button
          variant='outline'
          size='xs'
          onClick={onConfigureRoutes}
          disabled={!token.enabled || isMaskedPending}
          title={t('accounts.tokens.configureRoutes')}
        >
          <Route className='size-3.5' aria-hidden='true' />
          {t('accounts.tokens.configureRoutes')}
        </Button>
      )}
    </li>
  )
}

export function AccountInlineDetails({
  account,
  onViewDetail,
  onRefresh,
}: AccountInlineDetailsProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const locale = toBcp47(i18n.language || 'en')
  const {
    data: tokens = [],
    isLoading,
    error,
    refetch,
    isFetching,
  } = useAccountTokens(account.id)

  function navigateToTokenRoutes(tokenId?: number) {
    void navigate({
      to: '/token-routes',
      search: {
        accountId: account.id,
        ...(tokenId ? { tokenId } : {}),
        ...(account.siteId ? { siteId: account.siteId } : {}),
      },
    })
  }

  const healthReason = account.runtimeHealth?.reason?.trim() || null

  return (
    <div
      data-slot='row-disclosure-well'
      className='bg-card/60 flex flex-col gap-3 rounded-lg border p-3'
    >
      {/* Compact balance / check-in summary + primary actions */}
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <dl className='grid flex-1 grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-4'>
          <div className='min-w-0'>
            <dt className='text-muted-foreground'>
              {t('accounts.detail.todayReward')}
            </dt>
            <dd className='mt-0.5 text-sm font-medium tabular-nums'>
              {account.todayReward != null && account.todayReward > 0 ? (
                <span className='text-success-soft-fg'>
                  +{formatCurrency(account.todayReward)}
                </span>
              ) : (
                <span className='text-muted-foreground'>{EM_DASH}</span>
              )}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground'>
              {t('accounts.detail.todaySpend')}
            </dt>
            <dd className='mt-0.5 text-sm font-medium tabular-nums'>
              {account.todaySpend != null && account.todaySpend > 0 ? (
                <span>-{formatCurrency(account.todaySpend)}</span>
              ) : (
                <span className='text-muted-foreground'>
                  {formatCurrency(account.todaySpend)}
                </span>
              )}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground'>
              {t('accounts.detail.lastCheckin')}
            </dt>
            <dd
              className='mt-0.5 truncate text-sm tabular-nums'
              title={
                account.lastCheckinAt
                  ? formatAbsoluteDateTime(account.lastCheckinAt, locale) ||
                    undefined
                  : undefined
              }
            >
              {account.lastCheckinAt
                ? formatRelativeTime(account.lastCheckinAt, locale)
                : EM_DASH}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground'>
              {t('accounts.detail.lastBalanceRefresh')}
            </dt>
            <dd
              className='mt-0.5 truncate text-sm tabular-nums'
              title={
                account.lastBalanceRefresh
                  ? formatAbsoluteDateTime(
                      account.lastBalanceRefresh,
                      locale
                    ) || undefined
                  : undefined
              }
            >
              {account.lastBalanceRefresh
                ? formatRelativeTime(account.lastBalanceRefresh, locale)
                : EM_DASH}
            </dd>
          </div>
        </dl>

        <div className='flex flex-wrap items-center gap-1.5'>
          <Button
            variant='outline'
            size='xs'
            onClick={() => onRefresh(account)}
          >
            <RefreshCw className='size-3.5' aria-hidden='true' />
            {t('accounts.detail.refreshBalance')}
          </Button>
          <Button
            variant='outline'
            size='xs'
            onClick={() => navigateToTokenRoutes()}
          >
            <Route className='size-3.5' aria-hidden='true' />
            {t('accounts.created.action')}
          </Button>
          <Button
            variant='ghost'
            size='xs'
            onClick={() => onViewDetail(account)}
          >
            <ExternalLink className='size-3.5' aria-hidden='true' />
            {t('accounts.columns.viewDetails')}
          </Button>
        </div>
      </div>

      {healthReason && (
        <div className='bg-muted/40 rounded-md border px-2.5 py-2 text-xs'>
          <span className='text-muted-foreground font-medium'>
            {t('accounts.detail.healthReason')}:{' '}
          </span>
          <span className='break-words'>{healthReason}</span>
        </div>
      )}

      {/* Lazily loaded API tokens list */}
      <div className='flex flex-col gap-2 border-t pt-2.5'>
        <div className='flex items-center justify-between gap-2'>
          <span className='text-muted-foreground text-xs font-medium'>
            {t('accounts.tokens.title')}
          </span>
          {!isLoading && !error && (
            <span className='text-muted-foreground text-xs tabular-nums'>
              {tokens.length}
            </span>
          )}
        </div>

        <QueryErrorBanner
          error={error}
          messageKey='accounts.tokens.loadError'
          onRetry={() => void refetch()}
          isRetrying={isFetching}
        />

        {isLoading && (
          <div className='text-muted-foreground flex items-center gap-2 py-2 text-xs'>
            <Spinner className='size-3.5' />
            <span>{t('accounts.tokens.loading')}</span>
          </div>
        )}
        {!isLoading && !error && tokens.length === 0 && (
          <p className='text-muted-foreground py-1.5 text-xs'>
            {t('accounts.tokens.empty')}
          </p>
        )}
        {!isLoading && tokens.length > 0 && (
          <ul className='bg-background divide-y rounded-md border'>
            {tokens.map((token) => (
              <InlineTokenItem
                key={token.id}
                token={token}
                onConfigureRoutes={
                  !error ? () => navigateToTokenRoutes(token.id) : undefined
                }
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
