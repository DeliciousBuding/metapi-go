// Behavior test for the onboarding checklist state machine.
//
// Locks the two contracts the dashboard panel depends on:
//   (a) journey order + "done" verdict — a step is done when its count is
//       above zero, and the CTA lands on the FIRST step with nothing built;
//   (b) the anti-flash rule — an unanswered count (undefined/null/NaN, i.e.
//       still loading or errored) yields null rather than a checklist that
//       claims every step is empty. This is the contract the previous
//       single-step banner held by keying off `siteCount === 0`.

import { describe, expect, it } from 'vitest'

import { deriveOnboardingSteps, nextOnboardingStep } from '../onboarding-steps'

describe('deriveOnboardingSteps', () => {
  it('returns the three journey steps in upstream → route → key order', () => {
    const steps = deriveOnboardingSteps({
      upstreamChannels: 2,
      accounts: 1,
      routes: 3,
      keys: 0,
    })

    expect(steps?.map((step) => step.id)).toEqual([
      'upstreams',
      'routes',
      'keys',
    ])
    expect(steps?.map((step) => step.count)).toEqual([3, 3, 0])
    expect(steps?.map((step) => step.done)).toEqual([true, true, false])
  })

  it('marks every step pending on a brand-new deployment', () => {
    const steps = deriveOnboardingSteps({
      upstreamChannels: 0,
      accounts: 0,
      routes: 0,
      keys: 0,
    })

    expect(steps).not.toBeNull()
    expect(steps?.every((step) => !step.done)).toBe(true)
    expect(nextOnboardingStep(steps ?? [])?.id).toBe('upstreams')
  })

  it('marks the journey complete once all three counts are above zero', () => {
    const steps = deriveOnboardingSteps({
      upstreamChannels: 1,
      accounts: 1,
      routes: 1,
      keys: 1,
    })

    expect(steps?.every((step) => step.done)).toBe(true)
    // null = no next action = the panel retires itself.
    expect(nextOnboardingStep(steps ?? [])).toBeNull()
  })

  it.each([
    [
      'an unanswered independent-upstream count',
      { upstreamChannels: undefined },
    ],
    ['an unanswered key count', { keys: undefined }],
    ['a null route count', { routes: null }],
    ['a NaN account count', { accounts: Number.NaN }],
  ])(
    'returns null for %s instead of inventing an empty checklist',
    (_label, patch) => {
      const counts = {
        upstreamChannels: 2,
        accounts: 2,
        routes: 2,
        keys: 2,
        ...patch,
      }
      expect(deriveOnboardingSteps(counts)).toBeNull()
    }
  )
})

describe('nextOnboardingStep', () => {
  it('stops at the first gap, not at the last step', () => {
    const steps = deriveOnboardingSteps({
      upstreamChannels: 0,
      accounts: 0,
      routes: 7,
      keys: 9,
    })

    // Routes and keys already exist, but the missing upstream is still the
    // step to fix first — the checklist guides one action at a time in order.
    expect(nextOnboardingStep(steps ?? [])?.id).toBe('upstreams')
  })
})
