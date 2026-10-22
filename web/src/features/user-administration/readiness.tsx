import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Notice } from '@/components/ui/notice'
import {
  gatewayUsersApi,
  type GatewayUser,
  type GatewayUserGroup,
} from '@/lib/api/gateway-users'

import { userAdminKeys } from './api'
import { LoadError } from './feedback'
import { availableQuotaPreview } from './quota-preview'

type CheckState = 'passed' | 'action' | 'unknown' | 'checking'

export function ReadinessDialog({
  userId,
  close,
  editUser,
  editGroup,
  editAccess,
  editBudget,
}: {
  userId: number
  close: () => void
  editUser: (user: GatewayUser) => void
  editGroup: (group: GatewayUserGroup) => void
  editAccess: (group: GatewayUserGroup) => void
  editBudget: (user: GatewayUser) => void
}) {
  const { t } = useTranslation()
  // Mounted only for the selected row. Reuse existing API owners and invalidate
  // keys; directory caps mean a missing identity is unknown, never enabled.
  const users = useQuery({
    queryKey: userAdminKeys.users,
    queryFn: gatewayUsersApi.users,
    refetchOnMount: 'always',
  })
  const groups = useQuery({
    queryKey: userAdminKeys.groups,
    queryFn: gatewayUsersApi.groups,
    refetchOnMount: 'always',
  })
  const user = !users.isError
    ? users.data?.items.find((item) => item.id === userId)
    : undefined
  const group = !groups.isError
    ? groups.data?.items.find((item) => item.id === user?.groupId)
    : undefined
  const grants = useQuery({
    queryKey: userAdminKeys.grants(group?.id ?? 0),
    queryFn: () => gatewayUsersApi.grants(group?.id ?? 0),
    enabled: Boolean(group),
    refetchOnMount: 'always',
  })
  const routes = useQuery({
    queryKey: userAdminKeys.routes,
    queryFn: gatewayUsersApi.publicRoutes,
    refetchOnMount: 'always',
  })
  const budget = useQuery({
    queryKey: userAdminKeys.budget(userId),
    queryFn: () => gatewayUsersApi.budget(userId),
    refetchOnMount: 'always',
  })
  const queries = [users, groups, grants, routes, budget]
  const refreshing = queries.some((query) => query.isFetching)
  const refresh = () => {
    for (const query of queries) {
      if (query === grants && !group) continue
      void query.refetch()
    }
  }
  const knownState = (known: boolean, passed: boolean): CheckState => {
    if (refreshing) return 'checking'
    if (!known) return 'unknown'
    return passed ? 'passed' : 'action'
  }
  const remaining = budget.data
    ? availableQuotaPreview(
        String(budget.data.limit),
        budget.data.used,
        budget.data.reserved
      )
    : null
  const hasRoute =
    routes.data?.some(
      (route) =>
        route.enabled &&
        grants.data?.items.some((grant) => grant.routeId === route.id)
    ) ?? false
  const checks = [
    {
      key: 'account',
      state: knownState(Boolean(user), user?.enabled ?? false),
      detail: user?.enabled ? 'accountEnabled' : 'accountDisabled',
      action: user ? () => editUser(user) : undefined,
      actionLabel: 'userAdmin.editUser',
    },
    {
      key: 'group',
      state: knownState(Boolean(group), group?.enabled ?? false),
      detail: group?.enabled ? 'groupEnabled' : 'groupDisabled',
      action: group ? () => editGroup(group) : undefined,
      actionLabel: 'userAdmin.editGroup',
    },
    {
      key: 'routes',
      state: knownState(
        Boolean(group) && grants.isSuccess && routes.isSuccess,
        hasRoute
      ),
      detail: hasRoute ? 'routeGranted' : 'noEnabledRoute',
      action: group ? () => editAccess(group) : undefined,
      actionLabel: 'userAdmin.accessAndPricing',
    },
    {
      key: 'quota',
      state: knownState(
        budget.isSuccess && (budget.data.unlimited || remaining !== null),
        Boolean(
          budget.data &&
          !budget.data.frozen &&
          (budget.data.unlimited || Number(remaining) > 0)
        )
      ),
      detail: budget.data?.frozen ? 'quotaFrozen' : 'quotaAvailable',
      action: user ? () => editBudget(user) : undefined,
      actionLabel: 'userAdmin.budget',
    },
  ]
  const allPassed = checks.every((check) => check.state === 'passed')
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {t('userAdmin.readiness.title', {
              user: user?.displayName || user?.username || `#${userId}`,
            })}
          </DialogTitle>
          <DialogDescription>
            {t('userAdmin.readiness.boundary')}
          </DialogDescription>
        </DialogHeader>
        <Notice tone={allPassed ? 'success' : 'warning'} role='status'>
          {t(`userAdmin.readiness.${allPassed ? 'passed' : 'notPassed'}`)}
        </Notice>
        {queries.map((query, index) => (
          <LoadError
            key={['users', 'groups', 'grants', 'routes', 'budget'][index]}
            error={query.error}
            retry={refresh}
            pending={refreshing}
          />
        ))}
        <div className='divide-y rounded-lg border'>
          {checks.map((check) => (
            <section key={check.key} className='space-y-2 p-3'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <h2 className='font-medium'>
                  {t(`userAdmin.readiness.${check.key}`)}
                </h2>
                <span className='bg-muted rounded px-2 py-1 text-xs'>
                  {t(`userAdmin.readiness.state.${check.state}`)}
                </span>
              </div>
              <p className='text-muted-foreground text-sm'>
                {t(
                  `userAdmin.readiness.${check.state === 'unknown' || check.state === 'checking' ? 'unknownHint' : check.detail}`,
                  {
                    group: group?.name,
                    remaining: budget.data?.unlimited
                      ? t('userAdmin.readiness.unlimited')
                      : remaining,
                  }
                )}
              </p>
              {check.action && (
                <Button
                  size='sm'
                  variant='outline'
                  disabled={refreshing}
                  onClick={() => {
                    close()
                    check.action?.()
                  }}
                >
                  {t(check.actionLabel)}
                </Button>
              )}
              {check.key === 'group' && !group && user && (
                <Button
                  size='sm'
                  variant='outline'
                  disabled={refreshing || groups.isError}
                  onClick={() => {
                    close()
                    editUser(user)
                  }}
                >
                  {t('userAdmin.editUser')}
                </Button>
              )}
            </section>
          ))}
        </div>
        <section className='space-y-1'>
          <h2 className='font-medium'>{t('userAdmin.readiness.activation')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('userAdmin.readiness.activationUnknown')}
          </p>
        </section>
        <div className='flex flex-wrap justify-end gap-2'>
          <Button variant='outline' disabled={refreshing} onClick={refresh}>
            {t('userAdmin.refresh')}
          </Button>
          <Button variant='outline' onClick={close}>
            {t('userAdmin.close')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
