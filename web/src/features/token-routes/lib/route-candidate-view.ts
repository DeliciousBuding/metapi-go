import type { RouteDecision } from '../types'

type Candidate = NonNullable<RouteDecision['candidates']>[number]
export type CandidateFilter = 'all' | 'eligible' | 'blocked' | 'notEvaluated'

export function candidateEvaluation(
  candidate: Candidate,
  protocol: RouteDecision['requiredUpstreamProtocol']
): Exclude<CandidateFilter, 'all'> {
  if (candidate.eligibility === 'blocked') return 'blocked'
  if (protocol && candidate.eligibility === 'eligible') return 'eligible'
  return 'notEvaluated'
}

export function candidateMatches(
  candidate: Candidate,
  needle: string,
  blockerText: (blocker: { code: string; message: string }) => string
): boolean {
  const ref = candidate.candidateRef
  // Typed identifiers are searchable; old synthetic account ids and labels
  // must not resurrect identities that were never recorded.
  const identity = ref ? [candidate.label, ...Object.values(ref)] : []
  return [
    ...identity,
    candidate.actualModel,
    candidate.reasonText,
    ...(candidate.blockers ?? []).flatMap((blocker) => [
      blocker.code,
      blocker.message,
      blockerText(blocker),
    ]),
    ...(candidate.oauthMembers ?? []).flatMap((member) => [
      member.username,
      member.siteName,
      member.accountId,
      member.memberId,
      ...member.blockers.flatMap((blocker) => [
        blocker.code,
        blocker.message,
        blockerText(blocker),
      ]),
    ]),
  ]
    .filter((value) => value != null)
    .join(' ')
    .toLocaleLowerCase()
    .includes(needle.trim().toLocaleLowerCase())
}
