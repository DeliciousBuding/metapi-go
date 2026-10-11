import { describe, expect, it } from 'vitest'

import type {
  ImportedMember,
  ImportedUpstream,
} from '@/lib/api/imported-upstreams'

import { connectionReadiness, memberAvailability } from '../lib/upstream-view'

const item: ImportedUpstream = {
  id: 1,
  name: 'Fixture platform',
  originKey: 'native',
  dialect: 'generic',
  baseUrl: 'https://example.test',
  enabled: true,
  credentialCount: 1,
  modelCount: 1,
}
const member: ImportedMember = {
  id: 1,
  groupId: 1,
  groupName: 'public-model',
  routeId: 1,
  channelId: 1,
  modelName: 'actual-model',
  credentialName: 'Plan key',
  credentialEnabled: true,
  effectiveEnabled: true,
  protocols: 2,
  mode: 'failover',
  activeItemId: 0,
  priority: 0,
  weight: 1,
}
const now = Date.parse('2026-10-11T00:00:00Z')

describe('upstream configuration readiness', () => {
  it('never treats missing eligibility data as a healthy or configured member', () => {
    expect(memberAvailability(undefined, now)).toBe('unknown')
    expect(
      memberAvailability({ ...member, effectiveEnabled: undefined }, now)
    ).toBe('unknown')
    expect(
      connectionReadiness(
        item,
        [{ ...member, effectiveEnabled: undefined }],
        now
      )
    ).toBe('unknown')
  })
  it('identifies the first missing configuration step, not just the enabled flag', () => {
    expect(connectionReadiness({ ...item, credentialCount: 0 }, [], now)).toBe(
      'credentials'
    )
    expect(connectionReadiness({ ...item, modelCount: 0 }, [], now)).toBe(
      'models'
    )
    expect(connectionReadiness(item, [], now)).toBe('routes')
    expect(
      connectionReadiness({ ...item, enabled: false }, [member], now)
    ).toBe('paused')
  })
  it('shows a configured sibling despite another member cooling down, and lets expired cooldowns recover', () => {
    const cooling = { ...member, cooldownUntil: '2026-10-11T01:00:00Z' }
    expect(connectionReadiness(item, [cooling], now)).toBe('cooling')
    expect(connectionReadiness(item, [cooling, member], now)).toBe('configured')
    expect(
      memberAvailability(cooling, Date.parse('2026-10-11T02:00:00Z'))
    ).toBe('configured')
    expect(
      memberAvailability({ ...cooling, effectiveEnabled: false }, now)
    ).toBe('inactive')
  })
})
