/* eslint-disable no-nested-ternary -- status label selection uses chained ternary */
// metapi-go features/token-routes/components — route detail side sheet.
// i18n: all user-visible strings migrated to t() calls.
// `routingStrategyLabel()` returns an i18n key; wrapped with `t()`.

import { ExternalLink, Loader2, RefreshCw, Snowflake } from 'lucide-react'
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

import {
  useClearRouteCooldown,
  useRebuildRoutes,
  useRefreshRouteDecisions,
  useRouteChannels,
} from '../api'
import type { RouteChannel, RouteDecision, RouteSummaryRow } from '../types'
import {
  formatContextLength,
  isExplicitGroupRoute,
  resolveRouteTitle,
  routingStrategyLabel,
} from '../utils'

interface RouteDetailSheetProps {
  route: RouteSummaryRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function RouteDetailSheet({
  route,
  open,
  onOpenChange,
}: RouteDetailSheetProps) {
  const { t } = useTranslation()
  const channelsQuery = useRouteChannels(route?.id ?? null)
  const clearCooldownMutation = useClearRouteCooldown()
  const refreshDecisionMutation = useRefreshRouteDecisions()
  const rebuildMutation = useRebuildRoutes()

  if (!route) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' className='sm:max-w-md' />
      </Sheet>
    )
  }

  const isReadOnly = route.kind === 'zero_channel' || route.readOnly === true
  const title = resolveRouteTitle(route)
  const channels = channelsQuery.data ?? []
  const decision = route.decisionSnapshot ?? null

  const handleClearCooldown = async () => {
    if (!route) return
    try {
      await clearCooldownMutation.mutateAsync(route.id)
    } catch {}
  }

  const handleRefreshDecision = async () => {
    try {
      await refreshDecisionMutation.mutateAsync()
    } catch {}
  }

  const handleRebuild = async () => {
    try {
      await rebuildMutation.mutateAsync({ refreshModels: true })
    } catch {}
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-md'
      >
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2'>
            <span className='truncate'>{title}</span>
            {isExplicitGroupRoute(route) ? (
              <Badge variant='secondary'>
                {t('tokenRoutes.detail.badgeGroup')}
              </Badge>
            ) : (
              <Badge variant='outline'>
                {t('tokenRoutes.detail.badgeMatch')}
              </Badge>
            )}
            {isReadOnly && (
              <Badge variant='outline' className='text-muted-foreground'>
                {t('tokenRoutes.detail.notGenerated')}
              </Badge>
            )}
          </SheetTitle>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField label={t('tokenRoutes.detail.matchRule')}>
              <code className='font-mono text-xs'>{route.modelPattern}</code>
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.displayName')}>
              {route.displayName || '—'}
            </DetailField>
            <DetailField label={t('common.status')}>
              {isReadOnly
                ? t('tokenRoutes.columns.notEnabled')
                : route.enabled
                  ? t('tokenRoutes.columns.enable')
                  : t('tokenRoutes.columns.disable')}
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.strategy')}>
              {isReadOnly
                ? '—'
                : t(routingStrategyLabel(route.routingStrategy))}
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.context')}>
              {formatContextLength(route.contextLength) || '—'}
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.channels')}>
              {t('tokenRoutes.detail.channelCount', {
                total: route.channelCount,
                enabled: route.enabledChannelCount,
              })}
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.sites')}>
              {route.siteNames?.length ? route.siteNames.join(', ') : '—'}
            </DetailField>
            <DetailField label={t('tokenRoutes.detail.decisionRefresh')}>
              {route.decisionRefreshedAt || '—'}
            </DetailField>
          </dl>

          {route.modelMapping && (
            <div className='bg-muted/40 rounded-lg border p-2'>
              <div className='text-muted-foreground text-[11px]'>
                {t('tokenRoutes.detail.modelMapping')}
              </div>
              <code className='block font-mono text-xs break-all'>
                {route.modelMapping}
              </code>
            </div>
          )}

          <div className='flex flex-wrap justify-end gap-2'>
            {!isReadOnly && (
              <Button
                variant='outline'
                size='sm'
                onClick={handleClearCooldown}
                disabled={clearCooldownMutation.isPending}
              >
                <Snowflake />
                {t('tokenRoutes.detail.clearCooldown')}
              </Button>
            )}
            <Button
              variant='outline'
              size='sm'
              onClick={handleRefreshDecision}
              disabled={refreshDecisionMutation.isPending}
            >
              <RefreshCw
                className={
                  refreshDecisionMutation.isPending ? 'animate-spin' : undefined
                }
              />
              {t('tokenRoutes.detail.refreshDecision')}
            </Button>
          </div>

          <Separator />

          <div className='space-y-2'>
            <div className='flex items-center justify-between'>
              <h3 className='text-sm font-medium'>
                {t('tokenRoutes.detail.channelList')}
              </h3>
              {channelsQuery.isFetching && (
                <Loader2 className='text-muted-foreground size-3.5 animate-spin' />
              )}
            </div>
            {channels.length === 0 ? (
              <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-center text-sm'>
                {isReadOnly
                  ? t('tokenRoutes.detail.channelEmptyReadOnly')
                  : t('tokenRoutes.detail.channelEmptyEditable')}
              </p>
            ) : (
              <ul className='space-y-1.5'>
                {channels.map((channel) => (
                  <ChannelRow key={channel.id} channel={channel} />
                ))}
              </ul>
            )}
          </div>

          <Separator />
          <DecisionSnapshotSection decision={decision} />
        </div>

        <SheetFooter>
          {isReadOnly ? (
            <Button onClick={handleRebuild} variant='default'>
              <ExternalLink />
              {t('tokenRoutes.detail.rebuildRoutes')}
            </Button>
          ) : (
            <Button onClick={handleRebuild} variant='outline'>
              <RefreshCw
                className={
                  rebuildMutation.isPending ? 'animate-spin' : undefined
                }
              />
              {t('tokenRoutes.detail.rebuild')}
            </Button>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function ChannelRow({ channel }: { channel: RouteChannel }) {
  const { t } = useTranslation()
  const accountLabel =
    channel.account?.username || `account-${channel.accountId}`
  const siteLabel = channel.site?.name || channel.site?.platform || ''
  const tokenLabel =
    channel.token?.name ||
    (channel.tokenId
      ? `token-${channel.tokenId}`
      : t('tokenRoutes.detail.channelTokenUnbound'))
  const sourceModel = channel.sourceModel || '—'
  const cooldownActive =
    Boolean(channel.cooldownUntil) &&
    new Date(channel.cooldownUntil as string) > new Date()
  return (
    <li className='flex items-center gap-2 rounded-lg border p-2 text-xs'>
      <div className='flex flex-1 flex-col gap-0.5'>
        <div className='flex items-center gap-1.5'>
          <span className='truncate font-medium'>{accountLabel}</span>
          {siteLabel && (
            <span className='text-muted-foreground'>@ {siteLabel}</span>
          )}
        </div>
        <div className='text-muted-foreground flex flex-wrap items-center gap-1.5'>
          <span>token: {tokenLabel}</span>
          <span>
            {t('tokenRoutes.detail.channelUpstream')} {sourceModel}
          </span>
          <span>
            {t('tokenRoutes.detail.channelWeight')} {channel.weight}
          </span>
          <span>
            {t('tokenRoutes.detail.channelPriority')} {channel.priority}
          </span>
        </div>
      </div>
      <div className='flex flex-col items-end gap-1'>
        <Badge variant={channel.enabled ? 'default' : 'secondary'}>
          {channel.enabled
            ? t('tokenRoutes.columns.enable')
            : t('tokenRoutes.columns.disable')}
        </Badge>
        {cooldownActive && (
          <Badge variant='warning'>{t('tokenRoutes.detail.cooldown')}</Badge>
        )}
      </div>
    </li>
  )
}

function DecisionSnapshotSection({
  decision,
}: {
  decision: RouteDecision | null
}) {
  const { t } = useTranslation()
  if (!decision) {
    return (
      <div className='space-y-1'>
        <h3 className='text-sm font-medium'>
          {t('tokenRoutes.detail.decisionSnapshot')}
        </h3>
        <p className='text-muted-foreground text-xs'>
          {t('tokenRoutes.detail.decisionEmpty')}
        </p>
      </div>
    )
  }
  const candidates = decision.candidates ?? []
  const generatedAt = decision.generatedAt || '—'
  const reasonText = decision.reasonText || decision.matchedRoutePattern || ''
  return (
    <div className='space-y-2'>
      <h3 className='text-sm font-medium'>
        {t('tokenRoutes.detail.decisionSnapshot')}
      </h3>
      <dl className='grid grid-cols-2 gap-x-3 gap-y-1 text-xs'>
        <DetailField label={t('tokenRoutes.detail.decisionModel')}>
          {decision.model || '—'}
        </DetailField>
        <DetailField label={t('tokenRoutes.detail.decisionGeneratedAt')}>
          {generatedAt}
        </DetailField>
        <DetailField label={t('tokenRoutes.detail.decisionCandidateCount')}>
          {candidates.length}
        </DetailField>
        <DetailField label={t('tokenRoutes.detail.decisionSelectedChannel')}>
          {decision.selectedChannelId ?? '—'}
        </DetailField>
      </dl>
      {reasonText && (
        <div className='bg-muted/40 rounded-lg border p-2 text-xs'>
          <div className='text-muted-foreground text-[11px]'>
            {t('tokenRoutes.detail.decisionReason')}
          </div>
          <p className='break-words'>{reasonText}</p>
        </div>
      )}
      {candidates.length > 0 && (
        <ul className='space-y-1'>
          {candidates.slice(0, 6).map((candidate, index) => (
            <li
              key={candidate.channelId ?? index}
              className='flex items-center justify-between rounded border px-2 py-1 text-xs'
            >
              <span className='truncate'>
                {candidate.username || `account-${candidate.accountId}`}
                {candidate.sourceModel ? ` · ${candidate.sourceModel}` : ''}
              </span>
              <span className='text-muted-foreground tabular-nums'>
                {candidate.probability != null
                  ? `${(candidate.probability * 100).toFixed(1)}%`
                  : '—'}
              </span>
            </li>
          ))}
          {candidates.length > 6 && (
            <li className='text-muted-foreground px-2 text-[11px]'>
              {t('tokenRoutes.detail.moreCandidates', {
                count: candidates.length - 6,
              })}
            </li>
          )}
        </ul>
      )}
    </div>
  )
}

function DetailField({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className='flex flex-col'>
      <dt className='text-muted-foreground text-[11px]'>{label}</dt>
      <dd className='truncate'>{children}</dd>
    </div>
  )
}
