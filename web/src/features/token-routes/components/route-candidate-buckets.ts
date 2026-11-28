import {
  candidateEvaluation,
  candidateMatches,
  type CandidateFilter,
} from '../lib/route-candidate-view'
import type { RouteDecision } from '../types'

type Candidate = NonNullable<RouteDecision['candidates']>[number]
export type EvaluatedState = Exclude<CandidateFilter, 'all'>
export type BucketKey = 'eligible' | 'cooling' | 'blocked' | 'notEvaluated'

export type IndexedCandidate = {
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
      items: IndexedCandidate[]
    }

const BUCKET_ORDER: readonly BucketKey[] = [
  'eligible',
  'cooling',
  'blocked',
  'notEvaluated',
]

function resolveBucket(candidate: Candidate, state: EvaluatedState): BucketKey {
  if (
    state === 'blocked' &&
    candidate.blockers?.some((blocker) => blocker.code === 'cooldown')
  )
    return 'cooling'
  return state
}

export function stateLabelKey(state: EvaluatedState): string {
  if (state === 'eligible') return 'tokenRoutes.preview.noBlocker'
  if (state === 'blocked') return 'tokenRoutes.preview.blocked'
  return 'tokenRoutes.preview.notEvaluated'
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
  const groups = new Map<string, IndexedCandidate[]>()
  const members = items.map((item) => {
    const platform = resolvePlatformIdentity(item.candidate)
    if (!platform) return { item, group: null }
    const codes = (item.candidate.blockers ?? [])
      .map((blocker) => blocker.code)
      .sort()
    const key = `${bucket}:${platform.key}:${codes.join(',')}`
    const group = groups.get(key)
    if (group) group.push(item)
    else groups.set(key, [item])
    return { item, group: { key, label: platform.label } }
  })
  const entries: DisplayEntry[] = []
  for (const { item, group } of members) {
    const siblings = group && groups.get(group.key)
    if (!group || !siblings || siblings.length < 3) {
      entries.push({ kind: 'single', item })
    } else if (siblings[0] === item) {
      entries.push({
        kind: 'group',
        groupKey: group.key,
        platformLabel: group.label,
        items: siblings,
      })
    }
  }
  return entries
}

export function buildCandidateBuckets(
  candidates: Candidate[],
  protocol: RouteDecision['requiredUpstreamProtocol'],
  filter: CandidateFilter,
  search: string,
  visibleLimit: number,
  blockerText: Parameters<typeof candidateMatches>[2]
) {
  const counts = {
    all: candidates.length,
    eligible: 0,
    blocked: 0,
    notEvaluated: 0,
  }
  const indexed: IndexedCandidate[] = candidates.map((candidate, index) => {
    const state = candidateEvaluation(candidate, protocol)
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
  const hasUsableOrCooling = filtered.some(
    (entry) => entry.bucket === 'eligible' || entry.bucket === 'cooling'
  )
  const buckets = BUCKET_ORDER.flatMap((bucket) => {
    const items = visibleSlice.filter((item) => item.bucket === bucket)
    return items.length
      ? [
          {
            bucket,
            count: items.length,
            entries: buildGroupEntries(bucket, items),
          },
        ]
      : []
  })
  return { counts, filteredCount: filtered.length, hasUsableOrCooling, buckets }
}
