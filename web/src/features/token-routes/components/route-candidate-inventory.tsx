import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, Layers, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useCooldownClock } from '@/hooks/use-cooldown-clock'
import { api } from '@/lib/api'
import { importedUpstreamsApi } from '@/lib/api/imported-upstreams'
import { toast } from '@/lib/toast'

import { routeQueryKeys } from '../api'
import type { CandidateFilter } from '../lib/route-candidate-view'
import type { RouteDecision } from '../types'
import {
  buildCandidateBuckets,
  stateLabelKey,
  type IndexedCandidate,
  type BucketKey,
} from './route-candidate-buckets'
import { RouteCandidateCard, type CooldownTarget } from './route-candidate-card'

function bucketTitleKey(bucket: BucketKey): string {
  switch (bucket) {
    case 'eligible':
      return 'tokenRoutes.preview.inventory.bucketEligible'
    case 'cooling':
      return 'tokenRoutes.preview.inventory.bucketCooling'
    case 'blocked':
      return 'tokenRoutes.preview.inventory.bucketBlocked'
    case 'notEvaluated':
      return 'tokenRoutes.preview.inventory.bucketNotEvaluated'
  }
}

export function RouteCandidateInventory({
  decision,
  onEditRoute,
}: {
  decision: RouteDecision
  onEditRoute?: (id: number) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  // Reading controls are temporary to this result, like member disclosures;
  // they never mirror or write the page's URL-owned diagnostic input.
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<CandidateFilter>('all')
  const [visibleLimit, setVisibleLimit] = useState(20)
  const [blockedExpanded, setBlockedExpanded] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<ReadonlySet<string>>(
    () => new Set()
  )

  const clearCooldown = useMutation({
    mutationFn: (target: CooldownTarget) => {
      if (target.kind === 'oauthMember') {
        return api.clearOAuthMemberCooldown(target.unitId, target.id)
      }
      return target.kind === 'member'
        ? importedUpstreamsApi.clearImportedMemberCooldown(target.id)
        : api.clearChannelCooldown(target.id)
    },
    onSuccess: () => {
      toast.success(t('tokenRoutes.preview.cooldownCleared'))
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
    },
    onError: () => toast.error(t('tokenRoutes.preview.clearCooldownFailed')),
  })
  const candidates = useMemo(
    () => decision.candidates ?? [],
    [decision.candidates]
  )

  const deadlines = useMemo(
    () =>
      candidates.flatMap((candidate) => [
        candidate.cooldownUntil,
        ...(candidate.oauthMembers ?? []).map((member) => member.cooldownUntil),
      ]),
    [candidates]
  )
  const now = useCooldownClock(deadlines)

  const blockerText = (blocker: { code: string; message: string }) =>
    t(`tokenRoutes.preview.blockers.${blocker.code}`, {
      defaultValue: blocker.message,
    })
  const { counts, filteredCount, buckets, hasUsableOrCooling } =
    buildCandidateBuckets(
      candidates,
      decision.requiredUpstreamProtocol,
      filter,
      search,
      visibleLimit,
      blockerText
    )
  const isSearchActive = search.trim() !== ''
  const isBlockedOpen =
    blockedExpanded ||
    filter === 'blocked' ||
    isSearchActive ||
    !hasUsableOrCooling
  const reset = () => {
    setSearch('')
    setFilter('all')
    setVisibleLimit(20)
    setBlockedExpanded(false)
    setExpandedGroups(new Set())
  }

  const toggleGroup = (groupKey: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev)
      if (next.has(groupKey)) next.delete(groupKey)
      else next.add(groupKey)
      return next
    })
  }

  const matchedRouteId = decision.routeId ?? decision.matchedRouteId
  function renderCandidateCard(item: IndexedCandidate) {
    const { candidateRef, channelId } = item.candidate
    return (
      <RouteCandidateCard
        key={
          candidateRef
            ? `${candidateRef.kind}-${candidateRef.id}`
            : (channelId ?? item.index)
        }
        item={item}
        now={now}
        blockerText={blockerText}
        matchedRouteId={matchedRouteId}
        onEditRoute={onEditRoute}
        clearingCooldown={clearCooldown.isPending}
        onClearCooldown={clearCooldown.mutate}
      />
    )
  }
  return (
    <>
      <p className='font-medium'>
        {t('tokenRoutes.detail.decisionCandidateCount')}: {candidates.length}
      </p>
      {(candidates.length > 1 || search !== '' || filter !== 'all') && (
        <div className='space-y-2'>
          <div
            className='flex flex-wrap gap-1'
            role='group'
            aria-label={t('tokenRoutes.preview.inventory.filterLabel')}
          >
            {(['all', 'eligible', 'blocked', 'notEvaluated'] as const).map(
              (value) => (
                <Button
                  key={value}
                  type='button'
                  size='xs'
                  variant={filter === value ? 'secondary' : 'ghost'}
                  aria-pressed={filter === value}
                  onClick={() => {
                    setFilter(value)
                    setVisibleLimit(20)
                  }}
                >
                  {t(`tokenRoutes.preview.inventory.${value}`, {
                    count: counts[value],
                  })}
                </Button>
              )
            )}
          </div>
          <div className='relative'>
            <Search
              className='text-muted-foreground absolute top-2 left-2.5 size-4'
              aria-hidden='true'
            />
            <Input
              className='h-8 pl-8'
              aria-label={t('tokenRoutes.preview.inventory.searchLabel')}
              placeholder={t('tokenRoutes.preview.inventory.searchPlaceholder')}
              value={search}
              onChange={(event) => {
                setSearch(event.target.value)
                setVisibleLimit(20)
              }}
            />
          </div>
        </div>
      )}
      {candidates.length === 0 && (
        <p className='text-muted-foreground'>
          {t(
            decision.matched === false
              ? 'tokenRoutes.preview.inventory.noRoute'
              : 'tokenRoutes.preview.inventory.empty'
          )}
        </p>
      )}
      {candidates.length > 0 && (
        <p className='text-muted-foreground' role='status'>
          {t('tokenRoutes.preview.inventory.shown', {
            shown: Math.min(visibleLimit, filteredCount),
            total: filteredCount,
          })}
        </p>
      )}
      {candidates.length > 0 && filteredCount === 0 && (
        <div className='space-y-2 rounded-lg border border-dashed p-3'>
          <p>{t('tokenRoutes.preview.inventory.noMatches')}</p>
          <Button type='button' size='xs' variant='outline' onClick={reset}>
            {t('tokenRoutes.preview.inventory.reset')}
          </Button>
        </div>
      )}

      {buckets.length > 0 && (
        <div
          className='space-y-3'
          aria-label={t('tokenRoutes.preview.inventory.list')}
        >
          {buckets.map(({ bucket, count, entries }) => {
            const isBlockedBucket = bucket === 'blocked'
            const bucketOpen = !isBlockedBucket || isBlockedOpen

            return (
              <section key={bucket} className='space-y-2'>
                <div className='flex items-center justify-between gap-2'>
                  <h4 className='text-muted-foreground text-xs font-medium'>
                    {t(bucketTitleKey(bucket), { count })}
                  </h4>
                  {isBlockedBucket &&
                    filter !== 'blocked' &&
                    !isSearchActive &&
                    hasUsableOrCooling && (
                      <Button
                        type='button'
                        size='xs'
                        variant='ghost'
                        aria-expanded={bucketOpen}
                        onClick={() => setBlockedExpanded((value) => !value)}
                      >
                        {bucketOpen
                          ? t('tokenRoutes.preview.inventory.collapseBucket')
                          : t('tokenRoutes.preview.inventory.expandBucket', {
                              count,
                            })}
                      </Button>
                    )}
                </div>

                {isBlockedBucket && !bucketOpen ? (
                  <button
                    type='button'
                    onClick={() => setBlockedExpanded(true)}
                    className='bg-muted/30 hover:bg-muted/50 focus-visible:ring-ring flex w-full items-center justify-between gap-2 rounded-lg border border-dashed px-3 py-2 text-left text-xs outline-none focus-visible:ring-2'
                  >
                    <span className='text-muted-foreground'>
                      {t('tokenRoutes.preview.inventory.blockedCollapsedHint', {
                        count,
                      })}
                    </span>
                    <ChevronRight className='text-muted-foreground size-3.5 shrink-0' />
                  </button>
                ) : (
                  <ul className='space-y-2'>
                    {entries.map((entry) => {
                      if (entry.kind === 'single') {
                        return renderCandidateCard(entry.item)
                      }
                      const groupOpen =
                        expandedGroups.has(entry.groupKey) || isSearchActive
                      const firstState = entry.items[0].state
                      const stateText = t(stateLabelKey(firstState))
                      return (
                        <li
                          key={entry.groupKey}
                          className='bg-muted/20 rounded-lg border p-2.5'
                        >
                          <div className='flex flex-wrap items-center justify-between gap-2'>
                            <div className='flex min-w-0 items-center gap-2'>
                              <Layers
                                className='text-muted-foreground size-3.5 shrink-0'
                                aria-hidden='true'
                              />
                              <span className='font-medium'>
                                {t(
                                  'tokenRoutes.preview.inventory.siblingGroup',
                                  {
                                    platform: entry.platformLabel,
                                    count: entry.items.length,
                                    state: stateText,
                                  }
                                )}
                              </span>
                            </div>
                            <Button
                              type='button'
                              size='xs'
                              variant='ghost'
                              aria-expanded={groupOpen}
                              onClick={() => toggleGroup(entry.groupKey)}
                            >
                              {groupOpen
                                ? t(
                                    'tokenRoutes.preview.inventory.collapseGroup'
                                  )
                                : t(
                                    'tokenRoutes.preview.inventory.expandGroup'
                                  )}
                            </Button>
                          </div>
                          {groupOpen && (
                            <ul className='mt-2 space-y-2 border-t pt-2'>
                              {entry.items.map(renderCandidateCard)}
                            </ul>
                          )}
                        </li>
                      )
                    })}
                  </ul>
                )}
              </section>
            )
          })}
        </div>
      )}

      {filteredCount > visibleLimit && (
        <Button
          type='button'
          size='sm'
          variant='outline'
          onClick={() => setVisibleLimit((value) => value + 20)}
        >
          {t('tokenRoutes.preview.inventory.showMore', {
            count: Math.min(20, filteredCount - visibleLimit),
          })}
        </Button>
      )}
    </>
  )
}
