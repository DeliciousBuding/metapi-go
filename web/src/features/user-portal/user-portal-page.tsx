import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import {
  userPortalApi,
  portalQueryRoot,
  type UserSession,
  type PortalView,
  type PersonalModel,
} from './api'
import { PortalAccount } from './components/portal-account'
import { PortalKeys } from './components/portal-keys'
import { PortalLogin } from './components/portal-login'
import { PortalModels } from './components/portal-models'
import { PortalRequests } from './components/portal-requests'
import { readModelContext } from './connection-context'
import { PortalOverview } from './portal-overview'
import { PortalShell } from './portal-shell'

type PortalPageProps = {
  view: PortalView
  page: number
  requestId?: string
  model?: string
  protocol?: PersonalModel['protocols'][number]
}

function PortalContent({
  session,
  view,
  page,
  requestId,
  model,
  protocol,
}: PortalPageProps & { session: UserSession }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const locationSearch = useLocation({
    select: (location) => location.searchStr,
  })
  return (
    <>
      {view === 'overview' && <PortalOverview userId={session.user.id} />}
      {view === 'models' && <PortalModels userId={session.user.id} />}
      {view === 'keys' && (
        <>
          {model && (
            <Notice tone='info'>
              <p className='wrap-anywhere'>
                {t('userPortal.selectedConnectionModel', { model })}
              </p>
              <Link
                to='/user'
                search={{ view: 'models', page: 1, model, protocol }}
                className='text-primary font-medium underline underline-offset-4'
              >
                {t('userPortal.returnToConnection')}
              </Link>
            </Notice>
          )}
          <PortalKeys
            userId={session.user.id}
            connectionContext={readModelContext(locationSearch)}
          />
        </>
      )}
      {view === 'requests' && (
        <PortalRequests
          userId={session.user.id}
          page={page}
          requestId={requestId}
          onCloseDetail={() =>
            void navigate({
              href: (() => {
                const params = new URLSearchParams(locationSearch)
                params.delete('requestId')
                return '/user?' + params.toString()
              })(),
              replace: true,
            })
          }
        />
      )}
      {view === 'account' && <PortalAccount session={session} />}
    </>
  )
}

export function UserPortalPage(props: PortalPageProps) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const session = useQuery({
    queryKey: ['userPortal', 'session'],
    queryFn: async () => {
      const next = await userPortalApi.session()
      const previous = client.getQueryData<UserSession | null>([
        'userPortal',
        'session',
      ])
      if (
        !next ||
        (previous &&
          (previous.user.id !== next.user.id ||
            previous.user.groupId !== next.user.groupId))
      ) {
        await client.cancelQueries({
          queryKey: portalQueryRoot,
          predicate: (query) => query.queryKey[1] !== 'session',
        })
        client.removeQueries({
          queryKey: portalQueryRoot,
          predicate: (query) => query.queryKey[1] !== 'session',
        })
      }
      return next
    },
    retry: false,
    staleTime: 0,
    refetchInterval: 60_000,
    refetchOnWindowFocus: true,
  })
  if (session.isPending)
    return (
      <div
        role='status'
        className='flex min-h-svh items-center justify-center gap-2'
      >
        <Spinner />
        {t('common.loading')}
      </div>
    )
  if (session.isError)
    return (
      <main className='mx-auto max-w-lg space-y-4 p-6'>
        <PageHeader title={t('userPortal.title')} />
        <Notice tone='destructive' role='alert'>
          {t('userPortal.loadFailed')}
        </Notice>
        <Button onClick={() => void session.refetch()}>
          {t('common.retry')}
        </Button>
      </main>
    )
  if (!session.data) return <PortalLogin />
  return (
    <PortalShell
      key={session.data.user.id + ':' + session.data.user.groupId}
      session={session.data}
      {...props}
    >
      <PortalContent session={session.data} {...props} />
    </PortalShell>
  )
}
