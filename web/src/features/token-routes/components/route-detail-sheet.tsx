// metapi-go features/token-routes/components — route detail side sheet.
// Shows route metadata + the live channel list (fetched from
// GET /api/routes/:id/channels) + a decision snapshot projection + a small
// debug section. Mirrors the accounts feature's `account-detail-sheet.tsx`
// structure: a Sheet with an overview dl, an embedded data section, and a
// footer CTA row.
//
// For zero-channel placeholder routes (kind: 'zero_channel'), the channel
// list is intentionally empty and the footer CTAs collapse to a single
// "auto-rebuild" hint — mirroring the legacy RouteCard readOnly treatment.

import {
  ExternalLink,
  Loader2,
  RefreshCw,
  Snowflake,
} from 'lucide-react'
import type { ReactNode } from 'react'

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
import {
  type RouteChannel,
  type RouteDecision,
  type RouteSummaryRow,
} from '../types'
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
    } catch {
      // http-client toasted
    }
  }

  const handleRefreshDecision = async () => {
    try {
      await refreshDecisionMutation.mutateAsync()
    } catch {
      // http-client toasted
    }
  }

  const handleRebuild = async () => {
    try {
      await rebuildMutation.mutateAsync({ refreshModels: true })
    } catch {
      // http-client toasted
    }
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
              <Badge variant='secondary'>分组</Badge>
            ) : (
              <Badge variant='outline'>匹配</Badge>
            )}
            {isReadOnly && (
              <Badge variant='outline' className='text-muted-foreground'>
                未生成
              </Badge>
            )}
          </SheetTitle>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          {/* Overview grid */}
          <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField label='匹配规则'>
              <code className='font-mono text-xs'>{route.modelPattern}</code>
            </DetailField>
            <DetailField label='显示名'>
              {route.displayName || '—'}
            </DetailField>
            <DetailField label='状态'>
              {isReadOnly ? '未启用' : route.enabled ? '启用' : '禁用'}
            </DetailField>
            <DetailField label='策略'>
              {isReadOnly ? '—' : routingStrategyLabel(route.routingStrategy)}
            </DetailField>
            <DetailField label='上下文'>
              {formatContextLength(route.contextLength) || '未知'}
            </DetailField>
            <DetailField label='通道'>
              {route.channelCount}（启用 {route.enabledChannelCount}）
            </DetailField>
            <DetailField label='站点'>
              {route.siteNames?.length
                ? route.siteNames.join(', ')
                : '—'}
            </DetailField>
            <DetailField label='决策刷新'>
              {route.decisionRefreshedAt || '—'}
            </DetailField>
          </dl>

          {route.modelMapping && (
            <div className='rounded-lg border bg-muted/40 p-2'>
              <div className='text-[11px] text-muted-foreground'>模型映射</div>
              <code className='block break-all font-mono text-xs'>
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
                清除冷却
              </Button>
            )}
            <Button
              variant='outline'
              size='sm'
              onClick={handleRefreshDecision}
              disabled={refreshDecisionMutation.isPending}
            >
              <RefreshCw
                className={refreshDecisionMutation.isPending ? 'animate-spin' : undefined}
              />
              刷新决策
            </Button>
          </div>

          <Separator />

          {/* Channel list */}
          <div className='space-y-2'>
            <div className='flex items-center justify-between'>
              <h3 className='text-sm font-medium'>通道列表</h3>
              {channelsQuery.isFetching && (
                <Loader2 className='size-3.5 animate-spin text-muted-foreground' />
              )}
            </div>
            {channels.length === 0 ? (
              <p className='rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground'>
                {isReadOnly
                  ? '暂无通道，先补齐连接配置后再重建路由。'
                  : '暂无通道，编辑路由添加通道或等待自动重建。'}
              </p>
            ) : (
              <ul className='space-y-1.5'>
                {channels.map((channel) => (
                  <ChannelRow key={channel.id} channel={channel} />
                ))}
              </ul>
            )}
          </div>

          {/* Decision snapshot + debug */}
          <Separator />
          <DecisionSnapshotSection decision={decision} />
        </div>

        <SheetFooter>
          {isReadOnly ? (
            <Button onClick={handleRebuild} variant='default'>
              <ExternalLink />
              自动重建路由
            </Button>
          ) : (
            <Button onClick={handleRebuild} variant='outline'>
              <RefreshCw className={rebuildMutation.isPending ? 'animate-spin' : undefined} />
              自动重建
            </Button>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

// ---------------------------------------------------------------------------
// Channel row — compact summary of one route channel
// ---------------------------------------------------------------------------

function ChannelRow({ channel }: { channel: RouteChannel }) {
  const accountLabel = channel.account?.username || `account-${channel.accountId}`
  const siteLabel = channel.site?.name || channel.site?.platform || ''
  const tokenLabel = channel.token?.name || (channel.tokenId ? `token-${channel.tokenId}` : '未绑定')
  const sourceModel = channel.sourceModel || '—'
  const cooldownActive =
    Boolean(channel.cooldownUntil) && new Date(channel.cooldownUntil as string) > new Date()

  return (
    <li className='flex items-center gap-2 rounded-lg border p-2 text-xs'>
      <div className='flex flex-1 flex-col gap-0.5'>
        <div className='flex items-center gap-1.5'>
          <span className='font-medium truncate'>{accountLabel}</span>
          {siteLabel && (
            <span className='text-muted-foreground'>@ {siteLabel}</span>
          )}
        </div>
        <div className='flex flex-wrap items-center gap-1.5 text-muted-foreground'>
          <span>token: {tokenLabel}</span>
          <span>· 上游: {sourceModel}</span>
          <span>· 权重: {channel.weight}</span>
          <span>· 优先级: {channel.priority}</span>
        </div>
      </div>
      <div className='flex flex-col items-end gap-1'>
        <Badge variant={channel.enabled ? 'default' : 'secondary'}>
          {channel.enabled ? '启用' : '禁用'}
        </Badge>
        {cooldownActive && (
          <Badge variant='warning'>冷却中</Badge>
        )}
      </div>
    </li>
  )
}

// ---------------------------------------------------------------------------
// Decision snapshot — defensive projection of the server's RouteDecision
// ---------------------------------------------------------------------------

function DecisionSnapshotSection({
  decision,
}: {
  decision: RouteDecision | null
}) {
  if (!decision) {
    return (
      <div className='space-y-1'>
        <h3 className='text-sm font-medium'>决策快照</h3>
        <p className='text-xs text-muted-foreground'>
          暂无缓存决策，点击上方「刷新决策」生成。
        </p>
      </div>
    )
  }

  const candidates = decision.candidates ?? []
  const generatedAt = decision.generatedAt || '—'
  const reasonText = decision.reasonText || decision.matchedRoutePattern || ''

  return (
    <div className='space-y-2'>
      <h3 className='text-sm font-medium'>决策快照</h3>
      <dl className='grid grid-cols-2 gap-x-3 gap-y-1 text-xs'>
        <DetailField label='匹配模型'>
          {decision.model || '—'}
        </DetailField>
        <DetailField label='生成时间'>
          {generatedAt}
        </DetailField>
        <DetailField label='候选数'>
          {candidates.length}
        </DetailField>
        <DetailField label='选中通道'>
          {decision.selectedChannelId ?? '—'}
        </DetailField>
      </dl>
      {reasonText && (
        <div className='rounded-lg border bg-muted/40 p-2 text-xs'>
          <div className='text-[11px] text-muted-foreground'>决策理由</div>
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
              <span className='tabular-nums text-muted-foreground'>
                {candidate.probability != null
                  ? `${(candidate.probability * 100).toFixed(1)}%`
                  : '—'}
              </span>
            </li>
          ))}
          {candidates.length > 6 && (
            <li className='px-2 text-[11px] text-muted-foreground'>
              +{candidates.length - 6} 个候选
            </li>
          )}
        </ul>
      )}
    </div>
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
