/* eslint-disable no-nested-ternary -- display-name fallback uses chained ternary */
// metapi-go features/accounts/components — account detail side sheet.
// Shows account metadata + the embedded TokensPanel (tokens sub-module).
// Mirrors the legacy metapi design where tokens live inside account detail,
// not on a standalone page. A footer CTA continues the site → account → route
// guided chain ("下一步：配置路由").

import { useNavigate } from '@tanstack/react-router'
import { ExternalLink, RefreshCw } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { toBcp47 } from '@/i18n/languages'
import {
  EM_DASH,
  formatAbsoluteDateTime,
  formatPrice,
  formatRelativeTime,
  formatUsd,
} from '@/lib/format'

import { useRefreshAccount } from '../api'
import { TokensPanel } from '../tokens/components/tokens-panel'
import type { Account } from '../types'

interface AccountDetailSheetProps {
  account: Account | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AccountDetailSheet({
  account,
  open,
  onOpenChange,
}: AccountDetailSheetProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const refreshMutation = useRefreshAccount()
  const locale = toBcp47(i18n.language || 'en')

  // Dirty state of the embedded token form (reported by TokensPanel). Both
  // the sheet close and the "configure routes" CTA confirm before discarding.
  const [tokensFormDirty, setTokensFormDirty] = useState(false)
  const [confirmNavigateOpen, setConfirmNavigateOpen] = useState(false)

  const { handleOpenChange: guardedOpenChange, guard: dirtyCloseGuard } =
    useDirtyDialogClose({
      enabled: tokensFormDirty,
      onDiscard: () => setTokensFormDirty(false),
      onOpenChange,
    })

  if (!account) {
    return (
      <Sheet open={open} onOpenChange={guardedOpenChange}>
        <SheetContent side='right' className='sm:max-w-md' />
      </Sheet>
    )
  }

  function navigateToTokenRoutes() {
    if (!account) return
    void navigate({
      to: '/token-routes',
      search: {
        accountId: account.id,
        ...(account.siteId ? { siteId: account.siteId } : {}),
      },
    })
    onOpenChange(false)
  }

  const handleConfigureRoutes = () => {
    if (tokensFormDirty) {
      setConfirmNavigateOpen(true)
      return
    }
    navigateToTokenRoutes()
  }

  const handleRefresh = async () => {
    try {
      await refreshMutation.mutateAsync(account.id)
    } catch {
      // http-client toasted
    }
  }

  const site = account.site
  const displayName =
    account.username?.trim() ||
    (account.credentialMode === 'apikey'
      ? t('accounts.detail.fallbackApiKey')
      : t('accounts.detail.fallbackUnnamed'))
  // Same field the accounts list health tooltip surfaces; rendered here as a
  // full block so long reasons are not squeezed into a grid cell.
  const healthReason = account.runtimeHealth?.reason?.trim() || null

  return (
    <Sheet open={open} onOpenChange={guardedOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-md'
      >
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2'>
            <span className='truncate'>{displayName}</span>
            <Badge
              variant={
                account.credentialMode === 'session' ? 'default' : 'secondary'
              }
            >
              {account.credentialMode === 'session' ? 'Session' : 'API Key'}
            </Badge>
          </SheetTitle>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          {/* Overview grid */}
          <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField
              label={t('accounts.detail.site')}
              title={
                site
                  ? site.name || site.url || `#${site.id}`
                  : `#${account.siteId}`
              }
            >
              {site
                ? site.name || site.url || `#${site.id}`
                : `#${account.siteId}`}
            </DetailField>
            <DetailField
              label={t('accounts.detail.platform')}
              title={site?.platform || undefined}
            >
              {site?.platform || '—'}
            </DetailField>
            <DetailField label={t('accounts.detail.balance')}>
              {formatAmount(account.balance)}
            </DetailField>
            <DetailField label={t('accounts.detail.used')}>
              {formatAmount(account.balanceUsed)}
            </DetailField>
            <DetailField label={t('accounts.detail.todayReward')}>
              {formatAmount(account.todayReward, '+$')}
            </DetailField>
            <DetailField label={t('accounts.detail.todaySpend')}>
              {formatAmount(account.todaySpend)}
            </DetailField>
            <DetailField label={t('accounts.detail.checkin')}>
              {account.capabilities?.canCheckin
                ? account.checkinEnabled
                  ? t('accounts.detail.checkinOn')
                  : t('accounts.detail.checkinOff')
                : t('accounts.detail.checkinUnsupported')}
            </DetailField>
            <DetailField
              label={t('accounts.detail.lastBalanceRefresh')}
              title={account.lastBalanceRefresh || undefined}
            >
              {account.lastBalanceRefresh || '—'}
            </DetailField>
            <DetailField label={t('accounts.detail.quota')}>
              {(account.quota ?? 0) > 0 ? (
                <span className='tabular-nums'>{formatUsd(account.quota)}</span>
              ) : (
                '—'
              )}
            </DetailField>
            <DetailField label={t('accounts.detail.unitCost')}>
              {account.unitCost === null || account.unitCost === undefined ? (
                '—'
              ) : (
                <span className='tabular-nums'>
                  {formatPrice(account.unitCost)}
                </span>
              )}
            </DetailField>
            <DetailField label={t('accounts.detail.lastCheckin')}>
              {account.lastCheckinAt ? (
                <span
                  title={
                    formatAbsoluteDateTime(account.lastCheckinAt, locale) ||
                    undefined
                  }
                >
                  {formatRelativeTime(account.lastCheckinAt, locale)}
                </span>
              ) : (
                '—'
              )}
            </DetailField>
          </dl>

          {healthReason && (
            <div className='bg-muted/40 rounded-lg border p-2 text-xs'>
              <div className='text-muted-foreground text-[11px]'>
                {t('accounts.detail.healthReason')}
              </div>
              <p className='break-words'>{healthReason}</p>
            </div>
          )}

          {account.tags && account.tags.length > 0 && (
            <div className='flex flex-wrap gap-1'>
              {account.tags.map((tag) => (
                <Badge key={tag} variant='outline' className='text-[10px]'>
                  {tag}
                </Badge>
              ))}
            </div>
          )}

          <div className='flex justify-end'>
            <Button
              variant='outline'
              size='sm'
              onClick={handleRefresh}
              disabled={refreshMutation.isPending}
            >
              <RefreshCw
                className={
                  refreshMutation.isPending ? 'animate-spin' : undefined
                }
              />
              {t('accounts.detail.refreshBalance')}
            </Button>
          </div>

          <Separator />

          {/* Embedded tokens sub-module */}
          <TokensPanel
            accountId={account.id}
            onFormDirtyChange={setTokensFormDirty}
          />
        </div>

        <SheetFooter>
          <Button onClick={handleConfigureRoutes} variant='default'>
            <ExternalLink />
            {t('accounts.detail.configureRoutes')}
          </Button>
        </SheetFooter>

        {dirtyCloseGuard}

        <ConfirmDialog
          open={confirmNavigateOpen}
          title={t('settings.common.unsavedTitle')}
          description={t('settings.common.unsavedDescription')}
          confirmLabel={t('settings.common.discardChanges')}
          cancelLabel={t('settings.common.keepEditing')}
          destructive
          onConfirm={() => {
            setConfirmNavigateOpen(false)
            setTokensFormDirty(false)
            navigateToTokenRoutes()
          }}
          onCancel={() => setConfirmNavigateOpen(false)}
        />
      </SheetContent>
    </Sheet>
  )
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function DetailField({
  label,
  children,
  title,
}: {
  label: string
  children: ReactNode
  title?: string
}) {
  return (
    <div className='flex flex-col'>
      <dt className='text-muted-foreground text-[11px]'>{label}</dt>
      <dd className='truncate' title={title}>
        {children}
      </dd>
    </div>
  )
}

// A null amount means "never refreshed", not zero — rendering $0.00 would
// misreport a missing value. Show a bare em dash (no currency prefix),
// matching lib/format's null-display convention.
function formatAmount(value: number | undefined | null, prefix = '$'): string {
  if (value === undefined || value === null) return EM_DASH
  return `${prefix}${value.toFixed(2)}`
}
