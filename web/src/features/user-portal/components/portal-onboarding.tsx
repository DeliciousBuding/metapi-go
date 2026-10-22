import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Check, CircleHelp, KeyRound, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import { userPortalApi, type PersonalKey } from '../api'
import { personalKeyStatus } from '../key-status'

type StepState = 'complete' | 'incomplete' | 'unknown' | 'pending'

function queryState(
  pending: boolean,
  error: boolean,
  complete: boolean
): StepState {
  if (pending) return 'pending'
  if (error) return 'unknown'
  return complete ? 'complete' : 'incomplete'
}

function usableKey(key: PersonalKey): boolean {
  return personalKeyStatus(key) === 'available'
}

function StepMark({ state }: { state: StepState }) {
  if (state === 'pending') return <Spinner aria-hidden='true' />
  if (state === 'complete') {
    return (
      <span className='bg-primary text-primary-foreground flex size-7 shrink-0 items-center justify-center rounded-full'>
        <Check aria-hidden='true' className='size-4' />
      </span>
    )
  }
  return (
    <span className='text-muted-foreground flex size-7 shrink-0 items-center justify-center rounded-full border'>
      <CircleHelp aria-hidden='true' className='size-4' />
    </span>
  )
}

function OnboardingStep({
  index,
  state,
  title,
  description,
  action,
  detail,
}: {
  index: number
  state: StepState
  title: string
  description: string
  action?: React.ReactNode
  detail?: React.ReactNode
}) {
  const { t } = useTranslation()
  return (
    <li className='flex min-w-0 gap-3'>
      <StepMark state={state} />
      <div className='min-w-0 flex-1 space-y-1'>
        <p className='font-medium'>
          <span className='sr-only'>
            {t('userPortal.onboarding.step', { index })}:{' '}
          </span>
          {title}
        </p>
        <p className='text-muted-foreground text-sm leading-relaxed'>
          {description}
        </p>
        {detail}
        {action}
      </div>
    </li>
  )
}

function Retry({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation()
  return (
    <Button variant='outline' size='sm' onClick={onClick}>
      <RefreshCw aria-hidden='true' />
      {t('common.retry')}
    </Button>
  )
}

type NavigationSearch = {
  view: 'models' | 'keys' | 'requests'
  page: number
}

function StepAction({
  error,
  onRetry,
  search,
  children,
  icon,
}: {
  error: boolean
  onRetry: () => void
  search: NavigationSearch
  children: React.ReactNode
  icon?: React.ReactNode
}) {
  if (error) return <Retry onClick={onRetry} />
  return (
    <Link
      to='/user'
      search={search}
      className={buttonVariants({
        variant: 'link',
        className: 'h-auto gap-1 px-0',
      })}
    >
      {icon}
      {children}
    </Link>
  )
}

export function PortalOnboarding({ userId }: { userId: number }) {
  const { t } = useTranslation()
  const models = useQuery({
    queryKey: ['userPortal', userId, 'models'],
    queryFn: userPortalApi.models,
  })
  const keys = useQuery({
    queryKey: ['userPortal', userId, 'keys'],
    queryFn: userPortalApi.keys,
  })
  const requests = useQuery({
    queryKey: ['userPortal', userId, 'requests', 1],
    queryFn: () => userPortalApi.requests(1),
    refetchOnMount: 'always',
  })

  const modelState = queryState(
    models.isPending,
    models.isError,
    !models.isPending && !models.isError && models.data.items.length > 0
  )
  const keyCreated =
    !keys.isPending && !keys.isError && keys.data.items.length > 0
  const keyAvailable = keyCreated && keys.data.items.some(usableKey)
  const keyState = queryState(keys.isPending, keys.isError, keyAvailable)
  const requestCreated =
    !requests.isPending && !requests.isError && requests.data.items.length > 0
  const requestState = queryState(
    requests.isPending,
    requests.isError,
    requestCreated
  )
  const completedRequest =
    requestCreated &&
    requests.data.items.some((item) => item.terminalStatus === 'completed')
  const resultState = queryState(
    requests.isPending,
    requests.isError,
    completedRequest
  )

  const modelDescriptionKey = 'userPortal.onboarding.steps.model.' + modelState
  let keyDescriptionKey = 'userPortal.onboarding.steps.key.incomplete'
  if (keys.isError)
    keyDescriptionKey = 'userPortal.onboarding.steps.key.unknown'
  else if (keys.isPending)
    keyDescriptionKey = 'userPortal.onboarding.steps.key.pending'
  else if (keyAvailable)
    keyDescriptionKey = 'userPortal.onboarding.steps.key.complete'
  else if (keyCreated)
    keyDescriptionKey = 'userPortal.onboarding.steps.key.unavailable'
  const requestDescriptionKey =
    'userPortal.onboarding.steps.request.' + requestState
  const resultDescriptionKey =
    'userPortal.onboarding.steps.result.' + resultState

  return (
    <Card>
      <CardContent className='space-y-4'>
        <div className='space-y-1'>
          <h2 className='font-medium'>{t('userPortal.onboarding.title')}</h2>
          <p className='text-muted-foreground text-sm leading-relaxed'>
            {t('userPortal.onboarding.hint')}
          </p>
        </div>
        <ol className='grid gap-5 lg:grid-cols-2'>
          <OnboardingStep
            index={1}
            state={modelState}
            title={t('userPortal.onboarding.steps.model.title')}
            description={t(modelDescriptionKey)}
            action={
              <StepAction
                error={models.isError}
                onRetry={() => void models.refetch()}
                search={{ view: 'models', page: 1 }}
              >
                {t('userPortal.onboarding.openModels')}
              </StepAction>
            }
          />
          <OnboardingStep
            index={2}
            state={keyState}
            title={t('userPortal.onboarding.steps.key.title')}
            description={t(keyDescriptionKey)}
            detail={
              keyCreated && !keyAvailable ? (
                <p className='text-muted-foreground text-xs'>
                  {t('userPortal.onboarding.keyCreated')}
                </p>
              ) : undefined
            }
            action={
              <StepAction
                error={keys.isError}
                onRetry={() => void keys.refetch()}
                search={{ view: 'keys', page: 1 }}
                icon={<KeyRound aria-hidden='true' className='size-4' />}
              >
                {t('userPortal.onboarding.openKeys')}
              </StepAction>
            }
          />
          <OnboardingStep
            index={3}
            state={requestState}
            title={t('userPortal.onboarding.steps.request.title')}
            description={t(requestDescriptionKey)}
            action={
              <StepAction
                error={requests.isError}
                onRetry={() => void requests.refetch()}
                search={{ view: 'models', page: 1 }}
              >
                {t('userPortal.onboarding.openModels')}
              </StepAction>
            }
          />
          <OnboardingStep
            index={4}
            state={resultState}
            title={t('userPortal.onboarding.steps.result.title')}
            description={t(resultDescriptionKey)}
            action={
              <StepAction
                error={requests.isError}
                onRetry={() => void requests.refetch()}
                search={{ view: 'requests', page: 1 }}
              >
                {t('userPortal.onboarding.openRequests')}
              </StepAction>
            }
          />
        </ol>
        <Notice tone='info'>{t('userPortal.onboarding.clientCheck')}</Notice>
      </CardContent>
    </Card>
  )
}
