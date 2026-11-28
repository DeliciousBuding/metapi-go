import {
  ChevronDown,
  CircleAlert,
  CircleCheck,
  CircleHelp,
  Pencil,
  Snowflake,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ExpandableText } from '@/components/common/expandable-text'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { formatRemainingCooldown } from '@/hooks/use-cooldown-clock'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'

import {
  stateLabelKey,
  type EvaluatedState,
  type IndexedCandidate,
} from './route-candidate-buckets'

export type CooldownTarget =
  | { kind: 'channel'; id: number }
  | { kind: 'member'; id: number }
  | { kind: 'oauthMember'; id: number; unitId: number }

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

export function RouteCandidateCard({
  item: { candidate, index, state },
  now,
  matchedRouteId,
  onEditRoute,
  blockerText,
  clearingCooldown,
  onClearCooldown,
}: {
  item: IndexedCandidate
  now: number
  matchedRouteId?: number | null
  onEditRoute?: (id: number) => void
  blockerText: (blocker: { code: string; message: string }) => string
  clearingCooldown: boolean
  onClearCooldown: (target: CooldownTarget) => void
}) {
  const { t, i18n } = useTranslation()
  const identityKnown = !!candidate.candidateRef
  const label = identityKnown
    ? candidate.label
    : t('tokenRoutes.preview.legacyCandidate', { index: index + 1 })
  const status = t(stateLabelKey(state))
  const StatusIcon = statusIconForState(state)
  const ref = candidate.candidateRef
  const canEditRoute =
    onEditRoute &&
    matchedRouteId &&
    matchedRouteId > 0 &&
    ref?.kind === 'account' &&
    candidate.blockers?.some((blocker) =>
      ['disabled', 'source_model_mismatch'].includes(blocker.code)
    )
  const hasCooldownBlocker =
    candidate.blockers?.some((blocker) => blocker.code === 'cooldown') ?? false
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
    <li className='min-w-0 space-y-2 rounded-lg border p-3 text-sm'>
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
                            <Snowflake className='size-3' aria-hidden='true' />
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
                            disabled={clearingCooldown}
                            aria-label={t(
                              'tokenRoutes.preview.clearMemberCooldown',
                              { name: member.username }
                            )}
                            onClick={() =>
                              onClearCooldown({
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
          onClick={() => onEditRoute?.(matchedRouteId as number)}
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
            disabled={clearingCooldown}
            onClick={() => onClearCooldown(cooldownTarget)}
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
