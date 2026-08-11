/* eslint-disable no-nested-ternary -- display-name fallback uses chained ternary */
// metapi-go features/accounts/components — account detail side sheet.
// Shows account metadata + the embedded TokensPanel (tokens sub-module).
// Mirrors the legacy metapi design where tokens live inside account detail,
// not on a standalone page. A footer CTA continues the site → account → route
// guided chain ("下一步：配置路由").

import { ExternalLink, RefreshCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

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
  const { t } = useTranslation()
  const refreshMutation = useRefreshAccount()

  if (!account) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' className='sm:max-w-md' />
      </Sheet>
    )
  }

  const handleConfigureRoutes = () => {
    const params = new URLSearchParams()
    params.set('accountId', String(account.id))
    if (account.siteId) params.set('siteId', String(account.siteId))
    window.location.assign(`/token-routes?${params.toString()}`)
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
    (account.credentialMode === 'apikey' ? t('accounts.detail.fallbackApiKey') : t('accounts.detail.fallbackUnnamed'))

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='flex w-full flex-col gap-0 sm:max-w-md'>
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2'>
            <span className='truncate'>{displayName}</span>
            <Badge variant={account.credentialMode === 'session' ? 'default' : 'secondary'}>
              {account.credentialMode === 'session' ? 'Session' : 'API Key'}
            </Badge>
          </SheetTitle>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          {/* Overview grid */}
          <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField label={t('accounts.detail.site')}>
              {site ? site.name || site.url || `#${site.id}` : `#${account.siteId}`}
            </DetailField>
            <DetailField label={t('accounts.detail.platform')}>
              {site?.platform || '—'}
            </DetailField>
            <DetailField label={t('accounts.detail.balance')}>
              ${formatNumber(account.balance)}
            </DetailField>
            <DetailField label={t('accounts.detail.used')}>
              ${formatNumber(account.balanceUsed)}
            </DetailField>
            <DetailField label={t('accounts.detail.todayReward')}>
              +${formatNumber(account.todayReward)}
            </DetailField>
            <DetailField label={t('accounts.detail.todaySpend')}>
              ${formatNumber(account.todaySpend)}
            </DetailField>
            <DetailField label={t('accounts.detail.checkin')}>
              {account.capabilities?.canCheckin
                ? account.checkinEnabled
                  ? t('accounts.detail.checkinOn')
                  : t('accounts.detail.checkinOff')
                : t('accounts.detail.checkinUnsupported')}
            </DetailField>
            <DetailField label={t('accounts.detail.lastBalanceRefresh')}>
              {account.lastBalanceRefresh || '—'}
            </DetailField>
          </dl>

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
              <RefreshCw className={refreshMutation.isPending ? 'animate-spin' : undefined} />
              {t('accounts.detail.refreshBalance')}
            </Button>
          </div>

          <Separator />

          {/* Embedded tokens sub-module */}
          <TokensPanel accountId={account.id} />
        </div>

        <SheetFooter>
          <Button onClick={handleConfigureRoutes} variant='default'>
            <ExternalLink />
            {t('accounts.detail.configureRoutes')}
          </Button>
        </SheetFooter>
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
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className='flex flex-col'>
      <dt className='text-[11px] text-muted-foreground'>{label}</dt>
      <dd className='truncate'>{children}</dd>
    </div>
  )
}

function formatNumber(value: number | undefined | null): string {
  if (value === undefined || value === null) return '0.00'
  return value.toFixed(2)
}
