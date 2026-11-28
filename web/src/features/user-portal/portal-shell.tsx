import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Activity,
  Box,
  KeyRound,
  LayoutDashboard,
  LogOut,
  RefreshCw,
  UserRound,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { BrandLogo } from '@/components/ui/brand-logo'
import { Button, buttonVariants } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { useDelayedSpin } from '@/hooks/use-quiet-refresh'

import {
  userPortalApi,
  portalQueryRoot,
  type UserSession,
  type PortalView,
  type PersonalModel,
} from './api'

const navigation = [
  ['overview', LayoutDashboard],
  ['models', Box],
  ['keys', KeyRound],
  ['requests', Activity],
  ['account', UserRound],
] as const

export function PortalShell({
  session,
  view,
  model,
  protocol,
  children,
}: {
  children: ReactNode
  session: UserSession
  view: PortalView
  model?: string
  protocol?: PersonalModel['protocols'][number]
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [logoutPending, setLogoutPending] = useState(false)
  const [logoutFailed, setLogoutFailed] = useState(false)
  const refreshing = useIsFetching({ queryKey: portalQueryRoot }) > 0
  const showRefreshSpin = useDelayedSpin(refreshing)
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
          className='flex flex-wrap gap-1 md:flex-col md:flex-nowrap'
        >
          {navigation.map(([item, Icon]) => (
            <Link
              key={item}
              to='/user'
              search={{
                view: item,
                page: 1,
                ...(item === 'keys' || item === 'models'
                  ? { model, protocol }
                  : {}),
              }}
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
                disabled={refreshing}
                onClick={() =>
                  void client.invalidateQueries({ queryKey: portalQueryRoot })
                }
              >
                <RefreshCw
                  aria-hidden='true'
                  className={showRefreshSpin ? 'animate-spin' : undefined}
                />
                {t(refreshing ? 'userPortal.refreshing' : 'userPortal.refresh')}
              </Button>
            }
          />
          {logoutFailed && (
            <Notice tone='destructive' role='alert'>
              {t('userPortal.actionFailed')}
            </Notice>
          )}
          {children}
        </main>
      </div>
    </div>
  )
}
