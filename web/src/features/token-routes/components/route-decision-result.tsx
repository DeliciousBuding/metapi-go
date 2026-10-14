import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { importedUpstreamsApi } from '@/lib/api/imported-upstreams'
import { formatDateTime } from '@/lib/format'
import { toast } from '@/lib/toast'

import { routeQueryKeys } from '../api'
import type { RouteDecision } from '../types'
import { routingStrategyLabel } from '../utils'

type CooldownTarget =
  | { kind: 'channel'; id: number }
  | { kind: 'member'; id: number }

/**
 * The one route-decision renderer. The route detail's explicit check and the
 * route workspace need the same candidate truth; rendering it twice would let
 * “unknown identity”, “not evaluated”, and cooldown recovery drift.
 */
export function RouteDecisionResult(props: {
  decision: RouteDecision
  snapshotAt?: string | null
}) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  // Recovery entry (#1396 R batch): a cooldown blocker carries a per-candidate
  // clear action dispatched by candidate kind. Account/oauth_unit rows cool in
  // route_channels (channel endpoint); direct grant members cool in
  // upstream_grants (imported-member endpoint, id = group item id). Other
  // blockers are configuration, not recovery — they get no button here, and
  // legacy snapshots without a typed identity show none either.
  const clearCooldown = useMutation({
    mutationFn: (target: CooldownTarget) =>
      target.kind === 'member'
        ? importedUpstreamsApi.clearImportedMemberCooldown(target.id)
        : api.clearChannelCooldown(target.id),
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
  const candidates = decision.candidates ?? []
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
      <p>
        {t('tokenRoutes.detail.decisionCandidateCount')}: {candidates.length}
      </p>
      <ul className='space-y-2'>
        {candidates.map((candidate, index) => {
          // Old snapshots have no typed identity or evaluation context. Do not
          // infer a source type from the sign of channelId or trust their 0%.
          const identityKnown = !!candidate.candidateRef
          const label = identityKnown
            ? candidate.label
            : t('tokenRoutes.preview.legacyCandidate', { index: index + 1 })
          let status = t('tokenRoutes.preview.notEvaluated')
          if (
            decision.requiredUpstreamProtocol &&
            candidate.eligibility === 'eligible'
          ) {
            status = t('tokenRoutes.preview.noBlocker')
          }
          if (candidate.eligibility === 'blocked') {
            status = t('tokenRoutes.preview.blocked')
          }
          // Pick the cooldown recovery target by candidate kind: direct grant
          // members cool in upstream_grants (member endpoint, id = group item
          // id); account/oauth_unit rows cool in route_channels (channel
          // endpoint). Synthetic/legacy rows without a real id get no action.
          const ref = candidate.candidateRef
          const hasCooldownBlocker =
            candidate.blockers?.some(
              (blocker) => blocker.code === 'cooldown'
            ) ?? false
          let cooldownTarget: CooldownTarget | null = null
          if (hasCooldownBlocker && ref) {
            if (ref.kind === 'direct') {
              cooldownTarget =
                ref.id > 0 ? { kind: 'member', id: ref.id } : null
            } else if ((candidate.channelId ?? 0) > 0) {
              cooldownTarget = {
                kind: 'channel',
                id: candidate.channelId as number,
              }
            }
          }
          return (
            <li
              key={candidate.channelId ?? index}
              className='min-w-0 space-y-1 rounded-lg border p-2'
            >
              <div className='flex flex-wrap items-start justify-between gap-2'>
                <span className='min-w-0 break-words'>{label}</span>
                <Badge variant='outline'>{status}</Badge>
              </div>
              {candidate.candidateRef && (
                <p className='text-muted-foreground break-words'>
                  {t(`tokenRoutes.preview.kind.${candidate.candidateRef.kind}`)}{' '}
                  · #{candidate.candidateRef.id}
                </p>
              )}
              {candidate.actualModel && (
                <p className='break-words'>
                  {t('tokenRoutes.preview.upstreamModel')}:{' '}
                  {candidate.actualModel}
                </p>
              )}
              {candidate.candidateRef?.kind === 'oauth_unit' && (
                <p className='text-muted-foreground'>
                  {t('tokenRoutes.preview.memberNotSelected')}
                </p>
              )}
              {candidate.blockers?.map((blocker) => (
                <p key={blocker.code} className='break-words'>
                  {t(`tokenRoutes.preview.blockers.${blocker.code}`, {
                    defaultValue: blocker.message,
                  })}
                </p>
              ))}
              {cooldownTarget ? (
                <div className='flex flex-wrap items-center gap-2'>
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
        })}
      </ul>
    </div>
  )
}
