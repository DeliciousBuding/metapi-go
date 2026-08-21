// metapi-go/features/oauth/components — OAuth connection detail Sheet.
//
// Opens from the row "view details" action. Until now the OAuth page was the
// only list page in the console without a detail sheet, so contract fields
// that do not fit a table row (quota `remaining` / `resetAt`, the model
// preview list, the full model-sync error, the subscription window) had
// nowhere to surface. This panel is read-only apart from two footer actions
// that reuse the page's existing mutations.
//
// No extra fetch: `useOAuthConnections` already returns the full
// `OAuthConnectionInfo` projection, so the sheet renders the row it was
// handed (mirroring the channels / models detail sheets).
//
// Viewport contract: the panel body is the scroll region (`flex-1
// overflow-y-auto`) inside the flex-column `SheetContent`, so the header and
// the opaque `SheetFooter` (`mt-auto`) stay visible while long quota/error
// content scrolls.

import {
  RefreshCw as RefreshCwIcon,
  TriangleAlert as TriangleAlertIcon,
  Unplug as UnplugIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { toBcp47 } from '@/i18n/languages'
import {
  EM_DASH,
  formatAbsoluteDateTime,
  formatInt,
  formatRelativeTime,
} from '@/lib/format'

import { resolveOAuthConnectionLabel } from '../lib/oauth-connection-label'
import type { OAuthClient } from '../types'
import { OAuthStatusBadge } from './oauth-columns'
import { OAuthQuotaPanel } from './oauth-quota-panel'

type OAuthDetailSheetProps = {
  connection: OAuthClient | null
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Reuses the page's refresh-quota mutation (no second mutation instance). */
  onRefreshQuota: (connection: OAuthClient) => void
  /** Reuses the page's rebind mutation; opens the authorization URL in a tab. */
  onRebind: (connection: OAuthClient) => void
  isRefreshingQuota: boolean
  isRebinding: boolean
}

/** Site label with the accounts-sheet fallback ladder (name → url → #id). */
function resolveSiteLabel(connection: OAuthClient): string {
  const site = connection.site
  if (!site) return `#${connection.siteId}`
  return site.name || site.url || `#${site.id}`
}

function OAuthOverviewSection(props: { connection: OAuthClient }) {
  const { t } = useTranslation()
  const connection = props.connection
  const usernameLabel =
    connection.username ?? connection.email ?? connection.accountKey ?? null
  // Mirrors the list column's fallback so the list and the sheet cannot
  // disagree about which plan a connection is on.
  const planType =
    connection.planType ?? connection.quota?.subscription?.planType ?? null

  return (
    <section>
      <h3 className='text-sm font-medium'>
        {t('oauth.detail.sectionOverview')}
      </h3>
      <dl className='mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
        <DetailField label={t('oauth.detail.provider')}>
          <span className='font-medium'>{connection.provider}</span>
        </DetailField>
        <DetailField label={t('oauth.detail.status')}>
          <OAuthStatusBadge status={connection.status} />
        </DetailField>
        <DetailField
          label={t('oauth.detail.username')}
          title={usernameLabel ?? undefined}
        >
          {usernameLabel ?? EM_DASH}
        </DetailField>
        <DetailField
          label={t('oauth.detail.site')}
          title={resolveSiteLabel(connection)}
        >
          {resolveSiteLabel(connection)}
        </DetailField>
        <DetailField label={t('oauth.detail.accountId')}>
          <span className='tabular-nums'>#{connection.accountId}</span>
        </DetailField>
        <DetailField label={t('oauth.detail.planType')}>
          {planType?.trim() || EM_DASH}
        </DetailField>
        <DetailField
          label={t('oauth.detail.projectId')}
          title={connection.projectId ?? undefined}
          full
        >
          {connection.projectId?.trim() || EM_DASH}
        </DetailField>
      </dl>
    </section>
  )
}

function OAuthModelsSection(props: { connection: OAuthClient }) {
  const { t } = useTranslation()
  // `modelsPreview` is a truncated sample of the discovered models, not the
  // full set — deduped so repeated ids cannot collide as React keys.
  const previewModels = [...new Set(props.connection.modelsPreview ?? [])]

  return (
    <section>
      <h3 className='text-sm font-medium'>{t('oauth.detail.sectionModels')}</h3>
      <dl className='mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
        <DetailField label={t('oauth.detail.modelCount')}>
          <span className='tabular-nums'>
            {formatInt(props.connection.modelCount)}
          </span>
        </DetailField>
      </dl>
      {previewModels.length > 0 ? (
        <div className='mt-2'>
          <p className='text-muted-foreground text-[11px]'>
            {t('oauth.detail.modelsPreviewHint', {
              shown: previewModels.length,
              total: props.connection.modelCount,
            })}
          </p>
          <div className='mt-1 flex flex-wrap gap-1'>
            {previewModels.map((model) => (
              <Badge key={model} variant='outline' className='font-mono'>
                {model}
              </Badge>
            ))}
          </div>
        </div>
      ) : (
        <p className='text-muted-foreground mt-2 text-xs'>
          {t('oauth.detail.modelsEmpty')}
        </p>
      )}
    </section>
  )
}

function OAuthSyncSection(props: { connection: OAuthClient; locale: string }) {
  const { t } = useTranslation()
  const lastSyncAt = props.connection.lastModelSyncAt
  const syncError = props.connection.lastModelSyncError?.trim() || null

  return (
    <section>
      <h3 className='text-sm font-medium'>{t('oauth.detail.sectionSync')}</h3>
      <dl className='mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
        <DetailField label={t('oauth.detail.lastModelSync')}>
          {lastSyncAt ? (
            <span
              title={
                formatAbsoluteDateTime(lastSyncAt, props.locale) || undefined
              }
            >
              {formatRelativeTime(lastSyncAt, props.locale)}
            </span>
          ) : (
            EM_DASH
          )}
        </DetailField>
        <DetailField label={t('oauth.detail.routeChannelCount')}>
          <span className='tabular-nums'>
            {props.connection.routeChannelCount ?? EM_DASH}
          </span>
        </DetailField>
      </dl>
      {syncError && (
        <div className='border-warning/40 bg-warning/10 text-warning-soft-fg mt-2 flex items-start gap-2 rounded-lg border p-2 text-xs'>
          <TriangleAlertIcon aria-hidden='true' className='mt-0.5 size-3.5' />
          <div className='min-w-0'>
            <p className='font-medium'>
              {t('oauth.detail.lastModelSyncError')}
            </p>
            <p className='mt-0.5 break-words'>{syncError}</p>
          </div>
        </div>
      )}
    </section>
  )
}

export function OAuthDetailSheet(props: OAuthDetailSheetProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const connection = props.connection

  if (!connection) {
    return (
      <Sheet open={props.open} onOpenChange={props.onOpenChange}>
        <SheetContent side='right' className='sm:max-w-md' />
      </Sheet>
    )
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-md'
      >
        <SheetHeader>
          <SheetTitle className='truncate pr-6'>
            {resolveOAuthConnectionLabel(connection)}
          </SheetTitle>
          <SheetDescription>{t('oauth.detail.description')}</SheetDescription>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          <OAuthOverviewSection connection={connection} />
          <Separator />
          <OAuthQuotaPanel quota={connection.quota} />
          <Separator />
          <OAuthModelsSection connection={connection} />
          <Separator />
          <OAuthSyncSection connection={connection} locale={locale} />
        </div>

        <SheetFooter className='sm:flex-row sm:items-center'>
          <p className='text-muted-foreground max-w-[220px] text-xs sm:mr-auto'>
            {t('oauth.detail.rebindHint')}
          </p>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onRefreshQuota(connection)}
            disabled={props.isRefreshingQuota || props.isRebinding}
          >
            {props.isRefreshingQuota ? (
              <Spinner />
            ) : (
              <RefreshCwIcon aria-hidden='true' />
            )}
            {t('oauth.actions.refreshQuota')}
          </Button>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onRebind(connection)}
            disabled={props.isRefreshingQuota || props.isRebinding}
          >
            {props.isRebinding ? (
              <Spinner />
            ) : (
              <UnplugIcon aria-hidden='true' />
            )}
            {t('oauth.actions.rebind')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
