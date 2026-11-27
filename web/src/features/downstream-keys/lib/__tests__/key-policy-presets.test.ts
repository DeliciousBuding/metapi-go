import { describe, expect, it } from 'vitest'

import {
  detectPolicyIntersectionWarnings,
  KEY_POLICY_PRESETS,
} from '../key-policy-presets'

describe('key-policy-presets', () => {
  it('provides non-sensitive built-in presets with no secret values', () => {
    expect(KEY_POLICY_PRESETS.map((p) => p.id)).toEqual([
      'claude-code',
      'codex-cli',
      'gemini-cli',
      'sandbox',
    ])
    for (const preset of KEY_POLICY_PRESETS) {
      expect(preset.supportedModels.length).toBeGreaterThan(0)
      expect(preset).not.toHaveProperty('key')
    }
  })

  it('detects empty model policies, empty direct channels, credential conflicts, and model intersections', () => {
    expect(
      detectPolicyIntersectionWarnings({
        supportedModels: [],
        routeGrantCount: 0,
      })
    ).toContain('emptyModels')

    expect(
      detectPolicyIntersectionWarnings({
        supportedModels: [],
        routeGrantCount: 2,
      })
    ).not.toContain('emptyModels')

    expect(
      detectPolicyIntersectionWarnings({
        supportedModels: ['claude-*'],
        accessPolicy: {
          allowedUpstreamChannelIds: [],
          modelIds: ['gpt-4o'],
        },
        allowedCredentialRefs: [{ kind: 'account', id: 1 }],
        excludedCredentialRefs: [{ kind: 'account', id: 1 }],
        maxRequests: 0,
      })
    ).toEqual([
      'emptyDirectChannels',
      'credentialConflict',
      'modelIntersectionEmpty',
      'zeroQuota',
    ])
  })
})
