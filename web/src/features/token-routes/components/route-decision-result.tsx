import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ChevronDown,
  ChevronRight,
  CircleAlert,
  CircleCheck,
  CircleHelp,
  Layers,
  Pencil,
  Search,
  Snowflake,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ExpandableText } from '@/components/common/expandable-text'
import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import {
  formatRemainingCooldown,
  useCooldownClock,
} from '@/hooks/use-cooldown-clock'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { importedUpstreamsApi } from '@/lib/api/imported-upstreams'
import { formatDateTime } from '@/lib/format'
import { toast } from '@/lib/toast'

import { routeQueryKeys } from '../api'
import {
  candidateEvaluation,
  candidateMatches,
  type CandidateFilter,
} from '../lib/route-candidate-view'
import type { RouteDecision } from '../types'
import { routingStrategyLabel } from '../utils'

type Candidate = NonNullable<RouteDecision['candidates']>[number]
type EvaluatedState = Exclude<CandidateFilter, 'all'>
type BucketKey = 'eligible' | 'cooling' | 'blocked' | 'notEvaluated'

type CooldownTarget =
  | { kind: 'channel'; id: number }
  | { kind: 'member'; id: number }
  | { kind: 'oauthMember'; id: number; unitId: number }

type IndexedCandidate = {
  candidate: Candidate
  index: number
  state: EvaluatedState
  bucket: BucketKey
}

type DisplayEntry =
  | { kind: 'single'; item: IndexedCandidate }
  | {
      kind: 'group'
      groupKey: string
      platformLabel: string
      bucket: BucketKey
      items: IndexedCandidate[]
    }

const BUCKET_ORDER: readonly BucketKey[] = [
  'eligible',
  'cooling',
  'blocked',
  'notEvaluated',
]

function isCoolingCandidate(
  candidate: Candidate,
  state: EvaluatedState
): boolean {
  if (state !== 'blocked') return false
  return Boolean(
    candidate.blockers?.some((blocker) => blocker.code === 'cooldown')
  )
}

function resolveBucket(candidate: Candidate, state: EvaluatedState): BucketKey {
  if (isCoolingCandidate(candidate, state)) return 'cooling'
  return state
}

function stateLabelKey(state: EvaluatedState): string {
  if (state === 'eligible') return 'tokenRoutes.preview.noBlocker'
  if (state === 'blocked') return 'tokenRoutes.preview.blocked'
  return 'tokenRoutes.preview.notEvaluated'
}

function memberEligibilityKey(
  eligibility: 'eligible' | 'blocked' | 'notEvaluated'
): string {
  if (eligibility === 'blocked') return 'tokenRoutes.preview.memberBlocked'
  if (eligibility === 'eligible') return 'tokenRoutes.preview.memberReady'
  return 'tokenRoutes.preview.notEvaluated'
}

function badgeVariantForState(
  state: EvaluatedState
): 'destructive' | 'success' | 'secondary' {
  if (state === 'blocked') return 'destructive'
  if (state === 'eligible') return 'success'
  return 'secondary'
}

function statusIconForState(state: EvaluatedState) {
  if (state === 'eligible') return CircleCheck
  if (state === 'blocked') return CircleAlert
  return CircleHelp
}

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

function resolvePlatformIdentity(candidate: Candidate): {
  key: string
  label: string
} | null {
  const siteName = candidate.siteName?.trim()
  if (siteName) return { key: `site:${siteName}`, label: siteName }
  const siteId = candidate.candidateRef?.siteId
  if (typeof siteId === 'number' && siteId > 0) {
    return { key: `siteId:${siteId}`, label: `#${siteId}` }
  }
  return null
}

function buildGroupEntries(
  bucket: BucketKey,
  items: IndexedCandidate[]
): DisplayEntry[] {
  const countsBySignature = new Map<
    string,
    { label: string; items: IndexedCandidate[] }
  >()
  for (const item of items) {
    const platform = resolvePlatformIdentity(item.candidate)
    if (!platform) continue
    const blockerSig = (item.candidate.blockers ?? [])
      .map((b) => b.code)
      .sort()
      .join(',')
    const sig = `${bucket}:${platform.key}:${blockerSig}`
    const existing = countsBySignature.get(sig)
    if (existing) {
      existing.items.push(item)
    } else {
      countsBySignature.set(sig, { label: platform.label, items: [item] })
    }
  }

  const emittedGroups = new Set<string>()
  const entries: DisplayEntry[] = []

  for (const item of items) {
    const platform = resolvePlatformIdentity(item.candidate)
    if (!platform) {
      entries.push({ kind: 'single', item })
      continue
    }
    const blockerSig = (item.candidate.blockers ?? [])
      .map((b) => b.code)
      .sort()
      .join(',')
    const sig = `${bucket}:${platform.key}:${blockerSig}`
    const group = countsBySignature.get(sig)
    if (!group || group.items.length < 3) {
      entries.push({ kind: 'single', item })
      continue
    }
    if (emittedGroups.has(sig)) continue
    emittedGroups.add(sig)
    entries.push({
      kind: 'group',
      groupKey: sig,
      platformLabel: group.label,
      bucket,
      items: group.items,
    })
  }

  return entries
}

/**
 * The one route-decision renderer. The route detail's explicit check and the
 * route workspace need the same candidate truth; rendering it twice would let
 * “unknown identity”, “not evaluated”, and cooldown recovery drift.
 */
export function RouteDecisionResult(props: {
  decision: RouteDecision
  snapshotAt?: string | null
  onEditRoute?: (id: number) => void
}) {
  const { t, i18n } = useTranslation()
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

  const decision = props.decision
  const checkedAt =
    props.snapshotAt ?? decision.checkedAt ?? decision.generatedAt
  const model = decision.requestedModel ?? decision.model
  const pattern = decision.modelPattern ?? decision.matchedRoutePattern
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

  const counts = {
    all: candidates.length,
    eligible: 0,
    blocked: 0,
    notEvaluated: 0,
  }
  const indexed: IndexedCandidate[] = candidates.map((candidate, index) => {
    const state = candidateEvaluation(
      candidate,
      decision.requiredUpstreamProtocol
    )
    counts[state] += 1
    return {
      candidate,
      index,
      state,
      bucket: resolveBucket(candidate, state),
    }
  })

  const filtered = indexed.filter(
    ({ candidate, state }) =>
      (filter === 'all' || state === filter) &&
      candidateMatches(candidate, search, blockerText)
  )

  const visibleSlice = filtered.slice(0, visibleLimit)
  const isSearchActive = search.trim() !== ''
  const hasUsableOrCooling = filtered.some(
    (entry) => entry.bucket === 'eligible' || entry.bucket === 'cooling'
  )
  const isBlockedOpen =
    blockedExpanded ||
    filter === 'blocked' ||
    isSearchActive ||
    !hasUsableOrCooling

  const buckets = BUCKET_ORDER.map((bucket) => ({
    bucket,
    items: visibleSlice.filter((entry) => entry.bucket === bucket),
    totalInFiltered: filtered.filter((entry) => entry.bucket === bucket).length,
  })).filter((section) => section.items.length > 0)

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

  const renderCandidateCard = ({
    candidate,
    index,
    state,
  }: IndexedCandidate) => {
    const identityKnown = !!candidate.candidateRef
    const label = identityKnown
      ? candidate.label
      : t('tokenRoutes.preview.legacyCandidate', { index: index + 1 })
    const status = t(stateLabelKey(state))
    const StatusIcon = statusIconForState(state)
    const ref = candidate.candidateRef
    const canEditRoute =
      props.onEditRoute &&
      matchedRouteId &&
      matchedRouteId > 0 &&
      ref?.kind === 'account' &&
      candidate.blockers?.some((blocker) =>
        ['disabled', 'source_model_mismatch'].includes(blocker.code)
      )
    const hasCooldownBlocker =
      candidate.blockers?.some((blocker) => blocker.code === 'cooldown') ??
      false
    let cooldownTarget: CooldownTarget | null = null
    if (hasCooldownBlocker && ref) {
      if (ref.kind === 'direct') {
        cooldownTarget = ref.id > 0 ? { kind: 'member', id: ref.id } : null
      } else if ((candidate.channelId ?? 0) > 0) {
        cooldownTarget = {
          kind: 'channel',
          id: candidate.channelId as number,
        }
      }
    }
    const remaining = formatRemainingCooldown(candidate.cooldownUntil, now)

    return (
      <li
        key={ref ? `${ref.kind}-${ref.id}` : (candidate.channelId ?? index)}
        className='min-w-0 space-y-2 rounded-lg border p-3 text-sm'
      >
        <div className='flex flex-wrap items-start justify-between gap-2'>
          <span className='min-w-0 font-medium [overflow-wrap:anywhere]'>
            {label}
          </span>
          <Badge variant={badgeVariantForState(state)} className='gap-1'>
            <StatusIcon className='size-3.5' aria-hidden='true' />
            {status}
          </Badge>
        </div>
        {candidate.candidateRef && (
          <p className='text-muted-foreground break-words'>
            {t(`tokenRoutes.preview.kind.${candidate.candidateRef.kind}`)} · #
            {candidate.candidateRef.id}
          </p>
        )}
        {candidate.actualModel && (
          <p className='break-words'>
            {t('tokenRoutes.preview.upstreamModel')}: {candidate.actualModel}
          </p>
        )}
        {(candidate.priority != null || candidate.weight != null) && (
          <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs tabular-nums'>
            {candidate.priority != null && (
              <span>
                {t('tokenRoutes.preview.inventory.priority', {
                  value: candidate.priority,
                })}
              </span>
            )}
            {candidate.weight != null && (
              <span>
                {t('tokenRoutes.preview.inventory.weight', {
                  value: candidate.weight,
                })}
              </span>
            )}
          </div>
        )}
        {candidate.candidateRef?.kind === 'oauth_unit' && (
          <p className='text-muted-foreground'>
            {t('tokenRoutes.preview.memberNotSelected')}
          </p>
        )}
        {ref?.kind === 'oauth_unit' && candidate.oauthMembers && (
          <Collapsible>
            <CollapsibleTrigger className='group focus-visible:ring-ring flex w-full items-center justify-between gap-2 rounded-md py-2 text-left font-medium outline-none focus-visible:ring-2'>
              {t('tokenRoutes.preview.members', {
                count: candidate.oauthMembers.length,
              })}
              <ChevronDown
                aria-hidden='true'
                className='size-4 shrink-0 transition-transform group-aria-expanded:rotate-180'
              />
            </CollapsibleTrigger>
            <CollapsibleContent>
              <p className='text-muted-foreground mb-2'>
                {t('tokenRoutes.preview.memberScope')}
              </p>
              <ul className='divide-y border-t text-sm'>
                {candidate.oauthMembers.map((member) => {
                  const memberRemaining = formatRemainingCooldown(
                    member.cooldownUntil,
                    now
                  )
                  return (
                    <li key={member.memberId} className='space-y-2 py-3'>
                      <div className='flex flex-wrap items-start justify-between gap-2'>
                        <span className='min-w-0 [overflow-wrap:anywhere] break-words'>
                          {member.siteName} · {member.username}
                        </span>
                        <Badge variant='outline'>
                          {t(memberEligibilityKey(member.eligibility))}
                        </Badge>
                      </div>
                      {member.blockers.map((blocker) => (
                        <ExpandableText
                          key={blocker.code}
                          text={blockerText(blocker)}
                        />
                      ))}
                      {member.blockers.some(
                        (blocker) => blocker.code === 'cooldown'
                      ) && (
                        <div className='flex flex-wrap items-center gap-2'>
                          {memberRemaining && (
                            <Badge
                              variant='warning'
                              className='gap-1 tabular-nums'
                            >
                              <Snowflake
                                className='size-3'
                                aria-hidden='true'
                              />
                              {t('tokenRoutes.preview.cooldownRemaining', {
                                remaining: memberRemaining,
                              })}
                            </Badge>
                          )}
                          {member.cooldownUntil && (
                            <span className='text-muted-foreground text-xs'>
                              {t('tokenRoutes.preview.cooldownUntil', {
                                time: formatDateTime(
                                  member.cooldownUntil,
                                  toBcp47(i18n.language)
                                ),
                              })}
                            </span>
                          )}
                          {ref.id > 0 && member.memberId > 0 && (
                            <Button
                              type='button'
                              variant='outline'
                              size='xs'
                              disabled={clearCooldown.isPending}
                              aria-label={t(
                                'tokenRoutes.preview.clearMemberCooldown',
                                { name: member.username }
                              )}
                              onClick={() =>
                                clearCooldown.mutate({
                                  kind: 'oauthMember',
                                  id: member.memberId,
                                  unitId: ref.id,
                                })
                              }
                            >
                              {t('tokenRoutes.preview.clearCooldown')}
                            </Button>
                          )}
                        </div>
                      )}
                    </li>
                  )
                })}
              </ul>
            </CollapsibleContent>
          </Collapsible>
        )}
        {candidate.blockers?.map((blocker) => (
          <ExpandableText key={blocker.code} text={blockerText(blocker)} />
        ))}
        {!candidate.blockers?.length && candidate.reasonText && (
          <ExpandableText text={candidate.reasonText} />
        )}
        {canEditRoute && (
          <Button
            type='button'
            variant='outline'
            size='xs'
            onClick={() => props.onEditRoute?.(matchedRouteId as number)}
          >
            <Pencil className='size-3.5' aria-hidden='true' />
            {t('tokenRoutes.preview.inventory.editRoute')}
          </Button>
        )}
        {cooldownTarget ? (
          <div className='flex flex-wrap items-center gap-2'>
            {remaining ? (
              <Badge variant='warning' className='gap-1 tabular-nums'>
                <Snowflake className='size-3' aria-hidden='true' />
                {t('tokenRoutes.preview.cooldownRemaining', { remaining })}
              </Badge>
            ) : null}
            {candidate.cooldownUntil ? (
              <span className='text-muted-foreground'>
                {t('tokenRoutes.preview.cooldownUntil', {
                  time: formatDateTime(
                    candidate.cooldownUntil,
                    toBcp47(i18n.language)
                  ),
                })}
              </span>
            ) : null}
            <Button
              type='button'
              variant='outline'
              size='xs'
              disabled={clearCooldown.isPending}
              onClick={() => clearCooldown.mutate(cooldownTarget)}
            >
              {t('tokenRoutes.preview.clearCooldown')}
            </Button>
          </div>
        ) : null}
        {candidate.probabilityEvaluated === true &&
          candidate.probability != null && (
            <p>
              {t('tokenRoutes.preview.probability')}:{' '}
              {(candidate.probability * 100).toFixed(1)}%
            </p>
          )}
      </li>
    )
  }

  return (
    <div className='space-y-2 text-xs'>
      <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1'>
        <dt className='text-muted-foreground'>
          {t('tokenRoutes.detail.decisionModel')}
        </dt>
        <dd className='min-w-0'>{model ? <ModelPill model={model} /> : '—'}</dd>
        <dt className='text-muted-foreground'>
          {t('tokenRoutes.preview.checkedAt')}
        </dt>
        <dd>{formatDateTime(checkedAt, toBcp47(i18n.language))}</dd>
        <dt className='text-muted-foreground'>
          {t('tokenRoutes.preview.matchedRoute')}
        </dt>
        <dd className='break-words'>
          {pattern || t('tokenRoutes.preview.noMatch')}
        </dd>
        <dt className='text-muted-foreground'>
          {t('tokenRoutes.preview.strategy')}
        </dt>
        <dd>
          {decision.routingStrategy
            ? t(routingStrategyLabel(decision.routingStrategy))
            : '—'}
        </dd>
      </dl>
      <p className='text-muted-foreground'>
        {t('tokenRoutes.preview.noSelection')}
      </p>
      {!decision.requiredUpstreamProtocol && (
        <p>{t('tokenRoutes.preview.protocolUnknown')}</p>
      )}
      {decision.allowUpstreamProtocolConversion && (
        <p>{t('tokenRoutes.preview.conversion')}</p>
      )}
      {decision.notEvaluated?.includes('concreteModel') && (
        <p>{t('tokenRoutes.preview.modelUnknown')}</p>
      )}
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
            shown: Math.min(visibleLimit, filtered.length),
            total: filtered.length,
          })}
        </p>
      )}
      {candidates.length > 0 && filtered.length === 0 && (
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
          {buckets.map(({ bucket, items }) => {
            const isBlockedBucket = bucket === 'blocked'
            const bucketOpen = !isBlockedBucket || isBlockedOpen
            const entries = buildGroupEntries(bucket, items)

            return (
              <section key={bucket} className='space-y-2'>
                <div className='flex items-center justify-between gap-2'>
                  <h4 className='text-muted-foreground text-xs font-medium'>
                    {t(bucketTitleKey(bucket), { count: items.length })}
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
                              count: items.length,
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
                        count: items.length,
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

      {filtered.length > visibleLimit && (
        <Button
          type='button'
          size='sm'
          variant='outline'
          onClick={() => setVisibleLimit((value) => value + 20)}
        >
          {t('tokenRoutes.preview.inventory.showMore', {
            count: Math.min(20, filtered.length - visibleLimit),
          })}
        </Button>
      )}
    </div>
  )
}
