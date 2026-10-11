import type {
  ImportedMember,
  ImportedUpstream,
} from '@/lib/api/imported-upstreams'

export type UpstreamDetailTab =
  | 'connection'
  | 'credentials'
  | 'models'
  | 'routes'
export type MemberAvailability =
  | 'configured'
  | 'cooling'
  | 'inactive'
  | 'unknown'

// This is configuration eligibility, not a live upstream health check.
export function memberAvailability(
  member: ImportedMember | undefined,
  now: number
): MemberAvailability {
  if (!member) return 'unknown'
  if (member.effectiveEnabled === false || member.protocols === 0) {
    return 'inactive'
  }
  if (member.effectiveEnabled !== true) return 'unknown'
  if (member.cooldownUntil && Date.parse(member.cooldownUntil) > now) {
    return 'cooling'
  }
  return 'configured'
}

export function connectionReadiness(
  item: ImportedUpstream,
  members: ImportedMember[],
  now: number
) {
  if (!item.enabled) return 'paused'
  if (!item.credentialCount) return 'credentials'
  if (!item.modelCount) return 'models'
  const routes = members.filter((member) => member.routeId > 0)
  if (!routes.length) return 'routes'
  const states = new Set(
    routes.map((member) => memberAvailability(member, now))
  )
  if (states.has('configured')) return 'configured'
  if (states.has('cooling')) return 'cooling'
  if (states.has('unknown')) return 'unknown'
  return 'inactive'
}
