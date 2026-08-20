// metapi-go/features/sites — site detail Sheet (side panel).
//
// Opens from the row "view details" action. Shows the site's static config
// (url / platform / status / probe / endpoints / tags / timestamps) and
// surfaces the two follow-on CTAs of the guided config chain:
//   - 管理账号 → /accounts?siteId=…&create=1 (step 2)
//   - 管理路由 → /token-routes?siteId=… (step 3)

import { useNavigate } from '@tanstack/react-router'
import {
  ArrowRight as ArrowRightIcon,
  ExternalLink as ExternalLinkIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { toBcp47 } from '@/i18n/languages'
import {
  formatAbsoluteDateTime,
  formatRelativeTime,
  formatUsd,
} from '@/lib/format'

import { resolveSiteBalanceUsd } from '../lib/site-balance'
import type { Site, SiteApiEndpoint, SiteStatus } from '../types'

type SiteDetailSheetProps = {
  site: Site | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onEdit?: (site: Site) => void
}

const STATUS_LABEL_KEY: Record<SiteStatus, string> = {
  active: 'sites.status.active',
  disabled: 'sites.status.disabled',
}

function DetailRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='grid grid-cols-3 gap-2 py-1.5 text-sm'>
      <span className='text-muted-foreground col-span-1'>{label}</span>
      <div className='col-span-2 break-words'>{children}</div>
    </div>
  )
}

export function SiteDetailSheet({
  site,
  open,
  onOpenChange,
  onEdit,
}: SiteDetailSheetProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const locale = toBcp47(i18n.language || 'en')

  if (!site) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' />
      </Sheet>
    )
  }

  const status: SiteStatus = site.status === 'disabled' ? 'disabled' : 'active'
  const endpoints = site.apiEndpoints ?? []
  const tags = site.tags ?? []
  const siteId = site.id

  function goToAccounts() {
    onOpenChange(false)
    navigate({
      to: '/accounts',
      search: { siteId, create: true },
      replace: true,
    })
  }

  function goToRoutes() {
    onOpenChange(false)
    navigate({
      to: '/token-routes',
      search: { siteId },
      replace: true,
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='sm:max-w-md'>
        <SheetHeader>
          <SheetTitle className='pr-6'>{site.name}</SheetTitle>
          <SheetDescription className='truncate'>{site.url}</SheetDescription>
        </SheetHeader>

        <div className='flex flex-col gap-4 overflow-y-auto px-4 pb-4'>
          <div className='flex items-center gap-2'>
            <Badge variant={status === 'active' ? 'success' : 'secondary'}>
              {t(STATUS_LABEL_KEY[status])}
            </Badge>
            {site.isPinned && (
              <Badge variant='outline'>{t('sites.detail.pinned')}</Badge>
            )}
            {site.platform && (
              <span className='text-muted-foreground text-xs'>
                {site.platform}
              </span>
            )}
          </div>

          <Separator />

          <SiteBalanceSection site={site} locale={locale} />

          <section>
            <DetailRow label={t('sites.detail.url')}>
              <a
                href={site.url}
                target='_blank'
                rel='noopener noreferrer'
                className='text-primary inline-flex items-center gap-1 hover:underline'
              >
                {site.url}
                <ExternalLinkIcon className='size-3' />
              </a>
            </DetailRow>
            {site.externalCheckinUrl ? (
              <DetailRow label={t('sites.detail.externalCheckinUrl')}>
                <a
                  href={site.externalCheckinUrl}
                  target='_blank'
                  rel='noopener noreferrer'
                  className='text-primary inline-flex items-center gap-1 hover:underline'
                >
                  {site.externalCheckinUrl}
                  <ExternalLinkIcon className='size-3' />
                </a>
              </DetailRow>
            ) : null}
            {site.platform ? (
              <DetailRow label={t('sites.detail.platform')}>
                {site.platform}
              </DetailRow>
            ) : null}
            <DetailRow label={t('sites.detail.globalWeight')}>
              {site.globalWeight ?? 1}
            </DetailRow>
            <DetailRow label={t('sites.detail.maxConcurrency')}>
              {site.maxConcurrency
                ? site.maxConcurrency
                : t('sites.detail.unlimited')}
            </DetailRow>
            <DetailRow label={t('sites.detail.useSystemProxy')}>
              {site.useSystemProxy
                ? t('sites.detail.yes')
                : t('sites.detail.no')}
            </DetailRow>
            {site.proxyUrl ? (
              <DetailRow label={t('sites.detail.proxyUrl')}>
                {site.proxyUrl}
              </DetailRow>
            ) : null}
          </section>

          {endpoints.length > 0 && (
            <>
              <Separator />
              <section>
                <h3 className='text-sm font-medium'>
                  {t('sites.detail.endpoints')}
                </h3>
                <ul className='mt-2 space-y-2'>
                  {endpoints.map((endpoint) => (
                    <EndpointRow
                      key={endpoint.url}
                      endpoint={endpoint}
                      locale={locale}
                    />
                  ))}
                </ul>
              </section>
            </>
          )}

          {tags.length > 0 && (
            <>
              <Separator />
              <section>
                <h3 className='text-sm font-medium'>
                  {t('sites.detail.tags')}
                </h3>
                <div className='mt-2 flex flex-wrap gap-1.5'>
                  {tags.map((tag) => (
                    <Badge key={tag} variant='outline'>
                      {tag}
                    </Badge>
                  ))}
                </div>
              </section>
            </>
          )}

          <Separator />

          <section className='flex flex-col gap-2'>
            <Button variant='outline' onClick={goToAccounts}>
              {t('sites.detail.manageAccounts')}
              <ArrowRightIcon className='ml-1 size-4' />
            </Button>
            <Button variant='outline' onClick={goToRoutes}>
              {t('sites.detail.manageRoutes')}
              <ArrowRightIcon className='ml-1 size-4' />
            </Button>
            {onEdit && (
              <Button variant='ghost' onClick={() => onEdit(site)}>
                {t('sites.detail.editSite')}
              </Button>
            )}
          </section>
        </div>
      </SheetContent>
    </Sheet>
  )
}

// ---------------------------------------------------------------------------
// Balance & subscription block
// ---------------------------------------------------------------------------

/**
 * Renders only when the backend provided at least one displayable balance /
 * subscription value — a site without this data keeps the plain registry
 * view. The balance row prefers `totalBalance` and falls back to the
 * subscription summary's remaining USD (shared with the list column via
 * `resolveSiteBalanceUsd`); the separate "Remaining" row is suppressed in
 * the fallback case so the same number is never shown twice.
 */
function SiteBalanceSection({ site, locale }: { site: Site; locale: string }) {
  const { t } = useTranslation()
  const summary = site.subscriptionSummary ?? null
  const balanceUsd = resolveSiteBalanceUsd(site)
  const planNames = summary?.planNames ?? []
  const hasSubscriptionDetail =
    planNames.length > 0 ||
    typeof summary?.totalUsedUsd === 'number' ||
    typeof summary?.totalRemainingUsd === 'number' ||
    Boolean(summary?.nextExpiresAt)
  if (balanceUsd === null && !hasSubscriptionDetail) {
    return null
  }
  const showRemainingRow =
    typeof site.totalBalance === 'number' &&
    typeof summary?.totalRemainingUsd === 'number'
  return (
    <>
      <section>
        <h3 className='text-sm font-medium'>
          {t('sites.detail.balanceTitle')}
        </h3>
        {balanceUsd !== null && (
          <DetailRow label={t('sites.detail.balance')}>
            <span className='tabular-nums'>{formatUsd(balanceUsd)}</span>
          </DetailRow>
        )}
        {planNames.length > 0 && (
          <DetailRow label={t('sites.detail.plans')}>
            <div className='flex flex-wrap gap-1'>
              {planNames.map((planName) => (
                <Badge key={planName} variant='outline'>
                  {planName}
                </Badge>
              ))}
            </div>
          </DetailRow>
        )}
        {typeof summary?.totalUsedUsd === 'number' && (
          <DetailRow label={t('sites.detail.monthlyUsage')}>
            <span className='tabular-nums'>
              {formatUsd(summary.totalUsedUsd)}
              {typeof summary.totalMonthlyLimitUsd === 'number'
                ? ` / ${formatUsd(summary.totalMonthlyLimitUsd)}`
                : ''}
            </span>
          </DetailRow>
        )}
        {showRemainingRow && (
          <DetailRow label={t('sites.detail.remaining')}>
            <span className='tabular-nums'>
              {formatUsd(summary?.totalRemainingUsd)}
            </span>
          </DetailRow>
        )}
        {summary?.nextExpiresAt && (
          <DetailRow label={t('sites.detail.nextExpires')}>
            <span
              title={
                formatAbsoluteDateTime(summary.nextExpiresAt, locale) ||
                undefined
              }
            >
              {formatRelativeTime(summary.nextExpiresAt, locale)}
            </span>
          </DetailRow>
        )}
      </section>
      <Separator />
    </>
  )
}

// ---------------------------------------------------------------------------
// API endpoint row (enabled state + live cooldown / failure signals)
// ---------------------------------------------------------------------------

function EndpointRow({
  endpoint,
  locale,
}: {
  endpoint: SiteApiEndpoint
  locale: string
}) {
  const { t } = useTranslation()
  const cooldownActive =
    Boolean(endpoint.cooldownUntil) &&
    new Date(endpoint.cooldownUntil as string) > new Date()
  const failureReason = endpoint.lastFailureReason?.trim() || null
  return (
    <li className='flex flex-col gap-1 text-sm'>
      <div className='flex items-center gap-2'>
        <Badge variant={endpoint.enabled === false ? 'secondary' : 'outline'}>
          {endpoint.enabled === false
            ? t('sites.detail.disabled')
            : t('sites.detail.enabled')}
        </Badge>
        <span className='truncate'>{endpoint.url}</span>
      </div>
      {cooldownActive && (
        <div className='flex items-center gap-1.5'>
          <Badge variant='warning'>{t('sites.detail.endpointCooldown')}</Badge>
          <span
            className='text-muted-foreground text-xs tabular-nums'
            title={
              formatAbsoluteDateTime(endpoint.cooldownUntil, locale) ||
              undefined
            }
          >
            {formatRelativeTime(endpoint.cooldownUntil, locale)}
          </span>
        </div>
      )}
      {failureReason && (
        <p className='text-destructive truncate text-xs' title={failureReason}>
          {t('sites.detail.endpointFailureReason')}: {failureReason}
        </p>
      )}
    </li>
  )
}
