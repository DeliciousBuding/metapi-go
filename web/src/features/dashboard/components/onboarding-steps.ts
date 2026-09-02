// metapi-go/features/dashboard/components — onboarding checklist state machine.
//
// The journey to a callable /v1 is four entities deep, and each one has an
// existing admin count endpoint:
//
//   sites    GET /api/stats/dashboard?view=summary  → siteCount
//   accounts GET /api/stats/dashboard?view=summary  → totalAccounts
//   routes   GET /api/routes/summary                → array length
//   keys     GET /api/downstream-keys               → items length
//
// Deriving the checklist from those four counts (rather than from a
// "has the operator ever seen this card" flag) keeps the panel honest: it
// reports what is actually built, so it reappears if a step is emptied and
// disappears the moment the last step is filled. Pure module — the section
// owns the fetching, this owns the verdict.

export type OnboardingStepId = 'sites' | 'accounts' | 'routes' | 'keys'

export type OnboardingStep = {
  id: OnboardingStepId
  count: number
  done: boolean
}

/** Resolved entity counts. `undefined`/`null` = this source has not answered. */
export type OnboardingCounts = Record<
  OnboardingStepId,
  number | undefined | null
>

/** Journey order — the checklist renders top-down in exactly this sequence. */
const STEP_ORDER: OnboardingStepId[] = ['sites', 'accounts', 'routes', 'keys']

/**
 * Turn the four counts into ordered checklist steps, or `null` when any count
 * is still unknown.
 *
 * The `null` is the anti-flash contract the previous single-step banner had
 * (it keyed off `siteCount === 0`, which is false while the snapshot loads):
 * an unanswered source must never be read as "0 — to do", or the panel would
 * claim a gap that the very next byte disproves. A failed source therefore
 * keeps the panel hidden instead of inventing work.
 */
export function deriveOnboardingSteps(
  counts: OnboardingCounts
): OnboardingStep[] | null {
  const steps: OnboardingStep[] = []
  for (const id of STEP_ORDER) {
    const count = counts[id]
    if (typeof count !== 'number' || !Number.isFinite(count)) return null
    steps.push({ id, count, done: count > 0 })
  }
  return steps
}

/**
 * The first step with nothing built yet — the single row that carries a CTA,
 * so the panel guides one action at a time instead of offering four.
 * `null` means the whole journey is built and the panel should not render.
 */
export function nextOnboardingStep(
  steps: OnboardingStep[]
): OnboardingStep | null {
  return steps.find((step) => !step.done) ?? null
}
