// Configuration milestones, not proof of successful upstream requests.
// Account/OAuth and independent connections are alternative supply paths.

export type OnboardingStepId = 'upstreams' | 'routes' | 'keys'

export type OnboardingStep = {
  id: OnboardingStepId
  count: number
  done: boolean
}

/** Resolved entity counts. `undefined`/`null` = this source has not answered. */
export type OnboardingCounts = Record<
  'accounts' | 'upstreamChannels' | 'routes' | 'keys',
  number | undefined | null
>

/** Journey order — the checklist renders top-down in exactly this sequence. */
const STEP_ORDER: OnboardingStepId[] = ['upstreams', 'routes', 'keys']

// Missing, failed or invalid counts keep the advisory panel hidden.
export function deriveOnboardingSteps(
  counts: OnboardingCounts
): OnboardingStep[] | null {
  for (const count of Object.values(counts)) {
    if (typeof count !== 'number' || !Number.isFinite(count) || count < 0) {
      return null
    }
  }
  const milestoneCounts = {
    upstreams:
      (counts.accounts as number) + (counts.upstreamChannels as number),
    routes: counts.routes as number,
    keys: counts.keys as number,
  }
  const steps: OnboardingStep[] = []
  for (const id of STEP_ORDER) {
    const count = milestoneCounts[id]
    steps.push({ id, count, done: count > 0 })
  }
  return steps
}

/**
 * The first step with nothing built yet — the single row that carries a CTA,
 * so the panel guides one action at a time instead of offering four.
 * `null` means the configuration milestones exist and the panel should not render.
 */
export function nextOnboardingStep(
  steps: OnboardingStep[]
): OnboardingStep | null {
  return steps.find((step) => !step.done) ?? null
}
