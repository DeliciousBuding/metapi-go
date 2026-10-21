import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  Activity,
  Box,
  KeyRound,
  LayoutDashboard,
  LogOut,
  RefreshCw,
  UserRound,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { BrandLogo } from '@/components/ui/brand-logo'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import {
  userPortalApi,
  portalQueryRoot,
  type UserSession,
  type PortalView,
} from './api'
import { PortalAccount } from './components/portal-account'
import { CopyValue, PortalKeys } from './components/portal-keys'
import { PortalLogin } from './components/portal-login'
import { PortalRequests } from './components/portal-requests'

const navigation = [
  ['overview', LayoutDashboard],
  ['models', Box],
  ['keys', KeyRound],
  ['requests', Activity],
  ['account', UserRound],
] as const

function PortalOverview({ userId }: { userId: number }) {
  const { t, i18n } = useTranslation()
  const budget = useQuery({
    queryKey: ['userPortal', userId, 'budget'],
    queryFn: userPortalApi.budget,
  })
  const usage = useQuery({
    queryKey: ['userPortal', userId, 'usage'],
    queryFn: userPortalApi.usage,
  })
  const number = (value: number) =>
    new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 6 }).format(
      value
    )
  return (
    <div className='space-y-5'>
      {budget.isPending ? (
        <Spinner aria-label={t('common.loading')} />
      ) : budget.isError ? (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void budget.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      ) : (
        <>
          {budget.data.frozen && (
            <Notice tone='warning'>
              {t('userPortal.frozenBudget', {
                count: budget.data.unknownRequests,
              })}
            </Notice>
          )}
          <div className='grid gap-4 sm:grid-cols-3'>
            {[
              [
                'remaining',
                budget.data.unlimited
                  ? t('userPortal.unlimited')
                  : number(
                      Math.max(
                        0,
                        budget.data.limit -
                          budget.data.used -
                          budget.data.reserved
                      )
                    ),
              ],
              ['used', number(budget.data.used)],
              ['reserved', number(budget.data.reserved)],
            ].map(([label, value]) => (
              <Card key={label}>
                <CardContent className='space-y-2'>
                  <h2 className='text-muted-foreground text-sm'>
                    {t('userPortal.' + label)}
                  </h2>
                  <p className='text-2xl font-semibold tabular-nums'>{value}</p>
                </CardContent>
              </Card>
            ))}
          </div>
          <p className='text-muted-foreground text-sm leading-relaxed'>
            {t('userPortal.budgetHint')}
          </p>
        </>
      )}
      {usage.isError ? (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void usage.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      ) : usage.isPending ? (
        <Spinner aria-label={t('common.loading')} />
      ) : (
        <dl className='grid gap-4 rounded-xl border p-4 sm:grid-cols-2'>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('userPortal.keyCount')}
            </dt>
            <dd className='mt-1 text-xl font-semibold tabular-nums'>
              {usage.data.keyCount}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('userPortal.keyRequestCount')}
            </dt>
            <dd className='mt-1 text-xl font-semibold tabular-nums'>
              {usage.data.usedRequests.toLocaleString()}
            </dd>
          </div>
        </dl>
      )}
      <div className='flex flex-wrap gap-2'>
        <Button
          render={<Link to='/user' search={{ view: 'models', page: 1 }} />}
        >
          {t('userPortal.browseModels')}
        </Button>
        <Button
          variant='outline'
          render={<Link to='/user' search={{ view: 'keys', page: 1 }} />}
        >
          {t('userPortal.createKey')}
        </Button>
      </div>
    </div>
  )
}

function PortalModels({ userId }: { userId: number }) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['userPortal', userId, 'models'],
    queryFn: userPortalApi.models,
  })
  if (query.isPending) return <Spinner aria-label={t('common.loading')} />
  if (query.isError)
    return (
      <Notice tone='destructive' role='alert'>
        <span>{t('userPortal.loadFailed')}</span>
        <Button
          variant='outline'
          size='sm'
          onClick={() => void query.refetch()}
        >
          {t('common.retry')}
        </Button>
      </Notice>
    )
  const endpoint = window.location.origin + '/v1'
  return (
    <div className='space-y-5'>
      <p className='text-muted-foreground text-sm'>
        {t('userPortal.modelsHint')}
      </p>
      {query.data.items.length === 0 ? (
        <Notice tone='info'>{t('userPortal.noModels')}</Notice>
      ) : (
        <div className='divide-border divide-y rounded-xl border'>
          {query.data.items.map((model) => (
            <div
              className='flex flex-wrap items-center justify-between gap-3 p-4'
              key={model.routeId + ':' + model.id}
            >
              <span className='min-w-0 font-medium wrap-anywhere'>
                {model.id}
              </span>
              <CopyValue value={model.id} label={t('userPortal.copyModel')} />
            </div>
          ))}
        </div>
      )}
      <Card>
        <CardContent className='space-y-3'>
          <h2 className='font-semibold'>{t('userPortal.connectTitle')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('userPortal.connectHint')}
          </p>
          <p className='font-mono text-sm wrap-anywhere'>{endpoint}</p>
          <CopyValue value={endpoint} label={t('userPortal.copyEndpoint')} />
          <Button
            variant='outline'
            className='ms-2'
            render={<Link to='/user' search={{ view: 'keys', page: 1 }} />}
          >
            {t('userPortal.createKey')}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}

function PortalShell({
  session,
  view,
  page,
  requestId,
}: {
  session: UserSession
  view: PortalView
  page: number
  requestId?: string
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const navigate = useNavigate()
  const [logoutPending, setLogoutPending] = useState(false)
  const [logoutFailed, setLogoutFailed] = useState(false)
  async function logout() {
    setLogoutPending(true)
    setLogoutFailed(false)
    try {
      await userPortalApi.logout()
      await client.cancelQueries({ queryKey: portalQueryRoot })
      client.removeQueries({
        queryKey: portalQueryRoot,
        predicate: (q) => q.queryKey[1] !== 'session',
      })
      client.setQueryData(['userPortal', 'session'], null)
    } catch {
      setLogoutFailed(true)
    } finally {
      setLogoutPending(false)
    }
  }
  return (
    <div className='bg-background min-h-svh'>
      <header className='border-border bg-background/95 supports-[backdrop-filter]:bg-background/60 sticky top-0 z-10 border-b supports-[backdrop-filter]:backdrop-blur-lg'>
        <div className='max-w-container mx-auto flex flex-wrap items-center justify-between gap-3 px-4 py-3 md:px-6'>
          <div className='flex min-w-0 items-center gap-3'>
            <BrandLogo className='size-8 shrink-0' />
            <div className='min-w-0'>
              <p className='font-semibold'>{t('userPortal.title')}</p>
              <p className='text-muted-foreground text-xs wrap-anywhere'>
                {session.user.displayName || session.user.username}
                {' · '}
                {session.group.name}
              </p>
            </div>
          </div>
          <div className='flex items-center gap-2'>
            <InterfaceControls />
            <Button
              variant='ghost'
              size='sm'
              disabled={logoutPending}
              onClick={() => void logout()}
            >
              <LogOut aria-hidden='true' />
              {t('userPortal.signOut')}
            </Button>
          </div>
        </div>
      </header>
      <div className='max-w-container mx-auto grid gap-6 px-4 py-6 md:grid-cols-[176px_minmax(0,1fr)] md:px-6'>
        <nav
          aria-label={t('userPortal.navigation')}
          className='flex gap-1 overflow-x-auto md:flex-col md:overflow-visible'
        >
          {navigation.map(([item, Icon]) => (
            <Link
              key={item}
              to='/user'
              search={{ view: item, page: 1 }}
              aria-current={view === item ? 'page' : undefined}
              className={buttonVariants({
                variant: view === item ? 'secondary' : 'ghost',
                className: 'shrink-0 justify-start',
              })}
            >
              <Icon aria-hidden='true' />
              {t('userPortal.nav.' + item)}
            </Link>
          ))}
        </nav>
        <main className='min-w-0 space-y-6'>
          <PageHeader
            title={t('userPortal.nav.' + view)}
            description={t('userPortal.description.' + view)}
            actions={
              <Button
                variant='outline'
                size='sm'
                onClick={() =>
                  void client.invalidateQueries({ queryKey: portalQueryRoot })
                }
              >
                <RefreshCw aria-hidden='true' />
                {t('userPortal.refresh')}
              </Button>
            }
          />
          {logoutFailed && (
            <Notice tone='destructive' role='alert'>
              {t('userPortal.actionFailed')}
            </Notice>
          )}
          {view === 'overview' && <PortalOverview userId={session.user.id} />}
          {view === 'models' && <PortalModels userId={session.user.id} />}
          {view === 'keys' && <PortalKeys userId={session.user.id} />}
          {view === 'requests' && (
            <PortalRequests
              userId={session.user.id}
              page={page}
              requestId={requestId}
              onCloseDetail={() =>
                void navigate({
                  to: '/user',
                  search: { view: 'requests', page },
                  replace: true,
                })
              }
            />
          )}
          {view === 'account' && <PortalAccount session={session} />}
        </main>
      </div>
    </div>
  )
}

export function UserPortalPage(props: {
  view: PortalView
  page: number
  requestId?: string
}) {
  const { t } = useTranslation()
  const session = useQuery({
    queryKey: ['userPortal', 'session'],
    queryFn: userPortalApi.session,
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
    <PortalShell key={session.data.user.id} session={session.data} {...props} />
  )
}
