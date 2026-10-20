// Configuration-only guidance using existing dashboard, route and key query owners.

import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  Cable,
  CheckCircle2,
  Circle,
  KeyRound,
  Route,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  downstreamKeysQueryKeys,
  type DownstreamKeysResponse,
} from '@/features/downstream-keys'
import { useRoutes } from '@/features/token-routes'
import { api } from '@/lib/api'
import { formatInt } from '@/lib/format'

import {
  deriveOnboardingSteps,
  nextOnboardingStep,
  type OnboardingStepId,
} from './onboarding-steps'

// Link to existing page contracts; account creation remains an optional branch.
function StepCta({
  step,
  hasSites,
}: {
  step: OnboardingStepId
  hasSites: boolean
}) {
  const { t } = useTranslation()
  switch (step) {
    case 'upstreams':
      return (
        <div className='flex flex-wrap gap-2'>
          <Button size='sm' render={<Link to='/channels' />}>
            {t('dashboard.onboarding.connectUpstream')}
            <ArrowRight className='size-3.5' aria-hidden='true' />
          </Button>
          {hasSites && (
            <Button
              size='sm'
              variant='outline'
              render={<Link to='/accounts' search={{ create: true }} />}
            >
              {t('dashboard.onboarding.createAccount')}
            </Button>
          )}
        </div>
      )
    case 'routes':
      return (
        <Button size='sm' render={<Link to='/token-routes' />}>
          {t('dashboard.onboarding.createRoute')}
          <ArrowRight className='size-3.5' aria-hidden='true' />
        </Button>
      )
    case 'keys':
      return (
        <Button size='sm' render={<Link to='/downstream-keys' />}>
          {t('dashboard.onboarding.createKey')}
          <ArrowRight className='size-3.5' aria-hidden='true' />
        </Button>
      )
  }
}

export function OnboardingChecklist({
  siteCount,
  accountCount,
  upstreamChannelCount,
}: {
  /** From GET /api/stats/dashboard?view=summary — undefined while loading. */
  siteCount?: number
  accountCount?: number
  upstreamChannelCount?: number
}) {
  const { t } = useTranslation()

  const routesQuery = useRoutes()
  // Mirrors the keys section's query exactly (key + queryFn + staleTime) so
  // the two surfaces share one cache entry instead of racing two shapes.
  const keysQuery = useQuery({
    queryKey: downstreamKeysQueryKeys.list(),
    queryFn: async () =>
      (await api.getDownstreamApiKeys()) as DownstreamKeysResponse,
    staleTime: 15 * 1000,
  })

  const steps = deriveOnboardingSteps({
    accounts: accountCount,
    upstreamChannels: upstreamChannelCount,
    routes: routesQuery.data?.length,
    keys: keysQuery.data?.items.length,
  })
  const next = steps === null ? null : nextOnboardingStep(steps)

  // Hidden until every count has answered, and retired once the journey is
  // built — the panel is advisory, so an unknown or errored source keeps it
  // silent rather than claiming a gap.
  if (steps === null || next === null) return null

  return (
    <Card className='ring-primary/25 bg-primary/[0.04] overflow-hidden'>
      <CardContent className='flex flex-col gap-4'>
        <div className='space-y-1'>
          <h2 className='text-base font-semibold'>
            {t('dashboard.onboarding.title')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t('dashboard.onboarding.description')}
          </p>
        </div>
        <ol className='grid gap-2 sm:grid-cols-3'>
          {steps.map((step) => (
            <li
              key={step.id}
              className='bg-background/70 flex min-w-0 flex-col gap-3 rounded-lg border p-3'
            >
              <div className='flex min-w-0 items-start gap-2.5'>
                <span
                  className={
                    step.done
                      ? 'bg-success/10 text-success grid size-8 shrink-0 place-items-center rounded-md'
                      : 'bg-primary/10 text-primary grid size-8 shrink-0 place-items-center rounded-md'
                  }
                >
                  <StepIcon step={step.id} />
                </span>
                <div className='min-w-0'>
                  <p className='text-sm font-medium'>
                    {t(`dashboard.onboarding.steps.${step.id}.title`)}
                  </p>
                  <p className='text-muted-foreground mt-0.5 line-clamp-2 text-xs leading-5'>
                    {t(`dashboard.onboarding.steps.${step.id}.hint`)}
                  </p>
                </div>
              </div>
              <div className='mt-auto flex min-w-0 flex-wrap items-center justify-between gap-2'>
                {step.done ? (
                  <span className='text-muted-foreground inline-flex items-center gap-1.5 text-xs'>
                    <CheckCircle2
                      className='text-success size-3.5'
                      aria-hidden='true'
                    />
                    {t('dashboard.onboarding.stepDone')}
                    <span className='tabular-nums'>
                      {formatInt(step.count)}
                    </span>
                  </span>
                ) : (
                  <span className='text-muted-foreground inline-flex items-center gap-1.5 text-xs'>
                    <Circle className='size-3.5' aria-hidden='true' />
                    {t('dashboard.onboarding.stepPending')}
                  </span>
                )}
                {next.id === step.id ? (
                  <StepCta step={step.id} hasSites={(siteCount ?? 0) > 0} />
                ) : null}
              </div>
            </li>
          ))}
        </ol>
      </CardContent>
    </Card>
  )
}

function StepIcon({ step }: { step: OnboardingStepId }) {
  switch (step) {
    case 'upstreams':
      return <Cable className='size-4' aria-hidden='true' />
    case 'routes':
      return <Route className='size-4' aria-hidden='true' />
    case 'keys':
      return <KeyRound className='size-4' aria-hidden='true' />
  }
}
